## Why

`plans/ai-first/PROGRAM.md` (change C2) makes `team-analytics` the platform feature store: models read
declared features, never raw events. Today there is no feature layer at all. Verified in code (2026-10-01):

- `team-analytics` appends `TrackingEvent` rows to one DuckDB table (`tracking_events`) and serves two seller
  dashboard RPCs. Nothing turns those rows into model inputs. `ExportParquet` is a library method nothing
  calls; in compose the DuckDB file sits on `/tmp` (not the mounted volume) and in `platform-gitops` on an
  `emptyDir`, so every restart loses the history.
- Feature logic lives inside a consumer: `platform-recsys/recsys/interactions.py` re-implements the
  event-to-weight map (`weights.py`, impression 0.5 / view 1 / click 2 / add_to_cart 5) and its recency decay
  uses `current_timestamp()`, so the same training run gives different numbers on different days and nothing
  stops a label window from leaking into features.
- Serving (`team-ai` `Recommend`) reads only model outputs (`recs:v1:*`, Qdrant). It has no way to read a
  fresh signal such as "trending in the last hour" or "this visitor's last items", and
  `plans/mlops` P2-T4 (nearline) would give `platform-recsys` a second, separate implementation of the same
  counters in a `recs:` namespace.

Every planned model (ALS now; the ranker, two-tower, search learning-to-rank, assistant personalization and
demand forecasting later, PROGRAM "Later") needs the same item and user signals. Without one definition,
each will re-derive them and drift between training and serving. That is why this is not YAGNI here: the
classic caveat ("single model, simple features, small team: a well-built pipeline is enough", see the
`Feature Store` note) does not hold once a second consumer arrives, and C3 plus the `plans/mlops` phase 3
already name several.

## What Changes

- **team-analytics** (the bulk; capability owner):
  - A machine-readable **feature registry** `team-analytics/features/registry.yaml` with a JSON schema and a
    CI validator: name, version, entity, dtype, SQL definition, source events, window, aggregation, TTL,
    owner, freshness SLO, PII class, status (`experimental` / `stable` / `deprecated`). A published version's
    SQL is immutable; any change that alters values is a new version. Deprecation has a sunset date.
  - **One definition, two materializations.** Each feature version is one DuckDB SQL query parameterized by
    an `as_of` time. A **batch materializer** writes offline Parquet partitions plus a manifest to object
    storage (MinIO locally, S3-compatible when deployed). A **micro-batch materializer** runs the same SQL
    every 60 s for the entities whose window changed and writes `fs:v1:<entity>:<id>` hashes in
    valkey/Redis. A **parity test** (offline point-in-time value at t equals the online value written at t,
    for sampled entities) is a hard CI gate.
  - A **dataset builder**: a consumer-owned view file (`platform-recsys/feature_views/als_interactions@v1.yaml`)
    requests a point-in-time training set; it is built with a DuckDB `ASOF JOIN` on what was knowable at
    each label time and written as Parquet plus a manifest.
  - A **`FeatureService`** gRPC server (`GetOnlineFeatures`, `DescribeFeatures`, plus the dataset build
    RPCs `BuildDataset` / `GetDatasetBuild`, see design decision U1/U8) with explicit missing/stale markers
    (client failure reason `feature_failure`), never a silent zero. Feature references are
    `<entity>.<name>@v<N>` everywhere (online hash field `<name>@v<N>`, offline prefix `<entity>.<name>@v<N>`).
  - **Consent**: subjects whose latest consent state (C1 `consent_state_latest`, fed by `CONSENT_UPDATE`) is
    `denied` are excluded from every personalization feature (`user`, `user_item`, `session` entities) and
    from every dataset row; their online keys are removed; business facts (orders, favorites) still feed
    item-level features.
  - Freshness metrics and alerts per feature, a light **feature drift** monitor (PSI against the previous
    week), quality gates that block materialization on bad input, privacy enforcement (PII class, erasure,
    retention of feature artifacts; raw-event retention is C1's), a persistent volume and a backup for the
    DuckDB file (this change owns the compose `DUCKDB_PATH` `/tmp` fix and the analytics PVC for the program).
  - Events gain an ingestion timestamp so "knowable at time t" is defined by when the warehouse received a
    row, not only by when the client says it happened.
- **platform-core**: new `platform/analytics/v1/feature.proto` (`FeatureService`, additive; `buf breaking`
  passes); ADR-0015 "Feature store in team-analytics" (C1 = ADR-0014, C3 = ADR-0016, 0013 reserved). No change
  to existing messages.
- **team-identity**: two new SERVICE-ONLY scopes, `features.read` (online reads, describe; held by
  `service-team-ai`) and `features.dataset` (dataset builds; held by `service-platform-recsys`, the recsys
  trainer), granted to no user role, added to the scope inventory and to the existing negative drift test
  (`coverage_test.go`).
- **team-gateway**: no route. `FeatureService` is internal-only; a test pins that the gateway does not expose it.
- **platform-recsys**: a small dataset client (request a build with `BuildDataset`, wait with
  `GetDatasetBuild`, verify and load the manifest) and the raw-read guard test, plus a registry-validation test
  for C3's view file. The view file itself and switching the trainer to the client are C3's work.
- **team-ai**: a `FeatureService` client only (batched call, deadline, short in-process cache, explicit
  missing markers). Using features in ranking is C3's work.
- **platform-gitops**: persistent volume, `features` bucket, new env, NetworkPolicy allowing `team-ai` and the
  recsys job to reach `:50059`, Prometheus alert rules.
- **Root compose**: DuckDB file on the volume, the bucket, the materializers on, a tiny-traffic recipe that
  shows features in a few minutes.
- **platform-e2e**: integration scenarios (non-UI) for materialization, parity, datasets, the read contract,
  authorization, freshness and erasure.
- **plans/mlops/INDEX.md**: P2-T4 (nearline) and P3-T2 (feature store) re-owned to this change and
  `team-analytics`; the program's other supersessions (P1-T1 → C1; P1-T2/P1-T4 → C3, ADR-0016 instead of the
  ADR-0014 named there) recorded in the same edit.
- **Modified behaviour of `tracking`**: BigQuery stops being an alternative *replacement* for DuckDB.
  DuckDB is the system of record in every environment (the feature store computes on it); BigQuery becomes an
  optional extra BI sink. C1 also modifies `tracking` (other requirements); C1 archives first and this delta is
  rebased onto it (design "Cross-change resolutions").

## Capabilities

### New Capabilities

- `feature-registry`: the catalog of feature definitions, its schema, its versioning and deprecation rules,
  its CI validation and its `DescribeFeatures` view.
- `feature-materialization`: producing the offline store and the online store from one definition, on a
  schedule, with the parity guarantee and per-feature TTL.
- `feature-datasets`: point-in-time training datasets built from consumer-declared views, and the rule that
  trainers read only those datasets.
- `feature-serving`: the internal `FeatureService` read contract, its authorization, latency budget and
  failure semantics.
- `feature-monitoring`: freshness SLOs, staleness metrics and alerts, feature drift, and input quality gates.
- `feature-privacy`: PII classes, erasure propagation and retention for every feature store artifact.

### Modified Capabilities

- `tracking`: "The warehouse target is swappable behind a WarehouseWriter seam" changes: DuckDB is always the
  primary store and BigQuery an optional additional sink; each row gains an ingestion time.

## Impact

- Repos: `team-analytics` (registry, materializers, dataset builder, FeatureService, monitoring, privacy),
  `platform-core` (one new proto file, ADR-0015), `team-identity` (two service-only scopes), `team-gateway`
  (a not-routed test only), `platform-recsys` (view file + dataset client), `team-ai` (client),
  `platform-gitops`, root compose, `platform-e2e`, `plans/mlops/INDEX.md`.
- Contract: additive `platform.analytics.v1.FeatureService`; vendored + regenerated into `team-analytics`
  and `team-ai` (and `platform-recsys` for the Python dataset client). `buf lint` + `buf breaking` must pass.
- Data: DuckDB `tracking_events` gains `ingested_at` (append-only schema column; BigQuery parity test
  updated). New object storage prefixes `features/offline/`, `features/datasets/`. New valkey/Redis
  namespace `fs:v1:`.
- Runtime: the analytics pod runs more work (SQL every 60 s, a nightly batch) and must stay a single replica
  with a persistent volume; memory and CPU limits rise.
- Architecture rules: Rule 3 (the feature store is `team-analytics`' own data; consumers get it only through
  the RPC or the published datasets, never by reading the DuckDB file), Rule 4 (contract only in
  `platform-core`), Rule 5 (no new Kafka topic; dataset builds are requests over gRPC, not events). Rule 2
  holds because the gateway does not route it at all.
- Depends on C1 `tracking-event-platform` for field and event names, `consent_state_latest`, the erasure
  entry point and raw-event retention; runs first on today's 4 event types. Order and favorite features are registered as `pending` until C1's `order.events` and
  `engagement.events` land.
- Interacts with C3 `recommendations-end-to-end`: C3's trainer triggers `BuildDataset` and waits on
  `GetDatasetBuild`; team-ai reads `GetOnlineFeatures` (see design "Cross-change resolutions").

## Non-goals

- No model training, ranking or serving logic change: the trainer switch to datasets and any use of online
  features in `Recommend` are C3 (and later changes).
- No new event types, no attribution fields, no identity stitching, no raw-event retention (C1). Anonymous visitors stay
  `anon:<anonymous_id>` entities; no retroactive merge into the user entity in v1.
- No separate streaming engine (Flink, Spark Structured Streaming, Kafka Streams), no Feast, no new repo.
- No feature UI; `DescribeFeatures` and the registry file are the catalog.
- No features for RAG/LLM, no embeddings as features, no model outputs in the feature store (PROGRAM
  principle 5: ALS vectors and lists stay in `platform-recsys`' `recs:` namespace).
- No auto-rollback or auto-retrain on drift; drift alerts a human.
- No gateway exposure and no user-facing scenario; every scenario here is an integration (non-UI) test.
- No horizontal scaling of `team-analytics`; it stays one replica (design D3).
