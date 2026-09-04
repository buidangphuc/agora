# platform-recsys

Offline **PySpark ALS** training job — the *producer* half of the recommendation
system. It reads the behavioral warehouse that `team-analytics`
(`build-warehouse-writer`) fills, learns latent factors with implicit-feedback
matrix factorization, and loads the results into the low-latency stores the
online serving layer (`serve-recommendations-teamai`) reads: **Qdrant** (`:6333`)
for item/user vectors and **Redis** (`:6379`) for a precomputed Top-N cache.

It is a **new bounded context** (AGENTS.md §6(c)): it owns its own artifacts and
lifecycle (a scheduled batch run), reads only the warehouse and writes only its
own stores (Rule 3), and has **no request-serving surface** — a `platform-gitops`
**CronJob** runs it nightly.

## Pipeline

```
tracking_events (warehouse)                     warehouse.py   (reader seam: parquet local / BigQuery prod)
  → (user, item, weight) implicit triples       interactions.py (event→weight, principal|anonymous, drop empty listing)
  → StringIndexer → integer factor ids          interactions.py
  → Spark MLlib ALS (implicitPrefs=true)        train.py       (itemFactors + userFactors, dim = ALS_RANK)
  → L2-normalized vectors + Top-N ranking       recommend.py   (numpy on the driver)
  → Qdrant upsert + stale-generation prune      load/qdrant.py
  → Redis precomputed cache                     load/redis_cache.py
```

`WAREHOUSE_DRIVER` is the only driver-specific seam: `duckdb` reads the
DuckDB-exported Parquet locally, `bigquery` reads the BigQuery table via the
Spark BigQuery connector. Everything downstream operates on the same DataFrame.

## Output contract (must match `serve-recommendations-teamai`)

**Qdrant** (`:6333`, distance `Cosine`, vectors L2-normalized, dim = `ALS_RANK`):

| collection (default)         | one point per | payload                              |
| ---------------------------- | ------------- | ------------------------------------ |
| `item_als_vectors`           | listing       | `{listing_id, model_version, updated_at}` |
| `user_als_vectors`           | user/anon key | `{user_key, model_version, updated_at}`   |

`item_als_vectors` is the consumer's `RECS_QDRANT_COLLECTION` default — same
literal both sides. Each run writes under a fresh `model_version` and prunes
points not stamped with it, so the reader sees one consistent generation.

**Redis** (`:6379`, keys `{RECS_CACHE_PREFIX}:{RECS_SCHEMA_VERSION}:...`, default
prefix `recs`, version `v1`; ids + scores only, no card hydration):

| key                             | value                                            |
| ------------------------------- | ------------------------------------------------ |
| `recs:v1:user:{user_key}`       | ranked `[{listing_id, score}]`, capped at `TOP_N`|
| `recs:v1:item:{listing_id}`     | precomputed similar items (same shape)           |
| `recs:v1:popular`               | popularity fallback list (cold-start floor)      |
| `recs:v1:model_version`         | the current `model_version` (echoed by the RPC)  |

Every key carries a TTL (`RECS_CACHE_TTL_SECONDS`, default 48h > nightly cadence)
so a missed run degrades gracefully rather than emptying the cache.

## Local development (no Spark cluster)

The job runs in Spark **local mode** (`local[*]`). With `platform-core`'s Qdrant
and Redis up:

```bash
make sample SAMPLE=./data/tracking_events.parquet   # tiny Parquet warehouse (pandas, no Spark)
make run-local SAMPLE=./data/tracking_events.parquet # needs PySpark installed
# or via Docker on platform-core's network:
docker compose -f docker-compose.local.yaml run --rm recsys-train
```

## Configuration

All env-driven — see [`.env.example`](.env.example). The `_FIELDS` table in
[`recsys/config.py`](recsys/config.py) is the single source of truth for loading
AND the `.env.example` drift gate (`make check-env`). Key knobs: `WAREHOUSE_DRIVER`
+ warehouse source, `INTERACTION_WINDOW_DAYS`, `EVENT_WEIGHTS_JSON`, the ALS
hyperparameters (`ALS_RANK`, `ALS_REG_PARAM`, `ALS_ALPHA`, `ALS_MAX_ITER`),
`TOP_N`, `QDRANT_URL`, `REDIS_HOST`/`REDIS_PORT`, `MODEL_VERSION`.

## Testing

```bash
make compile     # byte-compile everything (syntax gate; no PySpark needed)
make test-host   # PySpark-free tests: config, weights, ranking, .env drift
make test        # full suite; Spark/Qdrant/Redis tests auto-skip when not installed
```

Spark, Qdrant, and Redis are Docker/CI concerns — the PySpark tests
(`test_interactions_spark.py`, `test_pipeline_smoke.py`) `importorskip` and are
verified in CI, not on the host.

## Deployment

Runs as a nightly `CronJob` in `platform-gitops` (`platform/recsys/`), registered
as an ArgoCD `Application` — mirroring the `postgres-backup` CronJob precedent.
