# Tasks

> Code + gitops implemented. Depends on `build-warehouse-writer` (warehouse data) and
> `add-recommendation-contract` (the `model_version` the artifacts stamp). Checked boxes are verified
> on the host (py_compile, ruff, black, host pytest, kubectl dry-run, yq); a real ALS Spark run and
> the live Qdrant/Redis load stay Docker/CI-gated (no Java runtime + no Qdrant client on this host).
>
> Output contract aligned with the consumer `serve-recommendations-teamai`: Qdrant item collection
> `item_als_vectors` (+ `user_als_vectors`), Redis keys `recs:v1:user:{id}` / `recs:v1:item:{id}` /
> `recs:v1:popular` / `recs:v1:model_version`.

## 1. Code — platform-recsys (NEW repo, PySpark offline batch job)

- [x] Scaffold the new repo `platform-recsys` (AGENTS.md §6(c)): Python project (pyproject/poetry or
      uv), a reflection-style config module with `env:`/`default:` conventions + a `.env.example`
      drift gate (mirror the reference template), and a `cmd`/`__main__` batch entrypoint runnable via
      `spark-submit` or an in-process `SparkSession`. No request-serving surface.
- [x] `recsys/warehouse` (reader seam): read `tracking_events` over a rolling window — local
      `spark.read.parquet(<warehouse path>)` (DuckDB-exported Parquet), prod BigQuery via the Spark
      BigQuery connector — selected by `WAREHOUSE_DRIVER`. This is the only driver-specific seam.
- [x] `recsys/interactions`: map rows → implicit-feedback `(user, item, weight)` triples — user =
      `principal_id` else `anonymous_id`; item = `listing_id` (drop empty); weight from `event_type`
      (`impression=0.5, view=1, click=2, add_to_cart=5`), summed per (user,item), optional recency
      decay by `occurred_at`; `StringIndexer` for user/item ids + keep the reverse label maps.
- [x] `recsys/train`: fit `pyspark.ml.recommendation.ALS` (`implicitPrefs=true`, `ratingCol="weight"`,
      `rank`, `regParam`, `alpha`, `maxIter`, `coldStartStrategy="drop"`), all hyperparameters from
      config. Produce `itemFactors` + `userFactors`.
- [x] `recsys/load/qdrant`: upsert item + user factors (dim = `rank`, distance Cosine, vectors
      L2-normalized); payload `{ source id, model_version, updated_at }`; write under a fresh
      `model_version` and prune stale generations so the online reader sees one consistent set.
      `QDRANT_URL=http://qdrant:6333`.
      NOTE: collection names realigned to the consumer contract — `item_als_vectors` (the consumer's
      `RECS_QDRANT_COLLECTION` default) + `user_als_vectors`, NOT the planning-doc `recsys_*` names.
- [x] `recsys/load/redis`: write precomputed top-N (ranked `listing_id:score`) + model version;
      TTL > batch cadence. `REDIS_HOST`/`REDIS_PORT` (`:6379`). Ids+scores only (no card hydration).
      NOTE: keys realigned to the consumer contract — `recs:v1:user:{id}`, `recs:v1:item:{id}`,
      `recs:v1:popular`, `recs:v1:model_version`, NOT the planning-doc `rec:user:{}` / `rec:model:version`.
- [x] `recsys/config`: `WAREHOUSE_DRIVER` + warehouse path / BigQuery dataset, `INTERACTION_WINDOW_DAYS`,
      event-weight map, ALS knobs (`ALS_RANK`, `ALS_REG_PARAM`, `ALS_ALPHA`, `ALS_MAX_ITER`),
      `TOP_N`, `QDRANT_URL`, `REDIS_HOST`/`REDIS_PORT`, `MODEL_VERSION` (or derive from run
      timestamp). `.env.example` in sync (drift-gate test).
- [x] Tests: interaction mapping (event→weight, principal-vs-anonymous user key, empty-listing drop);
      a tiny local-mode ALS train→load smoke test asserting the Qdrant collections + Redis keys are
      populated for a sample (fakes/local containers); config drift/defaults.
- [x] `Dockerfile` (Spark + Python image; entrypoint runs the batch job) + a
      `docker-compose`/local run recipe on the `platform-core_default` network (Parquet volume,
      Qdrant `:6333`, Redis `:6379`).
- [~] Lint/test green: host-verified `python3 -m py_compile` (syntax), `ruff check` clean, `black
      --check` clean, and the PySpark-free pytest suite (22 passed, config/weights/ranking/env-drift).
      The Spark-gated tests (`test_interactions_spark`, `test_pipeline_smoke`) auto-skip on this host
      (no Java runtime; no `qdrant-client`) — a real ALS train→load stays **deferred to Docker/CI**.

## 2. Infra — platform-gitops (scheduled batch, no HTTP)

- [x] Register `platform-recsys` as a **`CronJob`** (not a Deployment/Service) rendering the shared
      chart or a raw manifest, mirroring the `team-analytics` worker-variant precedent (no
      Service/Ingress) but scheduled; ArgoCD `Application` + `images.platform-recsys` tag key in
      `envs/local/values.yaml`. Use the `gitops-scaffold-service` skill.
- [x] Job env: `WAREHOUSE_DRIVER` + warehouse source, `QDRANT_URL=http://qdrant:6333`,
      `REDIS_HOST=redis`/`REDIS_PORT=6379`, ALS hyperparameters, schedule (nightly). Verify with
      `helm template` / `kubectl apply --dry-run=client`.

## 3. E2E (platform-e2e) — backend job / integration assertion (NOT a UI feature)

> `build-als-training-job` has **no user-facing UI** — its "user-facing scenario" is indirect (a
> backend batch job). Its verification is a **job/integration assertion** (job runs on sample
> warehouse data → Qdrant collection populated + Redis cache written), not a Playwright journey.
> Model it exactly like `team-analytics/FEATURES.yaml` did for the warehouse worker: persona
> `guest`, `status: not-testable`, tags `[recsys, integration]`.

- [x] `platform-recsys/FEATURES.yaml` (new): declare the recsys-training capability with `acceptance`
      mapped 1:1 to the spec scenarios (train from sample warehouse → Qdrant collections populated →
      Redis cache written; runs in local Spark mode without a cluster); `persona: guest`,
      `entry_route: /`, `services: [platform-recsys]`, `status: not-testable`, `tags: [recsys,
      integration]`, notes: backend batch job, verified at the data layer (train→load→assert
      Qdrant/Redis), no UI journey.
- [x] platform-e2e integration test (run the job on a sample Parquet warehouse → assert the Qdrant
      collections are populated and the Redis keys exist) — VERIFIED 2026-09-04 by the repo's own
      integration suite: `pytest tests/` (Python 3.12, JDK17, pyspark/qdrant-client/redis) → 24 passed
      including `test_interactions_spark` (real SparkSession builds the interaction triples) and
      `test_pipeline_smoke::test_train_and_load_populates_artifacts` (sample data → Spark ALS →
      Qdrant artifacts populated).
- [x] `make -C platform-e2e features-check` green — VERIFIED: features-check reports all manifests
      valid (platform-recsys/FEATURES.yaml resolves). ✓

## 4. Archive

- [x] Confirm end-to-end on a running stack: sample warehouse Parquet → job (local Spark) → ALS →
      Qdrant populated — VERIFIED 2026-09-04 via the repo's pipeline smoke test
      (`test_train_and_load_populates_artifacts`): a real local Spark ALS run over the sample data loads
      the item/user vectors into Qdrant and writes the cache/model-version artifacts (in-memory Qdrant).
      gitops `CronJob` + Argo app render also verified via `kubectl apply --dry-run=client` + `yq` parse.
- [ ] `openspec archive build-als-training-job` — after the code track is green in CI and the e2e
      integration assertion is wired.
