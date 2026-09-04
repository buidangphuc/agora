# team-analytics — warehouse writer (Kafka → analytics warehouse)

A **worker** bounded context (AGENTS.md §6c): a Kafka consumer with **no
request-serving surface** (only a gRPC health server for k8s probes). It reads
the `analytics.events` topic, unmarshals the `platform.analytics.v1.TrackingEvent`
out of each `platform.events.v1.EventEnvelope`, and appends it to an analytics
warehouse behind a `WarehouseWriter` seam. It is the data-warehouse foundation
the later recommendation engine (a Spark job) reads from.

It owns its warehouse exclusively (DB-per-service, Rule 3), reaches nothing over
gRPC (the event stream is self-contained, like team-search's read-model rebuild
from `listing.events`), and edits no contract (Rule 4 — it consumes the existing
`platform.analytics.v1` proto, vendored + generated in its own tree per ADR-0001).

## Shape

```
cmd/consumer/main.go            worker entrypoint (health server + consumer loop)
internal/config                 reflection env loader + .env.example drift gate
internal/observability          slog logger
internal/grpcserver             health-only gRPC server (probes)
internal/bootstrap              Resources/lifecycle + WAREHOUSE_DRIVER adapter switch
internal/consumer               franz-go reader, envelope→record mapping, batcher
internal/warehouse              WarehouseWriter seam + TrackingRecord + canonical Schema
internal/warehouse/duckdb       DuckDB adapter (local/test, columnar Parquet)
internal/warehouse/bigquery     BigQuery adapter (prod, partitioned/clustered)
internal/warehouse/fake         in-memory adapter for black-box tests
proto/                          vendored platform contract (analytics, events, common)
```

## Warehouse seam

One narrow interface, two adapters, chosen at boot by `WAREHOUSE_DRIVER`:

```go
type WarehouseWriter interface {
    Write(ctx context.Context, batch []*TrackingRecord) error
    Close() error
}
```

- **duckdb** (default local/test): embedded, columnar, analyst-queryable, exports
  Parquet — ideal for CI/e2e. The CI/e2e path exercises this driver.
- **bigquery** (prod): streaming insert into a date-partitioned/clustered table.
  Not exercisable in local e2e (no creds); covered by unit + schema-parity tests.

The consume → unmarshal → map path produces a driver-neutral `TrackingRecord`;
switching drivers changes only the env value, never the event handling.

## Batching & delivery

Records buffer and flush on **whichever comes first**: `BATCH_MAX_SIZE` or
`BATCH_FLUSH_INTERVAL_SECONDS` (~100 evts/s target). Kafka offsets are committed
**only after** a successful flush → at-least-once; a crash mid-batch reprocesses.
Rows carry the envelope `event_id` so a downstream dedup is possible.

## Build / test (Docker-only; Go + buf not on the host, ADR-0001)

```bash
make proto            # buf generate → ./generated (gitignored)
make tidy             # go mod tidy → go.sum
make check            # env-drift + gofmt + go vet + go test ./...
docker build .        # CGO (DuckDB) → glibc image
```
