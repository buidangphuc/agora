# platform-recsys

Offline recommendation training job (bounded context: recommendation model artifacts). It reads
behavioural events, trains an implicit-feedback ALS model with PySpark, evaluates it on a
leakage-free holdout, runs a champion/challenger promotion gate, and only when the run is promoted
publishes item/user vectors to **Qdrant** and precomputed Top-N lists to **Redis**.

**Status:** a batch job, not a service. No gRPC/HTTP surface, no Kafka, no database of its own.
The container entrypoint is the job (`python -m recsys`). Qdrant (`:6333`) and Redis (`:6379`) are
shared external dependencies it writes to, not ports it owns. It is in the root compose behind the
`jobs` profile; nothing schedules it locally.

It owns: the ALS training recipe, the evaluation protocol, the model registry/gate metadata in
Redis, the `item_als_vectors` / `user_als_vectors` Qdrant collections and the `recs:v1:*` Redis keys.
`team-ai` (`serve-recommendations-teamai`) reads those artifacts.

## 1. Contract

There are no served RPCs or endpoints, so there is no authorization rule. The contract is data in and
data out.

**Consumes**

| What | Source | Detail |
|---|---|---|
| `tracking_events` | Parquet exported by `team-analytics` (`PARQUET_EXPORT_PATH=/data/tracking_events.parquet`, every `PARQUET_EXPORT_INTERVAL_SECONDS=300`) on the `analytics_data` volume; mounted read-only at `/data` | Columns listed in `recsys/warehouse.py` `TRACKING_COLUMNS` (the read contract). Only those present are kept. Rows are bounded by `occurred_at >= now - INTERACTION_WINDOW_DAYS`. |
| Alternative source | BigQuery table via the Spark BigQuery connector | `WAREHOUSE_DRIVER=bigquery` + `BIGQUERY_PROJECT/DATASET/TABLE` |

Triples: `user_key` = `principal_id`, else `anonymous_id` (rows with neither are dropped);
`listing_id` must be non-empty; weight = per-event weight (unknown event types get 0.5) summed per
(user, item), optionally recency-decayed.

**Produces (only for a promoted run)**

| Store | Name | Content |
|---|---|---|
| Qdrant | `QDRANT_ITEM_COLLECTION` (`item_als_vectors`) | L2-normalised item factors, cosine, dim = `ALS_RANK`. Payload `listing_id`, `model_version`, `updated_at`. |
| Qdrant | `QDRANT_USER_COLLECTION` (`user_als_vectors`) | Same for users; payload `user_key`. |
| Qdrant | `QDRANT_TWO_TOWER_COLLECTION` (`item_two_tower_vectors`) | Only when `ENABLE_TWO_TOWER=true` (see Known gaps). |
| Redis | `recs:v1:user:{user_key}` | JSON `[{listing_id, score}]`, capped at `TOP_N` |
| Redis | `recs:v1:item:{listing_id}` | Similar items, same shape |
| Redis | `recs:v1:popular` | Popularity fallback (summed weight), same shape |
| Redis | `recs:v1:model_version` | Written last, flips only after the generation is loaded |
| Redis | `recs:model:champion`, `recs:model:meta:{model_version}` | Registry state (champion pointer, metadata JSON incl. metrics and status) |

All cache keys carry TTL `RECS_CACHE_TTL_SECONDS` (172800 s). Qdrant point id is
`uuid5(namespace, source_id)` (`recsys/load/qdrant.py`), pinned by `tests/test_point_id.py`; consumers
depend on it. After upserting, points whose `model_version` differs from the current one are deleted.

Producer/consumer agreements that must hold: `QDRANT_ITEM_COLLECTION` = team-ai `RECS_QDRANT_COLLECTION`;
`RECS_CACHE_PREFIX` / `RECS_SCHEMA_VERSION` = team-ai `RECS_CACHE_PREFIX` / `RECS_CACHE_SCHEMA_VERSION`.

## 2. Events

None produced, none consumed. (`tracking_events` reaches the job through the Parquet export, not Kafka.)

## 3. Data

No relational DB and no migrations. State lives in Qdrant and Redis (section 1). The model registry
uses Redis when it is reachable; otherwise it falls back to an in-memory registry for that process
(see Known gaps).

## 4. Pipeline, evaluation and promotion gate

Flow (`recsys/pipeline.py`): read events, build triples, index, fit ALS (`implicitPrefs=true`,
Spark MLlib), collect factors and L2-normalise, precompute Top-N, **evaluate**, **gate**, then publish.

**Evaluation protocol `leave-last-new-item-v1`** (`recsys/evals/holdout.py`):

- For each user with at least two distinct listings, the held-out target is the listing the user
  discovered most recently (latest first interaction). Anonymous rows are never held out.
- A **second ALS model** is fitted on training data that, per test user, keeps only events strictly
  before that discovery. The published model is trained on all events; it is not the one scored.
- Each test user's already-seen items are excluded from the ranking. Metrics: Recall, NDCG, MRR,
  coverage at K = 5, 10, 20 (e.g. `ndcg@10`, `coverage@10`), stamped with `eval_protocol`.
- The registry never compares metrics across protocols: if the champion's `eval_protocol` differs from
  the candidate's, the candidate is promoted as incomparable.
- `recsys/evals/split.py` (`temporal_train_test_split`, global time ratio) is not what the pipeline uses.

**Run outcomes** (`decision` in the returned and logged summary):

| decision | When | Effect |
|---|---|---|
| `skipped` | The split yields no test events (no usable holdout) | Nothing registered, gate not consulted, nothing published |
| `promoted` | No champion yet (first run bootstraps), or champion metadata missing, or protocol mismatch, or the gate passes, or `PROMOTION_FORCE=true` | Registered as `champion`, Qdrant + Redis published, optional Two-Tower stage |
| `rejected` | Gate fails against the same-protocol champion | Candidate marked `rejected`; previous generation keeps serving, nothing published |

Gate (`ModelEvaluator.compare_models`): primary metric `PROMOTION_PRIMARY_METRIC` (`ndcg@10`) must
improve by at least `PROMOTION_MIN_RELATIVE_IMPROVEMENT` (0.01) and, when both runs report it,
`coverage@10` must stay at least `PROMOTION_MIN_COVERAGE_RATIO` (0.8) of the champion's. Summary keys:
`model_version`, `decision`, `reason`, `primary_metric`, `candidate_value`, `incumbent_version`,
`incumbent_value`, `metrics`, `items`, `users`, `popular`, plus `qdrant`/`cache` counts (and
`two_tower_items`) when published. See ADR-0014.

**`PROMOTION_FORCE=true`** promotes and publishes the run even if the gate would reject it. Use it to
repopulate Qdrant/Redis after a reset. Never set it on a schedule. The first run with an empty
registry bootstraps on its own.

`MODEL_VERSION` overrides the stamp; otherwise `als-<UTC yyyymmddThhmmssZ>`.

## 5. Configuration

All settings are read in `recsys/config.py` (`_FIELDS`, the single source of truth); `.env.example`
mirrors it. `make check-env` (`tests/test_env_drift.py`) fails if the two drift, in either direction.

| Group | Variables (default) |
|---|---|
| Runtime | `ENV` (local), `LOG_LEVEL` (info) |
| Spark | `SPARK_MASTER` (local[*]), `SPARK_APP_NAME` (platform-recsys-als) |
| Warehouse | `WAREHOUSE_DRIVER` (duckdb; or bigquery), `WAREHOUSE_PARQUET_PATH` (/data/tracking_events.parquet), `BIGQUERY_PROJECT` (empty, required for bigquery), `BIGQUERY_DATASET` (analytics), `BIGQUERY_TABLE` (tracking_events) |
| Interactions | `INTERACTION_WINDOW_DAYS` (30), `EVENT_WEIGHTS_JSON`, `RECENCY_HALF_LIFE_DAYS` (0 = off), `MIN_INTERACTIONS_PER_USER` (1), `MIN_INTERACTIONS_PER_ITEM` (1) |
| ALS | `ALS_RANK` (64), `ALS_REG_PARAM` (0.05), `ALS_ALPHA` (40.0), `ALS_MAX_ITER` (15) |
| Outputs | `TOP_N` (50), `QDRANT_URL` (http://localhost:6333), `QDRANT_ITEM_COLLECTION`, `QDRANT_USER_COLLECTION`, `REDIS_HOST` (localhost), `REDIS_PORT` (6379), `REDIS_PASSWORD` (empty), `REDIS_DATABASE` (0), `RECS_CACHE_PREFIX` (recs), `RECS_SCHEMA_VERSION` (v1), `RECS_CACHE_TTL_SECONDS` (172800) |
| Two-Tower | `ENABLE_TWO_TOWER` (false), `QDRANT_TWO_TOWER_COLLECTION` (item_two_tower_vectors), `TWO_TOWER_DIM` (32) |
| Gate | `PROMOTION_PRIMARY_METRIC` (ndcg@10), `PROMOTION_MIN_RELATIVE_IMPROVEMENT` (0.01), `PROMOTION_MIN_COVERAGE_RATIO` (0.8), `PROMOTION_FORCE` (false), `MODEL_VERSION` (empty) |

Default event weights: impression 0.5, view 1, click 2, view_cart 2.5, add_to_cart 5,
add_shipping_info 6, add_payment_info 7, begin_checkout 8, purchase 10 (`EVENT_WEIGHTS_JSON`
overrides the whole map).

## 6. Run locally

**Primary (root compose)**, from the workspace root:

```bash
docker compose up -d --build        # stack incl. qdrant, redis, team-analytics (exports the Parquet every 300 s)
docker compose --profile jobs run --rm platform-recsys
```

The job service (`docker-compose.services.yaml`) builds `./platform-recsys`, mounts `analytics_data`
at `/data:ro`, and sets `WAREHOUSE_DRIVER`, `WAREHOUSE_PARQUET_PATH`, `QDRANT_URL=http://qdrant:6333`,
`REDIS_HOST=redis`, `REDIS_PORT`, and depends on `qdrant` and `redis`. Run order: tracking events reach
team-analytics, its export writes the Parquet, then the job. Until the job has run and been promoted,
`team-ai` Recommend answers empty.

**Standalone**

```bash
make sample SAMPLE=./data/tracking_events.parquet      # tiny synthetic Parquet warehouse
make run-local SAMPLE=./data/tracking_events.parquet   # needs PySpark, Java, local Qdrant + Redis
# or: docker compose -f docker-compose.local.yaml run --rm recsys-train   (external network platform-core_default)
make docker-build && make docker-run
```

## 7. Build, test and lint

| Command | Does |
|---|---|
| `make compile` | byte-compile `recsys sample_data tests` |
| `make lint` | `ruff check` + `black --check` (line length 110) |
| `make check-env` | `.env.example` drift gate |
| `make test-host` | PySpark-free tests (config, weights, recommend, env drift, evals, registry, nearline, ranker, point id) |
| `make test` | full pytest; Spark-gated tests skip without PySpark and run in the image |
| `make eval` | `python -m recsys.evals` offline evaluation CLI on built-in sample interactions |

CI (`.github/workflows/ci.yaml`, Python 3.10): install `requirements-dev.txt` + `pip install -e .`,
then `make compile`, `make lint`, `make check-env`, `make test-host`, `make eval`. Run those five
locally before pushing.

Runtime image: `python:3.11-slim-bookworm` + `openjdk-17-jre-headless` (PySpark 3.5.1 bundles Spark;
Java is required; the former bitnami/spark base is gone). `pyproject.toml` requires Python >= 3.10.

## 8. Spec and verification

- `FEATURES.yaml` lists 7 features: `recsys.als-training` (status `not-testable`; batch job, verified
  at the data layer by the platform-e2e job flow) and six `automated` ones (`eval-metrics`,
  `eval-holdout-gate`, `promotion-gate-reject`, `serving-isolation`, `bootstrap-registry`,
  `run-summary-observability`), covered by `recommendations/pipeline_eval_registry.feature` in
  `platform-e2e`.
- Coverage is verified from the workspace root with `make -C platform-e2e features-check` and, for a
  change, `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec in the root `openspec/changes/<id>` (this repo has no `openspec/` of its
  own), per the root README's ASDLC.

## 9. Gotchas

- Java must be on `PATH` for PySpark (the image has it).
- `data/` and `*.parquet` are gitignored; generate a sample with `make sample`.
- Collection and key names are a contract with `team-ai`; change them on both sides.
- Nothing is published unless the run is `promoted`. After resetting Qdrant/Redis while the registry
  still names a champion, the next run may be rejected or skipped: use `PROMOTION_FORCE=true` once.
- The registry falls back to in-memory when Redis is unreachable at start (logged at info level).
- The `push` path filter in `.github/workflows/ci.yaml` names `.github/workflows/ci-recsys.yaml`,
  which is not this file's name.

## 10. Known gaps

- `recsys/ranker/` (CVR / eGMV), `recsys/monitoring/` (PSI drift) and `recsys/nearline/` are library
  modules with unit tests; `pipeline.py` does not call them. There is no online reranker here and
  `lightgbm` is not a dependency.
- Two-Tower is a placeholder: it runs only when `ENABLE_TWO_TOWER=true` and only after a promoted
  run, and feeds the item tower hard-coded `price=100`, `popularity=1.0`, `category_id="general"` for
  every item. It is not evaluated or gated.
- With Redis unreachable the in-memory registry has no champion, so every run is a bootstrap and the
  gate is bypassed; the run then still attempts to publish to Redis.
- A user with fewer than two distinct listings is never a test user; with no test users the run is
  `skipped` and nothing is published, so very small datasets never publish.

## 11. Links

- Root rules: [`../AGENTS.md`](../AGENTS.md)
- [ADR-0014 Model registry and promotion gate](../platform-core/docs/ADR/0014-model-registry-promotion.md),
  [ADR-0011 Model serving](../platform-core/docs/ADR/0011-model-serving.md)
- OpenSpec: `../openspec/changes/` (`serve-trained-recs-locally`, `add-recsys-offline-eval`; archived
  `2026-09-20-wire-pipeline-eval-registry`)
