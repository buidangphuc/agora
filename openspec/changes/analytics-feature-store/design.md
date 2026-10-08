## Decisions needed from the user

Merged across C1/C2/C3 (deduplicated, with the legal-review items): `plans/ai-first/DECISIONS.md`.

Each item has a recommended default; `tasks.md` is written against the defaults. Changing one changes only the
named tasks. Names fixed by `plans/ai-first/PROGRAM.md` (key prefix, layouts, entity names, service name) are
not decisions here, except where noted as a conflict at the end of this file.

| # | Decision | Recommended default | Alternative(s) | Tasks affected |
|---|---|---|---|---|
| U1 | Online read path | **gRPC `FeatureService.GetOnlineFeatures`** served by team-analytics (D7): TTL/staleness, version checks, PENDING/RETIRED and authz enforced in one place; one RTT (~1-3 ms in-cluster) on top of the valkey read | **Direct key contract**: consumers `HGET fs:v1:*` themselves (faster, no new hop, but every consumer re-implements decoding, staleness and version rules, and valkey becomes a shared database across services, against Rule 3) | 1.x, 6.x, 11.x |
| U2 | Micro-batch interval | **60 s** for all online features; `item.covisit_session_7d` every 15 min (`online_every`). Online freshness promise **~2 min**, replacing `plans/mlops` P2-T4's "event → Redis < 10 s": one SQL definition re-run in micro-batches (no second streaming implementation, so no training/serving skew) costs freshness, and no consumer in C3 needs seconds (trending is cached 30 s, recent items drive a nightly-trained ANN) | 10 s for `item.trending_1h` and `user.recent_items_24h` only (meets the old P2-T4 "<10 s" target, ~6x the query load) | 3.6, 3.7 |
| U3 | Offline store location | **One bucket `features`** on the stack's MinIO (local compose and the in-cluster MinIO in `platform-gitops`); S3 when a cloud bucket exists; prefixes exactly as PROGRAM | GCS via BigQuery external tables (ties offline to the BI warehouse; see D10) | 3.4, 9.x, 10.x |
| U4 | Registry format | **One `features/registry.yaml`** with inline SQL, plus `features/registry.schema.json`; validator in Go inside `make check` | One file per feature (`features/defs/<entity>.<name>@v<N>.yaml` + `.sql`): nicer diffs at >30 features, more files to review | 2.x |
| U5 | Versioning policy | **Immutable versions**: any edit that can change a value (SQL, entity, dtype, window, aggregation, weights) = new `@v<N+1>`; in-place only for description/owner/SLO/status; deprecation needs `sunset_date` ≥ 30 days; `experimental` may be dropped with no notice | Semver (`@v1.2`) with "minor = additive column": more expressive but a consumer can no longer pin a value-exact definition with one integer, and the PROGRAM key format uses `@v<N>` | 2.3, 2.4 |
| U6 | Retention | **Raw events: C1's windows, enforced by C1's retention job** (behavioural rows 13 months, free text — search query, path, referrer — nulled after 90 d, facts 25 months, quarantine 14 d; C1 U2); this change only owns its artifacts: **offline partitions 180 d**; **datasets 30 d**, or up to 180 d when pinned by a promoted model; online by per-feature TTL; raw-event backup partitions follow C1's windows (rows 13 months, free-text columns nulled after 90 d) | Datasets kept forever when pinned (reproducibility vs erasure cost); shorter offline history (90 d) | 8.5 |
| U7 | Who may add features | **Any team proposes via PR; `team-analytics` owners must approve** (CODEOWNERS on `team-analytics/features/`); new entries start `experimental` or `pending`; `stable` needs a green parity gate | Only `team-analytics` writes entries (slower, safest); or consumer teams self-approve `experimental` (fastest, risks duplicate definitions) | 2.6 |
| U8 | How a consumer requests a dataset | **Two more RPCs on `FeatureService`: `BuildDataset(view_yaml)` → `build_id`, `GetDatasetBuild(build_id)`** (async, one build at a time, queue of 4) | Drop the view file in `features/requests/` in the bucket and poll for the manifest (no proto change, but object storage as a job queue and no auth beyond bucket ACLs); or a RabbitMQ job (Rule 5 says jobs belong there, but RabbitMQ task wiring is still an open seam, AGENTS §9) | 1.x, 5.x, 12.x |
| U9 | Where the materializers run | **In the team-analytics process** (goroutines sharing the single DuckDB handle), one replica (D3) | A sidecar or separate Job (impossible against the live file: DuckDB allows one read-write process per file; would need the Parquet event mirror of D9 as its input, adding up to one hour of delay) | 3.x, 9.x |
| U10 | Role of BigQuery | **BI-only optional sink** (`WAREHOUSE_BI_SINK=bigquery`), never a feature source; `WAREHOUSE_DRIVER=bigquery` refused (D10) | Keep BigQuery as an alternative primary (then every feature needs a second dialect and parity across engines) | 3.1, 3.2 |
| U11 | Service scopes | **Two service-only scopes**: `features.read` (held by `service-team-ai`) and `features.dataset` (held by `service-platform-recsys`, the recsys trainer, which calls `BuildDataset` then polls `GetDatasetBuild`) | One scope `features.read` for everything (simpler, but the serving path could start dataset builds) | 4.x, 11.x |
| U12 | Top-N-by-feature read (C3 U12 alternative) | **Not in v1**: `GetOnlineFeatures` is key-based only; C3 re-scores a candidate pool | Add `ListTopEntities(feature, k)` backed by a sorted set per feature (useful for trending, adds a write path per feature) | none (follow-up) |

## Context

See `proposal.md` (Why). State that shapes the design, verified in code on 2026-10-01:

- **team-analytics** is one Go binary (`cmd/consumer`): a franz-go consumer of `analytics.events` that batches
  `TrackingRecord`s into one DuckDB table `tracking_events` (13 columns, `warehouse.Schema`, mirrored in
  BigQuery), commits offsets after a successful write, and serves `AnalyticsQueryService` over the writer's
  own `*sql.DB` (single-writer file lock). `WAREHOUSE_DRIVER=bigquery` *replaces* DuckDB, and then the query
  service is not registered. No metrics, no object storage client, no Redis client. Go 1.23,
  `go-duckdb v1.8.3` (CGO).
- **Persistence is broken today**: compose sets `DUCKDB_PATH=/tmp/analytics.duckdb` while mounting the volume
  at `/data`; gitops mounts an `emptyDir`; NetworkPolicy admits only `team-gateway`.
- **Infra**: compose has `redis:7-alpine` and MinIO (bitnamilegacy image, `minio-init` creates buckets);
  gitops has `valkey/valkey:8-alpine` (no per-field hash expiry, which arrived in Valkey 9 / Redis 7.4) and an
  in-cluster MinIO; Prometheus exists but has no rule file (C3 adds the first one).
- **Consumers**: `platform-recsys` (Spark ALS, CronJob) builds `(user, item, weight)` from raw rows with the
  weight map impression 0.5 / view 1 / click 2 / add_to_cart 5 and a recency decay using
  `current_timestamp()`. C3 (`recommendations-end-to-end`, revised) already declares
  `als_interactions@v1` with two as-of snapshots and a `FeatureClient` with a `NullFeatureClient` fallback.
- **Events today**: 4 types (`view`, `click`, `add_to_cart`, `impression`) with `listing_id`, `session_id`,
  `anonymous_id`, `position`, `search_query`, envelope principal. C1 adds types, attribution and server
  facts later.
- Principles applied (from the user's notes `Feature Store`, `Training vs Inference`, `Data Leakage`,
  `Data Drift`, `Feature Engineering`): one definition feeds both stores; offline is throughput-tuned history,
  online is latency-tuned latest value; point-in-time (as-of) joins and time-based splits stop temporal and
  target leakage; a registry is the discovery layer that prevents duplicate definitions; drift is measured
  with PSI against a full-week reference so weekly seasonality does not read as drift; skew must be ruled out
  before drift is believed. The `Feature Store` note's YAGNI caveat (one model, simple features, small team)
  is answered in proposal.md: ALS, the C3 placements, the ranker, two-tower, search LTR, assistant
  personalization and demand forecasting are planned consumers of the same item/user signals.

## Goals / Non-Goals

**Goals:**

- Every feature a model uses has exactly one SQL definition, and both stores are produced from it.
- A trainer cannot see the future: datasets are as-of correct by construction and raw events are unreachable
  from training code.
- Serving gets fresh (≤ 2 min for short windows) values with explicit missing/stale semantics.
- It runs on the existing stack (DuckDB, MinIO, valkey) with no new engine and works locally in minutes.
- It has a clear exit to Feast/BigQuery when one node is not enough (D10).

**Non-Goals (design-level):** sub-second freshness; exactly-once Kafka-to-feature semantics beyond event-id
dedup; cross-entity joins against other services' data (listing price, category: those are C1/C3 concerns or
a later `listing.events` source); multi-replica analytics.

## Decisions

### D1 — Registry: one YAML file, JSON schema, Go validator in `make check`

`team-analytics/features/registry.yaml` (U4) holds entries; `registry.schema.json` (draft 2020-12) is the
structural contract; a Go validator (`internal/features/registry`) adds the semantic checks that a schema
cannot express. Example entry:

```yaml
- name: views_7d
  version: 1
  kind: feature            # feature | label (labels: offline only, never served)
  entity: item             # user | item | user_item | session
  dtype: int64             # int64 | float64 | bool | string_list | scored_list
  source_events: [analytics.events:view]
  window: 7d
  aggregation: count
  offline_grain: 1h        # as-of times per offline partition
  online_every: 60s        # micro-batch cadence (multiple of the base tick)
  ttl: 15m                 # online staleness tolerance on the effective as-of
  freshness_slo: {online: 2m, offline: 2h}
  owner: team-analytics
  pii_class: behavioral_aggregate
  status: stable
  description: Distinct view events on the listing in the 7 days before as_of.
  sql: |
    SELECT listing_id AS entity_id, count(*)::BIGINT AS value
    FROM fs_events(:as_of, INTERVAL 7 DAY)
    WHERE event_type = 'view' AND listing_id <> ''
    GROUP BY listing_id
```

`fs_events(as_of, window)` is a DuckDB table macro owned by the store, not by a feature: it returns
`tracking_events` de-duplicated by `event_id`, restricted to `ingested_at <= as_of` and
`occurred_at > as_of - window AND occurred_at <= as_of`, excluding `principal_type = 'service'`, with a derived
`user_key` (`principal_id` for users, `anon:` || `anonymous_id` otherwise). Every feature reads events only
through it, so point-in-time visibility is one piece of code (spec feature-materialization, first
requirement). Opted-out subjects are filtered by the store **outside** the feature SQL, like the entity filter
(D2): `Compute` drops rows of `user`, `user_item` and `session` features whose subject (`user_key`, the prefix of a
`user_item` key) is in `fs_consent_denied`, the set of subjects whose latest consent state is `denied` in C1's
`consent_state_latest` (refreshed every tick; an empty stub view until C1 task 5.5 lands). Item-level features are
unaffected, so business facts (orders, favorites) keep flowing; personalization stops. The online writer deletes the
keys of newly denied subjects (same per-user index as erasure, D9) and the dataset builder applies the same filter at
build time. The validator: schema; unique reference; SQL compiles (`EXPLAIN` against an empty schema) and
returns `entity_id` + `value` of the declared dtype; SQL reads only `fs_events`/`fs_events_*` macros; PII
column denylist (D11); `stable` needs a parity record; `deprecated` needs `sunset_date ≥ deprecated_on + 30d`;
**immutability**: the CI job computes `sql_hash` = SHA-256 of the normalized SQL + entity + dtype + window +
aggregation for every entry and compares with `git show origin/main:team-analytics/features/registry.yaml`;
a changed hash for an existing reference fails (U5). The registry is embedded into the binary
(`go:embed`) so the running service and CI see the same file.

*Alternatives:* Feast feature repo (Python objects; adds a Python runtime and a registry DB to a Go
service); dbt models (good SQL tooling but no online story and a second toolchain).

### D2 — One definition, two materializations: same SQL, as-of parameter, entity filter outside

Both materializers call one function `Compute(ref, asOf, entityFilter)` which binds `:as_of` and, when given,
wraps the registered SQL as `SELECT * FROM (<sql>) WHERE entity_id IN (SELECT id FROM fs_dirty)`. The filter
sits *outside* the feature query, so a feature's value for an entity cannot depend on whether the filter is
present (global terms such as the CTR position prior are computed inside). Parity is therefore true by
construction for the SQL; the parity gate (D6) checks everything around it: as-of binding, dedup, type
mapping, encoding to valkey and decoding in the read path.

- **Batch (offline)**: nightly at 01:00 UTC plus hourly for `offline_grain: 1h` features, for each as-of
  time in the partition: `COPY (Compute(...)) TO '<staging>/part-<n>.parquet'`, upload with `minio-go` to
  `features/offline/<entity>.<name>@v<N>/dt=YYYY-MM-DD/`, then upload `_manifest.json`
  (`schema_version: fs-offline.v1`, `feature_ref`, `sql_hash`, `as_of[]`, `input_watermark` = max
  `ingested_at` read, `row_count`, `sha256` per part). Rows: `entity_id, as_of, value`. Rerun overwrites
  parts then the manifest; readers ignore a prefix without a manifest. Backfill = same code over past days
  (bounded by raw-event retention, U6).
- **Micro-batch (online)**: a ticker every 60 s (U2). Per tick: `asOf = now()` truncated to the second;
  `fs_dirty` = entities with rows ingested in `(prev_asOf, asOf]` ∪ entities with events whose
  `occurred_at` crossed the window edge in `(prev_asOf - window, asOf - window]` (computed from the same
  macro); decayed features (`aggregation: decayed_sum`) also re-evaluate every entity with a value once per
  `offline_grain`. Results go to valkey in pipelined `HSET fs:v1:<entity>:<id> <name>@v<N> <json>` where
  `<json>` = `{"v":<value>,"t":<asOf ms>,"h":"<sql_hash[:12]>"}`, then `EXPIRE` (window + 1 d), then
  `HSET fs:v1:_meta <ref> <asOf ms>` (the run watermark) **last**. The read path's effective as-of =
  `max(value.t, _meta[ref])` (valid because every entity whose value changed was rewritten in that run).
  TTL enforcement happens at read time because valkey 8 has no per-field expiry.

*Why micro-batch with the same SQL and not a streaming engine:* the hard requirement is one definition. A
Flink/Kafka-Streams job would be a second implementation of every window, dedup and decay rule in a second
language and runtime, exactly the skew the `Training vs Inference` note warns about, plus a new cluster to
operate. The P2-T4 lesson ("do not build an approximate ALS in the speed layer") points the same way. The
cost is freshness (60 s + query time instead of seconds) and repeated work; at this stack's volume (target
~100 events/s) a tick touches a few thousand entities and runs in well under a second on DuckDB. If a
feature ever needs seconds, it gets a dedicated incremental path *and* stays in the parity gate (U2
alternative).

### D3 — Process placement: everything in the team-analytics process, one replica, persistent volume

DuckDB permits one read-write process per file; the query service already shares the writer's handle. The
materializers, dataset builder, drift job, retention and erasure all run as goroutines in the same process
over the same `*sql.DB`, coordinated by a small scheduler with one **heavy lane** (batch, dataset build,
backfill, partition rewrite: one at a time) and one **light lane** (micro-batch). DuckDB settings:
`threads` = 2, `memory_limit` = 60% of the container limit, `temp_directory` on the PV. Ingestion keeps
priority: the consumer's write transaction is short and DuckDB's MVCC lets reads run beside it. The
Deployment stays `replicas: 1` with `strategy: Recreate` (two pods must never open the file), gets a 20 Gi
`ReadWriteOnce` PVC (U6 sizing for local/staging traffic; revisit at 10 M events/day), memory limit 1 Gi.
gRPC and metrics stay on the same pod. Operator actions (materialize now, backfill, erase, pin, sweep, restore)
go through `analytics-ctl`, which talks to the running process over a Unix socket on the PV (an in-pod operator
interface, not a network API and not a cross-service contract, so Rule 4 is not involved); it never opens the
DuckDB file itself. *Alternatives:* sidecar (cannot open the locked file); a separate
CronJob reading the D9 Parquet mirror (adds up to one hour of delay and a second copy of the event schema;
kept as the scale-out path in D10).

### D4 — Point-in-time datasets from consumer view files

The view file is the consumer's (C3 already wrote `als_interactions@v1`). Schema `fs-view.v1`:

```yaml
view: als_interactions
version: 1
entity: user_item
features: [user_item.implicit_score_decayed@v1]
labels: []                               # registry entries of kind: label
spine: {kind: snapshot, as_of: [watermark - 2d, watermark]}   # or {kind: events, label: <ref>}
window_days: 30
consumer: platform-recsys
```

`BuildDataset(view_yaml)` validates it against the registry (unknown, pending or retired references refuse;
spec feature-datasets), stores it with its SHA-256 (`view_hash`), and queues a build. `watermark` = the
minimum, over the pinned features, of the latest complete offline as-of, or "now" for snapshot spines (which
are computed directly, below; their `watermark` is the build time capped by the source tables' ingestion
watermark — C1's `ingest_watermarks`, or max `ingested_at` before C1 — so a build over stalled ingestion carries
an old watermark that the trainer can refuse). Build:

- **Snapshot spine** (ALS): for each as-of in the list, run `Compute(ref, asOf)` for every pinned feature and
  outer-join on the entity key; output `entity keys, as_of, <feature>, <feature>__missing`.
- **Events spine** (rankers, later): the spine is a registered label (e.g. a future
  `user_item.clicked_after_impression@v1`, built from C1 attribution events) with a timestamp per row; each
  feature is attached with DuckDB `ASOF LEFT JOIN offline_partitions ON entity_id = ... AND spine.ts >= f.as_of`,
  which takes the latest value at or before the label time, never after. No match → null + `__missing = true`.
- **Splits**: `split: {train_until: T1, eval_until: T2}` labels rows `train`/`eval`; labels for eval rows read
  only events in `(T1, T2]` because the label SQL is itself evaluated as of `T2` with its window starting at
  `T1`.

Output: `features/datasets/<view>@v<N>/<build_id>/part-*.parquet`, then `_manifest.json`
(`schema_version: fs-dataset.v1`, `build_id` (ULID), `view_hash`, `features[{ref, sql_hash}]`, `spine`,
`as_of[]`, `watermark`, `partitions_used[]`, `row_count` per split, `sha256` per part, `invalidated: false`).
Rows are sorted by the entity keys and as-of before writing, so the content checksum is reproducible.

### D5 — Hard gate: trainers read datasets only

- `platform-recsys`: `recsys/datasets.py` (task 12.2, owned here) is the only reader: `request_build(view_path)`
  (`BuildDataset`, scope `features.dataset`) → `wait(build_id)` (polls `GetDatasetBuild` until DONE/FAILED or a
  timeout) → `load(manifest)`; C3's trainer calls these three before every training run rather than listing the
  newest build; it checks `view_hash` against the local file, `schema_version`, the
  per-part checksum and `invalidated`. A guard test (`tests/test_no_raw_events.py`, shared with C3 D1) walks
  the AST of every module under `recsys/` and fails on references to `tracking_events`, `analytics.events`,
  `WAREHOUSE_PARQUET_PATH`, `duckdb` or a BigQuery table read.
- `team-ai`: a guard test fails if any module outside the feature client reads `fs:v1:` keys.
- `team-analytics` side: the DuckDB file and the raw-event backup prefix are reachable only by this pod (PVC +
  bucket policy: the recsys MinIO user can read `features/datasets/` only).

### D6 — Parity gate

`internal/features/parity` replays `testdata/parity_stream.jsonl` (≈ 5 000 synthetic events over 9 days,
with duplicates, late arrivals, window-edge crossings, anonymous and service principals) into an in-memory
DuckDB + `miniredis`, running the micro-batch loop on a fake clock every 60 s. At 50 random tick times it
samples 200 (entity, feature) pairs, reads them back through the **FeatureService read path** (decode,
effective as-of, status) and compares with `Compute(ref, t)` read back through the **offline Parquet
writer/reader**. Integers and lists: exact; floats: relative 1e-9. Any mismatch fails `make check`. A
mutation test (deliberately disable dedup in the online path) must make it fail. At runtime the service
samples 20 pairs per hour the same way against the live stores and exports `feature_parity_mismatch_total`.
The parity record that `stable` requires is the CI job output pinned per `sql_hash` in
`features/parity.lock`.

### D7 — Read contract: `platform.analytics.v1.FeatureService`

New file `platform-core/packages/proto/platform/analytics/v1/feature.proto` (additive; no existing message
changes):

```proto
service FeatureService {
  rpc GetOnlineFeatures(GetOnlineFeaturesRequest) returns (GetOnlineFeaturesResponse);
  rpc DescribeFeatures(DescribeFeaturesRequest) returns (DescribeFeaturesResponse);
  rpc BuildDataset(BuildDatasetRequest) returns (BuildDatasetResponse);      // U8
  rpc GetDatasetBuild(GetDatasetBuildRequest) returns (GetDatasetBuildResponse);        // U8
}
enum FeatureEntity { FEATURE_ENTITY_UNSPECIFIED = 0; FEATURE_ENTITY_USER = 1; FEATURE_ENTITY_ITEM = 2;
                     FEATURE_ENTITY_USER_ITEM = 3; FEATURE_ENTITY_SESSION = 4; }
enum FeatureStatus { FEATURE_STATUS_UNSPECIFIED = 0; FEATURE_STATUS_OK = 1; FEATURE_STATUS_MISSING = 2;
                     FEATURE_STATUS_STALE = 3; FEATURE_STATUS_PENDING = 4; FEATURE_STATUS_RETIRED = 5; }
message FeatureValue { oneof kind { int64 int64_value = 1; double double_value = 2; bool bool_value = 3;
                       StringList string_list = 4; ScoredIdList scored_list = 5; } }
message FeatureResult { string feature_ref = 1; FeatureStatus status = 2; FeatureValue value = 3;
                        google.protobuf.Timestamp as_of = 4; }
message GetOnlineFeaturesRequest { FeatureEntity entity = 1; repeated string entity_ids = 2;
                                   repeated string feature_refs = 3; }
message GetOnlineFeaturesResponse { repeated EntityFeatures entities = 1; }  // same order as entity_ids
```

(`StringList`, `ScoredIdList`, `EntityFeatures`, `Describe*`, `BuildDataset*`, `DatasetBuild` follow the same
style; `DatasetBuild` carries `build_id`, `state` (QUEUED/RUNNING/DONE/FAILED), `manifest_uri`, `error`.)
`value` is unset unless status is OK: absence is explicit in the type, so a client cannot read a zero by
accident. Server: one pipelined `HMGET` per entity (fields = requested names), decode, effective as-of from a
1-second in-process cache of `fs:v1:_meta`, status. Budget p99 ≤ 20 ms for 100 × 20 at the server (U1);
`UNAVAILABLE` if valkey errors (fail loud; the client decides to degrade).

**Auth**: `interceptor.RequireService` + `RequireScopes("features.read")` (online, describe) or
`("features.dataset")` (builds), U11. Both scopes go into `team-identity`'s service-only inventory
(`serviceOnlyScopes` in `coverage_test.go`, comment in `scopes.go`). Callers present their own service
principal the way `team-order` does (`asServiceWithScope`): `service-team-ai`, `service-platform-recsys`.
**Gateway**: no forwarder; a test asserts `/platform.analytics.v1.FeatureService/*` answers 501
`unimplemented` like the other internal-only RPCs (AGENTS §4). **NetworkPolicy** for `team-analytics`
admits `team-gateway` (query service), `team-ai` and pods labelled `app: recsys-train`.

**team-ai client** (`app/modules/platform/features/`): async gRPC stub, deadline 30 ms, batches per request,
LRU cache keyed `(entity, id, ref)` with lifetime `min(30 s, freshness_slo.online / 2)` read from
`DescribeFeatures` at startup, and on deadline/error returns every value as unavailable with reason
`feature_failure` plus `feature_client_failures_total`. It exposes only that; C3 decides how to use it.

### D8 — Freshness, drift and quality (v1-light P3-T4)

Metrics through the OTel meter (ADR-0004) exported by the collector's Prometheus endpoint:
`feature_freshness_seconds{feature,store}`, `feature_materialization_duration_seconds{lane}`,
`feature_rows_written_total{feature,store}`, `feature_quality_check_failed_total{check,source}`,
`feature_drift_psi{feature}`, `feature_parity_mismatch_total`, `feature_read_requests_total{status}`,
`feature_read_latency_seconds`, `feature_dataset_builds_total{state}`. Alert rules
(`platform-gitops/platform/monitoring/rules/feature-store.yaml`, unit-tested with `promtool test rules`):
`FeatureOnlineStale` (freshness > SLO for 5 m), `FeatureOfflineStale`, `FeatureQualityGateFailed`,
`FeatureDriftHigh` (PSI > 0.2, warning), `FeatureParityMismatch` (> 0 in 1 h, critical).

- **Drift**: after each daily partition, for each numeric `stable` feature: deciles from the trailing 7 days
  (full weekly season, per the `Data Drift` note), PSI of the new day with ε-smoothing of empty bins, stored
  in `fs_drift` and exported. Warn only; never blocks.
- **Quality gates** (before each run, per source): empty-entity-key share > 5%, duplicate `event_id` share >
  20%, volume < 10% of the trailing-7-day median for the same hour (skipped while fewer than 7 days of
  history exist), share of `occurred_at > ingested_at + 5 m` > 1%, and C1's data-quality verdict for the source
  when C1 publishes one (until then: always pass). Failure: skip the features reading that source for this run,
  keep the watermark (so reads turn STALE after TTL, honestly), alert.

### D9 — Privacy, erasure, retention, backup

- **PII classes** (D1 validator): `none`, `pseudonymous_id`, `behavioral_aggregate`. Output columns may not
  be, or be a projection of, `search_query`, `page_path`, `referrer`, `properties`, IP or user agent; entity
  keys are principal ids, `anon:<anonymous_id>`, listing ids. (Aggregates *over* queries, such as a count,
  are allowed.)
- **Erasure** `Erase(principal_id, anon_ids[])` (entry point: an internal `cmd/analytics-ctl erase` command
  now; once C1 task 6.3 lands, C1's `EraseSubject` owns raw-row deletion and calls this function as its
  post-erasure hook, which then skips step 1): (1) until C1 lands, `DELETE` the subject's rows from
  `tracking_events`; always record the subject in `fs_erased(subject, erased_at)` (C1's `erasure_tombstones` are
  read as well once present); (2) delete `fs:v1:user:<id>`
  and every `fs:v1:user_item:<id>|*` key using the per-user index set `fs:v1:_idx:user_item:<id>` maintained
  by the online writer; (3) `fs_events` excludes erased subjects, so every later partition and dataset is
  clean; (4) a heavy-lane job rewrites retained offline partitions and backup partitions that contain the
  subject (newest first, within 30 days) and sets `invalidated: true` on dataset manifests whose
  `partitions_used` or snapshot as-of predates the erasure and that contain the subject; the recsys client
  refuses invalidated manifests. Step 1-3 within 24 h (spec).
- **Consent**: see D1 (`fs_consent_denied`). Opt-out is not erasure: retained offline partitions and already-built
  datasets are not rewritten (new builds and online values exclude the subject); deleting history is the erasure
  path. Whether opt-out must also purge history is a legal item (`plans/ai-first/DECISIONS.md`).
- **Retention** (U6): raw `tracking_events` and fact rows are **not** swept here; C1's retention job owns them
  (13 months behavioural, free text nulled after 90 d, facts 25 months, quarantine 14 d). The nightly sweep here
  deletes offline partitions older than 180 d, datasets older than 30 d unless the manifest carries
  `pinned_until` (set by C3's promotion through a future `PinDataset`, or by an operator), max 180 d, and
  applies C1's windows to the raw-event backup prefix (partitions older than 13 months deleted, free-text columns
  in partitions older than 90 d rewritten null).
- **Backup** (sized for C1's 13-month raw retention, ~4x the earlier 90-day assumption; the 20 Gi PVC of D3
  is for local/staging traffic): nightly the previous day's raw events are written to `features/backup/events/dt=YYYY-MM-DD/`
  (Parquet + manifest); restore = new DuckDB file + `INSERT ... FROM read_parquet(...)`; a restore drill is a
  task. This mirror is also the input of the scale-out path (D10). It is private to team-analytics.

### D10 — BigQuery stays BI-only; the exit to Feast/BigQuery

Feature SQL must have one dialect to keep "one definition". DuckDB is the feature engine in every environment
(the gitops manifests already pin `duckdb`, so nothing deployed changes). BigQuery becomes an optional
fan-out sink after the DuckDB write (`WAREHOUSE_BI_SINK`), best-effort with a failure counter (spec
`tracking`), for analysts and dashboards, never on the feature path. **Migration path** when one node is not
enough (any of: > 10 M events/day, DuckDB file > 100 GB, nightly batch > 30 min, dataset builds > 15 min):
(1) move compute out of process onto the D9 Parquet mirror (same SQL, DuckDB in a Job); (2) if still not
enough, generate Feast `FeatureView`s from `registry.yaml` with BigQuery as the offline store, transpiling
the SQL with `sqlglot` and re-running the parity gate against the generated SQL; online keys
(`fs:v1:*`) and `FeatureService` stay, so consumers see no change.

### D11 — Initial feature set

Built on today's 4 event types; `pending` until C1's server facts exist. Weights for implicit feedback move
from `platform-recsys` config into the feature definition (C3 records the same). Base tick 60 s.

| Reference | dtype | Window | Definition (abridged) | Offline grain | Online every / TTL | Freshness SLO online / offline | Status |
|---|---|---|---|---|---|---|---|
| `user_item.implicit_score_decayed@v1` | float64 | 30 d | Σ weight(type) · 0.5^(age_days/14); weights impression 0.5, view 1, click 2, add_to_cart 5; key `user_key|listing_id` | 1 d (snapshot spines computed at as-of) | 60 s (touched) + hourly decay / 26 h | 5 m / 26 h | stable after parity |
| `item.views_1d@v1` | int64 | 1 d | count `view` | 1 h | 60 s / 15 m | 2 m / 2 h | stable after parity |
| `item.views_7d@v1` | int64 | 7 d | count `view` | 1 h | 60 s / 15 m | 2 m / 2 h | stable after parity |
| `item.views_30d@v1` | int64 | 30 d | count `view` | 1 d | 60 s / 1 h | 5 m / 26 h | stable after parity |
| `item.clicks_7d@v1` | int64 | 7 d | count `click` | 1 h | 60 s / 15 m | 2 m / 2 h | stable after parity |
| `item.ctr_7d_position_debiased@v1` | float64 | 7 d | (clicks + 1) / (Σ_impressions prior_ctr(position) + 1); prior per position bucket 1..20, 21+ from all items in the same window; null below 20 impressions | 1 h | 60 s / 15 m | 2 m / 2 h | experimental (until C1 attribution makes positions reliable) |
| `item.add_to_cart_7d@v1` | int64 | 7 d | count `add_to_cart` | 1 h | 60 s / 15 m | 2 m / 2 h | stable after parity |
| `item.orders_30d@v1` | int64 | 30 d | count order placed minus cancelled, from `order.events` | 1 d | 60 s / 1 h | 5 m / 26 h | **pending C1** |
| `item.favorites_30d@v1` | int64 | 30 d | favorites added minus removed, from `engagement.events` | 1 d | 60 s / 1 h | 5 m / 26 h | **pending C1** |
| `item.trending_1h@v1` | float64 | 1 h | Σ weight(type) · 0.5^(age_min/15); view 1, click 2, add_to_cart 5 | 1 h | 60 s / 10 m | 2 m / 2 h | stable after parity |
| `user.recent_items_24h@v1` | string_list | 24 h | up to 20 distinct listing ids from view/click/add_to_cart, most recent first; key `user_key` (incl. `anon:`) | 1 h | 60 s / 10 m | 2 m / 2 h | stable after parity |
| `user.event_count_30d@v1` | int64 | 30 d | count of all non-impression events (consumers derive cold start, e.g. < 5) | 1 d | 60 s / 1 h | 5 m / 26 h | stable after parity |
| `item.covisit_session_7d@v1` | scored_list | 7 d | top 20 other listings viewed in the same session, score = number of shared sessions, min 2; excludes self | 1 d | 15 m / 2 h | 30 m / 26 h | experimental |

All are `pii_class: behavioral_aggregate`, owner `team-analytics`. Later (C1): `session` entity features,
search features from `SEARCH`, placement-level CTR from attribution fields, `item.orders_30d` and
`item.favorites_30d` flip to `experimental` when their source topics exist.

### D12 — Local development story

Compose: team-analytics gets `DUCKDB_PATH=/data/analytics.duckdb` (fixes the `/tmp` bug; **owned by this change**,
task 9.1, which has no dependency so it can land first; C1 11.1 needs it and C3 does not touch it); then (task 9.4) `FS_ENABLED=true`, `FS_ONLINE_INTERVAL=60s` (overridable to `10s`),
`FS_BATCH_CRON` hourly locally, `S3_*` for `minio:9000`, `REDIS_ADDR=redis:6379`, `depends_on` minio + redis;
`minio-init` creates bucket `features`. `scripts/fs_local_demo.sh` (root): sends ~300 beacons for 3 buyers
and 2 anonymous visitors through the gateway (reusing the platform-e2e traffic helper), waits ≤ 3 minutes
for `fs:v1:item:<id>` fields, prints `GetOnlineFeatures` for two items and one user via `grpcurl` from inside
the network with a service principal, runs `analytics-ctl materialize --now` (offline for today) and
`analytics-ctl build-dataset platform-recsys/feature_views/als_interactions@v1.yaml`, and prints the manifest.
That is C3 D14's "documented local trigger".

## Risks / Trade-offs

- [DuckDB single file is a single point of failure and a scale ceiling] → PVC + nightly Parquet backup +
  restore drill; documented thresholds and exit path (D10); `Recreate` strategy so two pods never fight over
  the lock.
- [Heavy batch or dataset build slows ingestion or the micro-batch] → one heavy lane, DuckDB thread and memory
  caps, batch at 01:00 UTC, micro-batch on its own lane; metrics on lane durations; offsets still commit only
  after writes, so slowness never loses events.
- [60 s freshness is slower than the old P2-T4 "<10 s" target] → U2 alternative per feature; SLOs state the
  real promise (2 min).
- [Effective as-of relies on "every changed entity was rewritten"] → the dirty-set query is part of the
  parity stream (window-edge crossings and late arrivals are in the fixture); a full re-evaluation of every
  online entity runs daily as a safety net.
- [Read-side TTL means expired values still occupy memory] → key EXPIRE window + 1 d; valkey memory metric
  watched; per-field expiry when valkey ≥ 9.
- [Erasure rewrites are heavy] → bounded by 180 d retention, newest-first, 30-day SLA, heavy lane.
- [View/label semantics grow complex (events spines, labels)] → v1 implements snapshot spines end to end and
  events spines with one test label; rankers (later) exercise the rest.
- [C1 field names change after C2 ships] → only `fs_events` touches event columns; new event types are new
  source events, and value-changing reinterpretations are new versions.

## Migration Plan

1. Land the proto (feature.proto), ADR-0015 and the identity scopes; vendor into team-analytics and team-ai.
2. team-analytics: `ingested_at` column (append-only `ALTER TABLE ... ADD COLUMN ingested_at TIMESTAMP`;
   existing rows get `ingested_at = occurred_at`, documented as approximate history), BI-sink refactor,
   registry, materializers, FeatureService; all behind `FS_ENABLED` (default false) until the parity gate is
   green.
3. Compose and gitops: PVC, bucket, env, NetworkPolicy, alert rules; enable `FS_ENABLED` locally, then
   staging. Data on the current `emptyDir` is lost once (acceptable: today it is lost on every reschedule).
4. platform-recsys and team-ai clients land; C3 switches the trainer input and the serving `FeatureClient`.
5. Rollback: `FS_ENABLED=false` stops all feature work; consumers already degrade (`NullFeatureClient`,
   dataset refusal); ingestion and the seller query service are unaffected.

## Cross-change resolutions (program planner, applied; PROGRAM.md updated)

1. **Reference format.** `<entity>.<name>@v<N>` everywhere (features); `<view>@v<N>` for views; online hash
   field `<name>@v<N>` inside `fs:v1:<entity>:<id>`; offline prefix `features/offline/<entity>.<name>@v<N>/`.
2. **FeatureService RPCs and scopes.** `GetOnlineFeatures`, `DescribeFeatures` (`features.read`, team-ai) and
   `BuildDataset`, `GetDatasetBuild` (`features.dataset`, recsys trainer); both scopes service-only (task 4.1).
3. **Online freshness.** ~2 min replaces P2-T4's "< 10 s" (U2 states why). P2-T4 keys are not built.
4. **Failure term.** `feature_failure` everywhere (C3 renamed its earlier term to match).
5. **C3 input.** C3 has no raw export; its trainer calls `BuildDataset` and waits with `GetDatasetBuild` before
   every run (D5) and refuses a build whose `watermark` (D4) is older than its maximum age. C3 owns the view file
   `platform-recsys/feature_views/als_interactions@v1.yaml` (C3 4.1, written in `fs-view.v1`); this change owns
   the dataset client and the raw-read guard (12.2, 12.3) and only validates the view (12.1).
6. **Raw-event retention** is C1's (U6); **consent** comes from C1's `consent_state_latest` (D1, D9);
   **erasure** of raw rows is C1's `EraseSubject`, which calls this change's `Erase` hook (D9).
7. **ADR numbers.** C1 = ADR-0014, this change = **ADR-0015**, C3 = ADR-0016; 0013 reserved (placement engine).
8. **Compose `DUCKDB_PATH` and the analytics PVC** are owned here (9.1, 10.1); C1 and C3 depend on them.
9. **`ingested_at`** is owned here (3.1, `tracking` delta); C1 does not re-add it.
10. **`tracking` main spec, archive order.** C1 modifies "Analytics tracking events are defined in the contract"
    and "The edge collector accepts browsing beacons and produces tracking events"; this change modifies only
    "The warehouse target is swappable behind a WarehouseWriter seam". Disjoint requirements, so the least
    conflicting option is to keep both deltas separate rather than merge this one into C1. **C1 archives
    first**; before archiving this change, re-run `openspec validate analytics-feature-store --strict` against
    the archived main spec and rebase the delta if its requirement text moved (task 16.4).

## Open Questions

- Exact quality-gate thresholds once real traffic exists (defaults above are conservative and local-safe).
- Whether `item.ctr_7d_position_debiased` should switch its prior to per-placement once C1 attribution
  lands (that is a new version, not a change to v1).
