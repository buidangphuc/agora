# ADR-0015 — Feature Store Materialization (platform-featurestore as a batch job)

**Status:** Accepted · **Date:** 2026-10-09 · **Relates to:** ADR-0005, ADR-0011, ADR-0013, ADR-0014

## Context

The recommenders need per-user and per-item features computed from the warehouse. agora has none: `platform-featurestore` is an unused library whose stores are in-memory dicts, nothing materialises features, and `team-ai` wires an `InMemoryFeatureStore`.

On 2026-10-08 the decision (D1) was taken that features are computed by `platform-featurestore` jobs that read the warehouse's Parquet exports, not inside `team-analytics`. The `team-analytics` exporter already writes `tracking_events.parquet` atomically to the shared `analytics_data` volume, which `platform-recsys` reads as precedent for a batch job on that volume.

## Decision

1. **Features are computed by a `platform-featurestore` batch job** (`python -m featurestore materialize`), in-process DuckDB over the Parquet exports. `team-analytics` owns the warehouse and the export; it computes no features.
2. **Inputs contract.** The exporter writes these files, each replaced atomically (write to `.tmp`, then rename), in the directory of `PARQUET_EXPORT_PATH`:
   - `tracking_events.parquet`
   - `tracking_events_resolved.parquet` (stitched `user_key`, `ingested_at`)
   - `engagement_facts.parquet`
   - `order_facts.parquet`

   The job mounts the volume read-only and reads only these names. A rename of any of them is a breaking change to this ADR.
3. **Registry.** `registry/features.yaml` declares versioned feature views (`user_activity@v1`, `item_popularity@v1`), one SQL definition each. The definition hash (SHA-256 of the SQL plus the YAML entry) is locked in `registry/features.lock`; a changed definition without a version bump fails the job.
4. **Point-in-time rule.** Every view is computed as of `AS_OF` (RFC 3339, default now in UTC). The job exposes only pre-filtered inputs to the SQL: events and facts with `ingested_at <= AS_OF`, orders with `occurred_at <= AS_OF`. Windows are relative to `AS_OF` (`occurred_at > AS_OF - N days AND occurred_at <= AS_OF`). Current-state reductions use the same latest-fact rule as the warehouse views but over the filtered rows, so no definition can see data from after `AS_OF`.
5. **Key scheme (online, Redis).**
   - `fs:<view>:v<version>:<entity_id>` — JSON feature row, with a TTL (`FEATURESTORE_ONLINE_TTL_SECONDS`, default 172800).
   - `fs:<view>:current` — the live version number.
   - `fs:<view>:meta` — `as_of`, `materialized_at`, input watermark.

   `current` and `meta` are written last, so a reader never sees a pointer to a half-written version. The job uses Redis DB 2 (`redis://redis:6379/2`). The unversioned `fs:u:` / `fs:i:` keys of the old prototype are retired; nothing read them.
6. **Offline output.** `<FEATURESTORE_OFFLINE_DIR>/<view>/v<ver>/as_of=<YYYYMMDDTHHMMSSZ>.parquet` per view, version and `as_of`, plus `runs/<as_of>/manifest.json` (`as_of`, `materialized_at`, input watermark, per-view row count and definition hash, input file size and mtime).
7. **Parity gate.** After writing, the job compares a deterministic sample of online values with the offline snapshot and exits with code 3 on any mismatch. `python -m featurestore parity` runs the same check alone.
8. **Packaging.** Compose service `featurestore-job` under `profiles: [featurestore]`, mounting `analytics_data:/data:ro` and `featurestore_data:/features`. The job runs on demand (compose profile, e2e, CI); no scheduler yet.

## Alternatives Rejected

- **Compute features inside `team-analytics`.** Couples feature definitions to the warehouse service's release cycle and bloats its scope; superseded by D1.
- **Spark, as in `platform-recsys`.** Needless for this data size; DuckDB starts in seconds and runs identical SQL in e2e and CI. Revisit when the data outgrows one node.
- **A gRPC online feature service.** Left open. Online reads stay a library over Redis for now. If a service boundary is later needed (shared access control, multi-language consumers), a gRPC read API can sit in front of the same keys without changing this key scheme.
- **Reading the unfiltered warehouse views.** They cannot honour `AS_OF`, which would leak future data into point-in-time features.

## Consequences

- Features are at most one export cycle behind (`PARQUET_EXPORT_INTERVAL_SECONDS`, 300 s locally).
- Any `AS_OF` is reproducible, which is the basis for training datasets (a later change).
- Redis memory grows with entities × versions; the TTL lets an old version expire after a bump.
- Serving features to `team-ai` and building training sets are out of scope here and handled by later changes.

## Supersedes

The `platform-featurestore` README's "unused prototype" status and its pointer to a team-analytics feature store are superseded by this ADR.
