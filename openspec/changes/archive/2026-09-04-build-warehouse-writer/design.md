## Context

`emit-tracking-events` puts `platform.analytics.v1.TrackingEvent` (wrapped in
`platform.events.v1.EventEnvelope`) on the Kafka topic `analytics.events`. This change consumes that
stream into an analytics warehouse. It is a **new bounded context** (AGENTS.md §6(c)): it owns new
storage (the warehouse) and its own lifecycle, so it is a new repo `team-analytics`, not a feature
on an existing service. It is a **worker** — a Kafka consumer with no request-serving surface, the
same shape as `team-search`'s `cmd/indexer` (registered in gitops as `platform/search-indexer`, a
Deployment with no Service). Two warehouse backends are needed: a zero-dependency local/test store
and a managed prod store — hence the adapter seam.

## Decisions

- **Worker, not a gRPC service.** No RPC/HTTP is served; input is the Kafka topic. It keeps only a
  gRPC **health** server (for k8s probes) like the other Go services, and mirrors the reference
  template (config + bootstrap Resources/lifecycle + graceful shutdown). Gitops registration is the
  worker variant (Deployment only, no Service/Ingress), copying `platform/search-indexer`.

- **`WarehouseWriter` interface as the swap seam.** One narrow interface —
  `Write(ctx, batch []*TrackingRecord) error` + `Close()` — with two adapters chosen at boot by
  `WAREHOUSE_DRIVER`:
  - `duckdb` (default local/test): appends to DuckDB, persisted as columnar **Parquet**; no external
    service, fast, analyst-queryable. Ideal for e2e assertions and CI.
  - `bigquery` (prod): streams/loads rows into a BigQuery table.
  The consume→unmarshal→map path produces a driver-neutral `TrackingRecord`; only the adapter differs.
  New backends (e.g. Snowflake) are a third adapter, no consumer change.

- **Batching (~100 evts/s).** Buffer records and flush on **whichever comes first**: batch size
  (e.g. 500) or max interval (e.g. 2s). Both DuckDB (bulk append) and BigQuery (batch load / storage
  write) strongly prefer batches over per-row writes; the interval bound keeps a partial batch from
  lingering. **Offsets are committed only after a successful flush** → at-least-once delivery; a
  crash mid-batch reprocesses. Rows carry the envelope `event_id` so a downstream dedup is possible,
  but exactly-once is out of scope.

- **Why NOT Postgres.** Tracking is a high-volume, append-only, **analytical (OLAP)** workload —
  scans/aggregations over columns, not transactional row lookups. A row-store Postgres would be the
  wrong engine (and would also violate the spirit of DB-per-service by looking like another
  transactional service DB). A columnar warehouse (DuckDB local / BigQuery prod) matches the query
  shape the recommendation engine will run, and keeps the analytics store cleanly separate from the
  services' operational Postgres instances.

- **DuckDB ↔ BigQuery SQL parity.** Keep the table schema and write path to the intersection of both
  engines: a flat, append-only `tracking_events` table with primitive columns (event_id, event_type
  as string/enum-name, listing_id, session_id, anonymous_id, path, referrer, position, query,
  occurred_at TIMESTAMP, principal_id, principal_type) plus a JSON/`properties` column
  (DuckDB `JSON` / BigQuery `JSON`). Avoid engine-specific types and DDL; partition/cluster by date
  in BigQuery, plain Parquet partition dirs in DuckDB. The adapters own their own DDL; the consumer
  only produces `TrackingRecord`s.

## Risks / Trade-offs

- **At-least-once → possible duplicates** on reprocessing after a crash. Accepted for analytics;
  `event_id` supports later dedup. Exactly-once would need transactional offset+write coupling not
  worth it here.
- **DuckDB is single-writer / embedded.** Fine for one worker replica (this is a single-consumer
  worker); scaling out consumers later means BigQuery in prod or a shared store — noted, not solved
  now.
- **Adapter parity drift.** DuckDB and BigQuery SQL/type surfaces differ; the mitigation is the
  deliberately minimal shared schema above and adapter-owned DDL. A parity test (same
  `TrackingRecord` batch → both adapters in CI where credentials allow) guards it.
- **BigQuery in CI.** BigQuery needs credentials/emulation not available in local e2e; the e2e/CI
  path exercises the DuckDB adapter, and the BigQuery adapter is covered by unit tests + a
  parity/contract test rather than live e2e.
