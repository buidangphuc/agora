# platform-recsys

Offline recommendation training job (bounded context: recommendation model artifacts). It reads
the governed `als_interactions` dataset, trains an implicit-feedback ALS model with PySpark, evaluates it on a
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
| Governed dataset `als_interactions@v1` | Written by `platform-featurestore` (`python -m featurestore dataset`) as `<DATASET_DIR>/as_of=<YYYYMMDDTHHMMSSZ>.parquet` plus `as_of=<stamp>.manifest.json`; the root compose mounts the `featurestore_data` volume read-only at `/features` | Columns `user_key`, `listing_id`, `weight` (double), `interactions` (int), `last_occurred_at` (timestamp). Manifest keys: `name`, `version`, `as_of`, `window_days`, `rows`, `users`, `items`, `definition_sha256`, `input_watermark`, `file_sha256`, `file`. |

Resolution (`recsys/dataset.py`): `DATASET_PATH` (an explicit file; its manifest is the same name with
`.parquet` replaced by `.manifest.json`) wins, otherwise the lexically latest `as_of=*.parquet` under
`DATASET_DIR` that has a manifest. **With no dataset the job exits 2**, logs `no governed dataset under
DATASET_DIR=...`, and registers nothing; there is no fallback to raw events.

Triples: `user_key`, `listing_id` and `weight` are used **as given** (identity stitching, event weights,
favourites and reviews are the featurestore's job). Only rows with an empty key or a weight <= 0 are
dropped. The offline evaluation split uses `last_occurred_at`.

**Lineage:** every registered model carries `parameters["dataset"] = {name, version, as_of, sha256}`
copied from the manifest (`sha256` is the manifest's `file_sha256`), and the run summary repeats it.

**Produces (only for a promoted run)**

| Store | Name | Content |
|---|---|---|
| Qdrant | `item_als_vectors__<model_version>` | One generation's L2-normalised item factors, cosine, dim = `ALS_RANK`. Payload `listing_id`, `model_version`, `updated_at`. Created fresh by each publish. |
| Qdrant | `user_als_vectors__<model_version>` | Same for users; payload `user_key`. |
| Qdrant | alias `QDRANT_ITEM_COLLECTION` (`item_als_vectors`) | Alias, not a collection: points at the serving generation's item collection. Deprecated compatibility shim (serving-switch-atomicity): team-ai names `<alias>__<recs:v1:serving>` itself, so one pointer decides Redis and Qdrant; the alias is only for readers that predate that and for a deployment with no pointer yet. |
| Qdrant | alias `QDRANT_USER_COLLECTION` (`user_als_vectors`) | Same for users. |
| Qdrant | `QDRANT_TWO_TOWER_COLLECTION` (`item_two_tower_vectors`) | Only when `ENABLE_TWO_TOWER=true` (see Known gaps). |
| Redis | `recs:v1:gen:<model_version>:user:{user_key}` | JSON `[{listing_id, score}]`, capped at `TOP_N` |
| Redis | `recs:v1:gen:<model_version>:item:{listing_id}` | Similar items, same shape |
| Redis | `recs:v1:gen:<model_version>:popular` | Popularity fallback (summed weight), same shape |
| Redis | `recs:v1:serving` | The generation being served. No TTL. Moved last, by one Lua script |
| Redis | `recs:v1:previous` | The generation `serving` replaced (rollback target). No TTL. Same script |
| Redis | `recs:v1:model_version` | Mirrors `serving` (same script) for readers that predate generations |
| Redis | `recs:v1:user:{user_key}`, `:item:{listing_id}`, `:popular` | Unscoped compatibility copies of the latest generation. Written while `RECS_WRITE_LEGACY_KEYS=true` (default); to be removed in a follow-up release |
| Redis | `recs:model:champion`, `recs:model:meta:{model_version}` | Registry state (champion pointer, metadata JSON incl. metrics and status) |

All generation and unscoped cache keys carry TTL `RECS_CACHE_TTL_SECONDS` (172800 s); the three
pointers do not. Every run (promoted or not) refreshes the TTL of the serving and previous generations
so a rollback never lands on expired keys. Qdrant point id is
`uuid5(namespace, source_id)` (`recsys/load/qdrant.py`), pinned by `tests/test_point_id.py`; consumers
depend on it. After upserting, points whose `model_version` differs from the current one are deleted.

Producer/consumer agreements that must hold: `QDRANT_ITEM_COLLECTION` = team-ai `RECS_QDRANT_COLLECTION`;
`RECS_CACHE_PREFIX` / `RECS_SCHEMA_VERSION` = team-ai `RECS_CACHE_PREFIX` / `RECS_CACHE_SCHEMA_VERSION`.

## 2. Events

The batch job produces and consumes none (the dataset reaches it as a file on a shared volume). The separate
**nearline consumer** (`python -m recsys.nearline`, change `add-recsys-nearline-signals`) consumes Kafka
`analytics.events` as consumer group `platform-recsys-nearline` and keeps the `recs:nearline:*` Redis keys
(recents, category affinity, co-views, position-debiased CTR; 24 h TTL, not generation-scoped) fresh for
team-ai. It is a long-running process, not part of `python -m recsys`; the key layout team-ai reads is in
`openspec/changes/add-recsys-nearline-signals/design.md`. Offsets are committed after the Redis write; a
Redis failure exits 1 and the restart replays (idempotent by event id). Settings: `KAFKA_BROKERS`,
`KAFKA_ANALYTICS_TOPIC`, `NEARLINE_CONSUMER_GROUP`, `NEARLINE_START_OFFSET`, `NEARLINE_TTL_SECONDS`,
`NEARLINE_IDLE_EXIT_SECONDS`.

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
| `promoted` | Structural gate passed and: no champion yet (first run bootstraps), or champion metadata missing, or protocol mismatch, or the gate passes, or `PROMOTION_FORCE=true` | Registered as `champion`, published as a generation (summary adds `serving`, `previous`), optional Two-Tower stage |
| `rejected` | Structural gate fails (`gate: structural`), or the metric gate fails against the same-protocol champion | Candidate marked `rejected`; the serving generation is untouched, nothing published |

Gate (`ModelEvaluator.compare_models`): primary metric `PROMOTION_PRIMARY_METRIC` (`ndcg@10`) must
improve by at least `PROMOTION_MIN_RELATIVE_IMPROVEMENT` (0.01) and, when both runs report it,
`coverage@10` must stay at least `PROMOTION_MIN_COVERAGE_RATIO` (0.8) of the champion's. Summary keys:
`model_version`, `decision`, `reason`, `primary_metric`, `candidate_value`, `incumbent_version`,
`incumbent_value`, `metrics`, `items`, `users`, `popular`, plus `qdrant`/`cache` counts (and
`two_tower_items`) when published. See ADR-0014.

**`PROMOTION_FORCE=true`** promotes and publishes the run even if the gate would reject it. Use it to
repopulate Qdrant/Redis after a reset. Never set it on a schedule. The first run with an empty
registry bootstraps on its own.

**Structural gate** (`recsys/structural_gate.py`, spec `recsys-generations`) runs on the candidate's
own Top-N lists and factors *before* evaluation and the metric gate. The first failing check rejects:

| Check | Rejects when | Setting (default) |
|---|---|---|
| Factors | any NaN or infinite value in the user or item factors | none, always on |
| User coverage | users with at least one recommendation / dataset users is below | `GATE_MIN_USER_COVERAGE` (0.5) |
| Item coverage | distinct items in any user's top-N / dataset items is below | `GATE_MIN_ITEM_COVERAGE` (0.05) |
| List overlap | mean Jaccard of users' top-N lists (all pairs, or a deterministic sample of 500) is above | `GATE_MAX_LIST_OVERLAP` (0.9) |

A rejection registers the model with status `rejected`, the reason in `metrics.gate_reason` (a string,
like `eval_protocol`; also mirrored in `parameters.gate_reason`), returns `decision: rejected` with
`gate: structural`, and publishes nothing. `PROMOTION_FORCE` does not bypass it. A catalogue no larger
than `TOP_N` gives every user the same list, so it trips the overlap check; lower `TOP_N` or raise
`GATE_MAX_LIST_OVERLAP` for such toy datasets.

**Generations.** A promoted model is published as one generation (its `model_version`):
1. its Redis keys and Qdrant collections are written (invisible to readers);
2. the Qdrant aliases move to them, then one Lua script sets `recs:v1:previous` to the old `serving`,
   `recs:v1:serving` to the new one and mirrors `recs:v1:model_version` (the switch);
3. every generation that is neither serving nor previous is deleted (keys and collections).
A crash before step 2 leaves serving as it was. The first publish on a deployment with plain
`item_als_vectors` / `user_als_vectors` collections copies them to `<name>__legacy`, deletes them and
lets the aliases take the names; retention drops `__legacy` after the first switch.

**Rollback**: `python -m recsys rollback` swaps `serving` and `previous`, moves the aliases to the
restored generation's collections and makes its model the registry champion (the demoted model becomes
`archived`). Exit 0 on success; exit 2, changing nothing, when there is no previous generation or its
keys/collections are gone; exit 1 when Redis is unreachable. Running it twice goes forward again. The
next training run compares against the restored champion.

`MODEL_VERSION` overrides the stamp; otherwise `als-<UTC yyyymmddThhmmssZ>`.

**Drift** (`recsys/monitoring/generation.py`, change `add-recsys-drift-monitoring`). Before the structural
gate every run summarises four distributions as quantile sketches (`weight` of the dataset pairs,
`user_items`, `item_users`, and each user's best recommendation score `top_score`), computes PSI against the
champion's stored sketches (the generation it would replace) and records the verdict: `parameters.distribution`
(the sketches, the next run's baseline), `parameters.drift` (`status` no_baseline/ok/drifted, `baseline_version`,
per-feature `psi` and `drift_level`, `is_drifted`), `metrics.drift_psi_max`, the summary's `drift`, a log line
(WARNING when flagged) and, with `DRIFT_METRICS_PATH`, a Prometheus text file. A feature is flagged at
`DRIFT_ALERT_THRESHOLD` (0.25). It is observational: it never changes `decision`. ALS factors are not compared
(they are only defined up to a rotation).

## 5. Configuration

All settings are read in `recsys/config.py` (`_FIELDS`, the single source of truth); `.env.example`
mirrors it. `make check-env` (`tests/test_env_drift.py`) fails if the two drift, in either direction.

| Group | Variables (default) |
|---|---|
| Runtime | `ENV` (local), `LOG_LEVEL` (info) |
| Spark | `SPARK_MASTER` (local[*]), `SPARK_APP_NAME` (platform-recsys-als) |
| Dataset | `DATASET_DIR` (/features/datasets/als_interactions/v1), `DATASET_PATH` (empty; an explicit file, overrides `DATASET_DIR`) |
| Interactions | `MIN_INTERACTIONS_PER_USER` (1), `MIN_INTERACTIONS_PER_ITEM` (1) |
| ALS | `ALS_RANK` (64), `ALS_REG_PARAM` (0.05), `ALS_ALPHA` (40.0), `ALS_MAX_ITER` (15) |
| Outputs | `TOP_N` (50), `QDRANT_URL` (http://localhost:6333), `QDRANT_ITEM_COLLECTION`, `QDRANT_USER_COLLECTION`, `REDIS_HOST` (localhost), `REDIS_PORT` (6379), `REDIS_PASSWORD` (empty), `REDIS_DATABASE` (0), `RECS_CACHE_PREFIX` (recs), `RECS_SCHEMA_VERSION` (v1), `RECS_CACHE_TTL_SECONDS` (172800) |
| Two-Tower | `ENABLE_TWO_TOWER` (false), `QDRANT_TWO_TOWER_COLLECTION` (item_two_tower_vectors), `TWO_TOWER_DIM` (32) |
| Nearline | `KAFKA_BROKERS` (localhost:19092), `KAFKA_ANALYTICS_TOPIC` (analytics.events), `NEARLINE_CONSUMER_GROUP` (platform-recsys-nearline), `NEARLINE_START_OFFSET` (latest), `NEARLINE_TTL_SECONDS` (86400), `NEARLINE_IDLE_EXIT_SECONDS` (0 = run until stopped) |
| Drift | `DRIFT_ALERT_THRESHOLD` (0.25), `DRIFT_METRICS_PATH` (empty: no Prometheus file) |
| Gate | `PROMOTION_PRIMARY_METRIC` (ndcg@10), `PROMOTION_MIN_RELATIVE_IMPROVEMENT` (0.01), `PROMOTION_MIN_COVERAGE_RATIO` (0.8), `PROMOTION_FORCE` (false), `MODEL_VERSION` (empty) |
| Structural gate | `GATE_MIN_USER_COVERAGE` (0.5), `GATE_MIN_ITEM_COVERAGE` (0.05), `GATE_MAX_LIST_OVERLAP` (0.9) |
| Compatibility | `RECS_WRITE_LEGACY_KEYS` (true): also write the unscoped `recs:v1:{user,item,popular}` keys |

Removed with the raw-events ALS path: `WAREHOUSE_PARQUET_PATH`, `INTERACTION_WINDOW_DAYS`,
`EVENT_WEIGHTS_JSON` (the weights and window now live in the featurestore dataset definition), plus
the now-unused `WAREHOUSE_DRIVER`, `BIGQUERY_*` and `RECENCY_HALF_LIFE_DAYS`. `recsys/weights.py`
stays for the nearline and two-tower helpers.

## 6. Run locally

**Primary (root compose)**, from the workspace root:

```bash
docker compose up -d --build        # stack incl. qdrant, redis, team-analytics
# build the dataset first (platform-featurestore `dataset` command), then:
docker compose --profile jobs run --rm platform-recsys
```

The job service (`docker-compose.services.yaml`) builds `./platform-recsys`, mounts `featurestore_data`
at `/features:ro`, and sets `DATASET_DIR`, `QDRANT_URL=http://qdrant:6333`, `REDIS_HOST=redis`,
`REDIS_PORT`, and depends on `qdrant` and `redis` (compose wiring is owned by the root change). Run
order: the featurestore builds the dataset, then the job. Until the job has run and been promoted,
`team-ai` Recommend answers empty.

**Standalone**

```bash
make sample SAMPLE=./data/datasets/als_interactions/v1      # tiny governed dataset (parquet + manifest)
make run-local SAMPLE=./data/datasets/als_interactions/v1   # needs PySpark, Java, local Qdrant + Redis
# or: docker compose -f docker-compose.local.yaml run --rm recsys-train   (external network platform-core_default)
make docker-build && make docker-run
```

## 7. Build, test and lint

| Command | Does |
|---|---|
| `make compile` | byte-compile `recsys sample_data tests` |
| `make lint` | `ruff check` + `black --check` (line length 110) |
| `make check-env` | `.env.example` drift gate |
| `make test-host` | PySpark-free tests (config, dataset, weights, recommend, env drift, evals, registry, nearline, ranker, point id, structural gate, generations). The Lua pointer tests need `fakeredis` + `lupa` (in `requirements-dev.txt`) and skip without them |
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
- The registry champion is set before the publish step: a crash mid-publish leaves serving untouched
  but the registry naming the unpublished model champion. Re-run with `PROMOTION_FORCE=true` or
  `python -m recsys rollback` once the stores are healthy.
- Nothing is published unless the run is `promoted`. After resetting Qdrant/Redis while the registry
  still names a champion, the next run may be rejected or skipped: use `PROMOTION_FORCE=true` once.
- The registry falls back to in-memory when Redis is unreachable at start (logged at info level).
- The `push` path filter in `.github/workflows/ci.yaml` names `.github/workflows/ci-recsys.yaml`,
  which is not this file's name.

## 10. Known gaps

- `recsys/ranker/` (CVR / eGMV) is a library module with unit tests; `pipeline.py` does not call it. `recsys/nearline/` runs as its own process
  (section 2); its compose service is proposed in the change's `design.md`, not yet in the root compose. There is no online reranker here and
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
