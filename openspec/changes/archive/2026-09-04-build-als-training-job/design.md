## Context

The behavioral warehouse (`team-analytics`, `build-warehouse-writer`) holds one flat, append-only
`tracking_events` table (DuckDB/Parquet local, BigQuery prod) whose columns are the canonical
`warehouse.Schema`: `event_id, event_type, listing_id, session_id, anonymous_id, page_path,
referrer, position, search_query, occurred_at, principal_id, principal_type, properties`. This is
implicit-feedback interaction data — no explicit ratings, just views/clicks/add-to-carts.

`add-recommendation-contract` defines `RecommendationService/Recommend`, whose response carries a
`model_version` and a ranked list of `listing_id`+`score`. Something must **produce** the model those
scores come from. `team-ai` already runs a **Qdrant** vector store (`RAG_QDRANT_URL` default
`http://localhost:6333`, `QdrantClient`/`QdrantVectorStore` in `app/modules/ai/rag/factory.py`) and a
**Redis** client (`app/core/redis.py`, `:6379`); the online recommender will read from the same
infra. This change is the offline half: a PySpark job that turns warehouse interactions into ALS
factors and loads them into Qdrant + Redis.

It is a **new bounded context** (AGENTS.md §6(c)): it owns new artifacts (its Qdrant collections, its
Redis keyspace) and its own lifecycle (a scheduled batch run), so it is a new repo `platform-recsys`,
not a feature on an existing service.

## Decisions

- **Offline batch, not online.** ALS matrix factorization is a full-corpus optimization over the
  whole interaction matrix — minutes of Spark compute over the rolling window, not a per-request
  operation. So the model is trained offline on a cadence and its results are **precomputed** into
  low-latency stores (Qdrant ANN + Redis cache); the online RPC (`serve-recommendations-teamai`) only
  does a vector query / cache lookup. Day-fresh recommendations are acceptable for this surface, and
  the split keeps heavy compute off the request path (Rule 5 spirit: background work runs off-path).

- **Input: warehouse → implicit-feedback triples.** Read `tracking_events` over a rolling window
  (e.g. last 30 days by `occurred_at`), drop rows with empty `listing_id`, and collapse to
  `(user_id, item_id, weight)`:
  - `user_id` = `principal_id` when the actor was authenticated, else `anonymous_id` (matches how a
    `RecommendRequest` identifies a caller: user xor anonymous);
  - `item_id` = `listing_id`;
  - `weight` = a confidence from `event_type` — e.g. `impression=0.5, view=1, click=2, add_to_cart=5`
    — summed per (user,item), optionally recency-decayed by `occurred_at`. ALS string ids are indexed
    to integer factor ids with a `StringIndexer`, and the reverse maps are kept to label the outputs.
  - Local reads DuckDB-exported Parquet directly (`spark.read.parquet(<warehouse path>)`); prod reads
    the BigQuery table via the Spark BigQuery connector. The read path is the only driver-specific
    seam — everything downstream operates on the same DataFrame of triples.

- **ALS approach + params (Spark MLlib).** `pyspark.ml.recommendation.ALS` with `implicitPrefs=true`
  (no explicit ratings), `ratingCol="weight"`, starting hyperparameters `rank≈64`, `regParam≈0.05`,
  `alpha≈40`, `maxIter≈15`, `coldStartStrategy="drop"`, `nonnegative=false`. These are the tunable
  knobs exposed as job env/config; they are defaults, not final tuned values.

- **Two outputs: item-item similarity + user embeddings.**
  - **Item factors** (`model.itemFactors`, dim = `rank`) are the item embeddings; item-item
    similarity is nearest-neighbor over them (cosine/dot). Used for PDP "similar items" (the
    `seed_listing_id` / `RECOMMENDATION_CONTEXT_SIMILAR_ITEMS` path) and as candidates when a user has
    no vector.
  - **User factors** (`model.userFactors`, dim = `rank`) are the user embeddings; a user's
    recommendations are the top-N item factors by dot product with the user vector (the "for-you"
    homepage path).

- **Qdrant collection layout (`:6333`).** Two collections, both vector dim = `rank`, distance =
  `Cosine` (ALS scores are dot-product-ranked; vectors L2-normalized on upsert so cosine≈dot order):
  - `recsys_item_factors` — one point per listing; id derived from `listing_id`; payload
    `{ listing_id, model_version, updated_at }`. Similar-items = ANN query with the seed's vector.
  - `recsys_user_factors` — one point per user/anonymous id; payload `{ user_key, model_version,
    updated_at }`. For-you = ANN query of this vector against `recsys_item_factors`.
  Each batch upserts under a fresh `model_version`; the job writes into the live collections and
  prunes vectors whose `model_version` is stale (or swaps a versioned collection alias) so the online
  reader always sees one consistent generation.

- **Redis cache shape (`:6379`).** A precomputed top-N so the hot path avoids an ANN query entirely:
  - `rec:user:{user_key}` → the user's ranked `listing_id:score` list (JSON array or ZSET), capped at
    a configured N;
  - `rec:item:{listing_id}` → precomputed similar items for that listing (same shape);
  - `rec:model:version` → the current `model_version` string (the value the serving RPC echoes back).
  Keys carry a TTL a bit longer than the batch cadence so a missed run degrades gracefully rather than
  emptying the cache; the online reader falls back to a live Qdrant query on a cache miss.

- **Batch cadence / orchestration.** Nightly (daily) rebuild over the rolling window, run as a
  `platform-gitops` **`CronJob`** (mirrors the `team-analytics` worker-variant registration precedent,
  but scheduled rather than long-running). The job is idempotent per `model_version` (re-running the
  same window reproduces the same artifacts), so a retried/failed run is safe.

- **Local-dev without a Spark cluster.** The job runs in Spark **local mode** (`--master local[*]`,
  or an in-process `SparkSession.builder.master("local[*]")`) — no cluster, no YARN/k8s executors.
  It reads the DuckDB-exported Parquet from the local warehouse volume and writes to the local Qdrant
  (`:6333`) and Redis (`:6379`) already in `platform-core`'s compose. A tiny sample interaction set
  makes the whole train→load→assert loop runnable on a laptop and in CI.

## Risks / Trade-offs

- **Cold start.** New users/listings with no interactions get no ALS factor (`coldStartStrategy=drop`).
  Mitigation is a popularity/recency fallback in the serving layer (out of scope here); the batch job
  simply omits them, and item-item similarity still covers a freshly-seen listing once it has views.
- **Anonymous identity churn.** `anonymous_id` is a cookie/device id that rotates; user vectors for
  anonymous keys are noisier and shorter-lived. Accepted — the TTL'd Redis cache and the item-item
  path both degrade gracefully; authenticated `principal_id` gives the stronger signal.
- **Sparse / small warehouse early on.** Until enough behavior accrues, ALS factors are weak. The job
  is safe to run on sparse data (it just produces a low-signal model); quality improves as the
  warehouse fills. A minimum-interactions guard can skip a listing/user below a threshold.
- **Offline/online skew.** The scores served online must match what was trained offline; the
  `model_version` stamped on every Qdrant point and the `rec:model:version` Redis key make the served
  generation explicit and debuggable (and flow into the RPC's `model_version` response field).
- **Batch staleness.** Recommendations are up to a cadence old. Acceptable for this surface;
  shortening the cadence trades compute for freshness, and streaming/incremental training is an
  explicit non-goal.
- **Local DuckDB→Parquet dependency.** Local dev reads Parquet the DuckDB adapter exports; if the
  warehouse volume layout changes, the job's local reader must follow. The read path is deliberately
  isolated as the one driver-specific seam to contain that coupling.
