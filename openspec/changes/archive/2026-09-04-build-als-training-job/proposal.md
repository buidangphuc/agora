## Why

`build-warehouse-writer` sinks `platform.analytics.v1.TrackingEvent`s into the behavioral warehouse
(DuckDB/Parquet local, BigQuery prod) via `team-analytics/internal/warehouse/*`, and
`add-recommendation-contract` defines the `platform.recommendation.v1.RecommendationService/Recommend`
RPC the online path will implement. Between them there is a missing producer: something has to turn
warehouse behavior into recommendation artifacts the online service can serve with low latency.

This change builds that producer — an **offline PySpark batch job** that reads the behavioral
warehouse, runs **ALS collaborative filtering** (implicit feedback) to learn latent factors, and
loads the results into the infra the online serving layer already uses: **Qdrant** (`:6333`) for
item and user vectors and **Redis** (`:6379`) for a precomputed top-N cache. It is a new bounded
context that owns its own lifecycle (a scheduled batch job, its own model artifacts), so per
AGENTS.md §6(c) it is a **new repo**, `platform-recsys`. Producing the model offline and serving it
online is the standard split for matrix-factorization recommenders — see design.md.

## What Changes

- **platform-recsys** (NEW repo / bounded context, AGENTS.md §6(c)) — a **PySpark batch job** (no
  request-serving surface; run by a scheduler, not behind the gateway). It:
  - reads the behavioral warehouse produced by `team-analytics` — local: the DuckDB-exported
    **Parquet** `tracking_events` files on the warehouse volume; prod: the **BigQuery**
    `tracking_events` table — over a rolling interaction window;
  - maps `tracking_events` rows to implicit-feedback `(user, item, weight)` triples (user =
    `principal_id` when present, else `anonymous_id`; item = `listing_id`; weight derived from
    `event_type`), then fits **Spark MLlib ALS** (`implicitPrefs=true`) to produce **item factors**
    (→ item-item similarity) and **user factors** (→ user embeddings);
  - loads item vectors and user vectors into **Qdrant** collections (`:6333`, the same instance
    `team-ai` already points `RAG_QDRANT_URL` at) and writes a **precomputed top-N recommendation
    cache** into **Redis** (`:6379`), stamped with a `model_version`;
  - depends on `build-warehouse-writer` for the warehouse data and on `add-recommendation-contract`
    for the `model_version` field the artifacts populate.
- **platform-gitops** — register `platform-recsys` as a **scheduled batch job** (a `CronJob`, not a
  Deployment/Service) that runs the training job on a cadence, mirroring the worker-variant
  precedent used for `team-analytics` (Deployment-only, no Service/Ingress) but as a `CronJob`. The
  `gitops-scaffold-service` skill covers this scaffold. Job env points at the warehouse source
  (`WAREHOUSE_DRIVER`/path or BigQuery dataset), `QDRANT_URL=http://qdrant:6333`,
  `REDIS_HOST=redis`/`REDIS_PORT=6379`, and ALS hyperparameters.

### Architecture compliance

- **Rule 3 (DB-per-service):** `platform-recsys` reads only the analytics warehouse (an already-
  published analytical store, self-contained like `team-search`'s replay of `listing.events`) and
  writes only its own artifact stores (its Qdrant collections + its Redis keyspace). It joins no
  service's operational Postgres and calls no service over gRPC.
- **Rule 4 (contract SoT):** no proto change. It **consumes** `add-recommendation-contract` only to
  align the `model_version` provenance the serving RPC returns; platform-core is untouched here.
- **Rule 5 (right tool):** this is a scheduled **batch job** (offline model production), not a
  Kafka/RabbitMQ message path — it is orchestrated by a scheduler (`CronJob`), consistent with
  "background jobs" living off the request path.

## Non-goals

- **Online serving of recommendations** — implementing `RecommendationService/Recommend` (reading
  these Qdrant/Redis artifacts and returning rankings) is `serve-recommendations-teamai`, not here.
- **Real-time / streaming training** — no online/incremental model updates; this is a periodic
  full/rolling-window batch rebuild only.
- **A feature store** — no online feature service or feature registry; artifacts are the ALS
  factors + a precomputed cache, nothing more.
- **Any change to the producer** (`emit-tracking-events`), the **warehouse writer**
  (`build-warehouse-writer`), or the **contract** (platform-core).
- **Card hydration / listing content** — artifacts hold listing ids + scores only; hydration is the
  serving layer's job (Rule 3).
