# team-analytics

Analytics bounded context: a Go worker that consumes `analytics.events` and `order.events` from Kafka, appends them to a warehouse (embedded DuckDB locally, BigQuery as a write-only prod adapter), and serves the read-only `AnalyticsQueryService` over gRPC on `:50059`. It owns the `tracking_events` and `order_facts` tables, the seller funnel, revenue breakdown, a baseline demand forecast, the admin order summaries, and the Parquet export that `platform-recsys` trains from. Status: deployed in the root compose as `team-analytics-svc`.

## 1. Contract

Served: `platform.analytics.v1.AnalyticsQueryService` (`proto/platform/analytics/v1/analytics.proto`), plus the standard gRPC Health service and optional reflection. The query service is registered only when `WAREHOUSE_DRIVER=duckdb`; with `bigquery` only Health is served (`cmd/consumer/main.go`).

| RPC | Authorization (`internal/query/service.go`) |
|---|---|
| `GetSellerFunnel` | `requireSellerAccess`: no or anonymous principal -> `Unauthenticated`; `admin` scope -> allowed; a `user` principal whose id equals `seller_id` -> allowed; otherwise `PermissionDenied` |
| `GetRevenueBreakdown` | same as above |
| `GetDemandForecast` | same as above |
| `GetPlatformOrderSummary` | `admin` scope only (`RequireScopes`) |
| `ListRecentOrders` | `admin` scope only |
| `GetTrackingQualityReport` | `admin` scope only |

Trust model (ADR-0003): the gateway verifies the token and forwards a resolved Principal as gRPC metadata `x-principal-id`, `x-principal-type` (`user`/`service`/`anonymous`), `x-principal-scopes` (comma-separated). `internal/interceptor/auth.go` reads these headers and does no verification, so the service trusts any caller that can reach `:50059` (see Known gaps).

| RPC | Behavior |
|---|---|
| `GetSellerFunnel` | Returns `impressions`, `views`, `adds`, `begin_checkouts`, `purchases` (counts from `tracking_events`, joined to `listing_sellers` so only events on the seller's own listings count; events on listings with no known seller are excluded, including events with no `listing_id` such as a cart-level `begin_checkout`) and `orders` (distinct `order_id` from `order_facts` for the seller). Window: optional `from`/`to`, open-ended when omitted. |
| `GetRevenueBreakdown` | Per-day revenue and order count, plus the top 10 SKUs by revenue (`SUM(quantity * unit_price)`), from `order_facts`. |
| `GetDemandForecast` | Baseline forecast, `model_version = duckdb_baseline_v1`. Mean and population std-dev of `quantity` per order line (not per day, not time-windowed) for the seller and listing/variant; `is_cold_start` when there is no history, with fallback mean 2.0 and std 1.0. Daily P10/P50/P90 = `max(0, mu - 1.28*sigma)`, `mu`, `mu + 1.28*sigma`. Defaults: horizon 28 days, lead time 3 days, service level 0.95. `z` is 2.33 for service level >= 0.99, 1.28 for 0.90 to below 0.95, else 1.65. `safety_stock = z * mean(P90 - P50 over lead time)`, `reorder_point = sum(P50 over lead time) + safety_stock`. |
| `GetPlatformOrderSummary` | Distinct paid orders and GMV over the trailing window (default 24h), all sellers. |
| `ListRecentOrders` | Latest orders, newest first; default 10, max 100; no buyer PII. |
| `GetTrackingQualityReport` | Health of the tracking stream over a trailing `window_hours` (1 to 168, default 24; outside it -> `InvalidArgument`). Per event type: events, distinct `user_key` visitors (from `tracking_events_resolved`) and, for `view`/`click`/`add_to_cart`/`impression`, the share of events with an empty `listing_id`. Also ingest lag p50/p95 (`ingested_at - occurred_at`, rows with null `ingested_at` excluded), the latest `ingested_at`, and the summed `decode_failures` and `duplicates_skipped`. `status` is `OK` or `DEGRADED` with sorted `reasons`: `stale` (no event in the window, or latest ingest older than `TRACKING_STALE_AFTER_SECONDS`), `lagging` (p95 lag above `TRACKING_LAG_P95_MAX_SECONDS`), `incomplete` (a listing-scoped missing ratio above `TRACKING_MISSING_LISTING_MAX_RATIO`). DuckDB only. |

Callers: `team-gateway` (`UPSTREAM_ANALYTICS_ADDR=team-analytics-svc:50059`). Upstream RPCs consumed: none. Data consumed: the two Kafka topics below.

## 2. Events

| Direction | Topic | Envelope `type` | Effect |
|---|---|---|---|
| Consume | `analytics.events` (`KAFKA_ANALYTICS_TOPIC`) | `platform.analytics.v1.TrackingEvent` | One row in `tracking_events`, `event_id` = envelope `event_id` |
| Consume | `order.events` (`KAFKA_ORDER_TOPIC`) | `platform.order.v1.OrderPaidEvent` | One `order_facts` row per line item, `event_id` = `<envelope event_id>-<item index>`, status `PAID`, currency defaults to `VND` |
| Consume | `listing.events` (`KAFKA_LISTING_TOPIC`) | `platform.listing.v1.ListingChanged` | Idempotent upsert of `listing_id -> seller_id` in `listing_sellers` (DuckDB only). Deletes keep the mapping. |
| Produce | none | | |

The code does not use the Kafka record key.

- `analytics.events` and `order.events` are read by one consumer group (`KAFKA_CONSUMER_GROUP`). `listing.events` is read by its own group (`KAFKA_LISTING_CONSUMER_GROUP`) that starts from the earliest offset, so listings created before the consumer existed are backfilled (`internal/consumer/listing.go`); records are upserted, then offsets are committed. Envelopes of any other type are skipped; an undecodable record is logged and skipped (`internal/consumer/consumer.go`).
- Tracking event types mapped (`internal/consumer/tracking.go`): `view`, `click`, `add_to_cart`, `impression`, `remove_from_cart`, `begin_checkout`, `apply_promotion`, `search_filter`, `favorite`, `share`, `view_cart`, `add_shipping_info`, `add_payment_info`, `purchase`; anything else is stored as `unspecified`.
- Delivery is at-least-once: auto-commit is off and offsets are committed only after a successful batch write. Batches flush at `BATCH_MAX_SIZE` rows or every `BATCH_FLUSH_INTERVAL_SECONDS`, and on shutdown (best effort, 5s). A failed flush keeps the records in memory for retry.
- Appends are idempotent on `event_id`: the DuckDB writer inserts with an anti-join (`INSERT ... SELECT ... WHERE NOT EXISTS (same event_id)`), one transaction per batch, so redelivered events and duplicates inside a batch are skipped (`internal/warehouse/duckdb/duckdb.go`). An anti-join is used rather than a unique index because existing volumes may already hold duplicates. The BigQuery adapter passes `event_id` as the insert id, which is best-effort deduplication only.

## 3. Data

Database-per-service; there are no SQL migration files.

| Object | Notes |
|---|---|
| `tracking_events` | Columns defined in `internal/warehouse/warehouse.go` (`Schema`): event and session ids, `event_type`, `listing_id`, `occurred_at`, principal id/type, `properties` JSON, placement/impression/model_version, GA4-style commerce fields (`currency`, `value`, `price`, `quantity`, `transaction_id`, `coupon`, `item_*`, `shipping_tier`, `payment_type`) |
| `order_facts` | `event_id`, `order_id`, `listing_id`, `variant_id`, `seller_id`, `quantity`, `unit_price`, `currency`, `occurred_at`, `status` |
| `listing_sellers` | `listing_id` (primary key), `seller_id`, `updated_at`. Maps a listing to its owner so tracking events (which carry only a listing id) can be attributed to a seller. An older event never overwrites a newer row. DuckDB only. |
| `tracking_ingest_counters` | `hour` (UTC hour, primary key), `decode_failures`, `duplicates_skipped`. The sink adds to it: one upsert per written batch that skipped a duplicate `event_id`, and one per undecodable message. A counter write failure is logged and never blocks ingestion. DuckDB only. |
| `ga4_events` | DuckDB view over `tracking_events` that renames event types to GA4 names (`view` -> `view_item`, `click` -> `select_item`, `impression` -> `view_item_list`) |

- Schema is applied at boot by the writer: `CREATE TABLE IF NOT EXISTS`, then `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` for every schema column (additive only), then `CREATE OR REPLACE VIEW ga4_events`. The BigQuery adapter creates its tables at boot and treats "already exists" as success. `warehouse.Schema` and `OrderFactsSchema` are the single source for both adapters.
- DuckDB is an embedded file (`DUCKDB_PATH`, default `/data/analytics.duckdb`) on the `analytics_data` volume in the root compose. The query service reuses the writer's `*sql.DB` because DuckDB holds an exclusive file lock.
- Parquet export (`internal/export`): DuckDB only. At start and then every `PARQUET_EXPORT_INTERVAL_SECONDS`, `tracking_events` is written to `PARQUET_EXPORT_PATH.tmp` and renamed over `PARQUET_EXPORT_PATH`, so readers never see a partial file. Enabled only when the path is non-empty and the interval is positive. In the root compose it writes `/data/tracking_events.parquet` every 300s; `platform-recsys` (profile `jobs`) mounts the volume read-only and reads it. `order_facts` is not exported.
- BigQuery (`WAREHOUSE_DRIVER=bigquery`) is write-only: no query path and no Parquet export; it is only unit/parity-tested.

## 4. Configuration

Loaded by reflection from the `env`/`default` tags in `internal/config/config.go`. `.env.example` must list exactly these keys: `make check-env` (`TestEnvExampleInSync`) fails on drift in either direction.

| Variable | Default | Notes |
|---|---|---|
| `ENV` | `local` | Read into settings; nothing uses it |
| `LOG_LEVEL` | `info` | `debug`, `warn`, `error`, else info |
| `LOG_JSON` | `true` | JSON vs text slog output |
| `GRPC_HOST` | `0.0.0.0` | |
| `GRPC_PORT` | `50059` | 1 to 65535 |
| `GRPC_REFLECTION_ENABLED` | `true` | |
| `SHUTDOWN_GRACE_SECONDS` | `10` | Validated but unused; shutdown calls `GracefulStop` with no timeout |
| `KAFKA_ENABLED` | `false` | The process exits with an error unless `true` |
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated |
| `KAFKA_CONSUMER_GROUP` | `team-analytics` | |
| `KAFKA_ANALYTICS_TOPIC` | `analytics.events` | |
| `KAFKA_ORDER_TOPIC` | `order.events` | |
| `KAFKA_LISTING_TOPIC` | `listing.events` | Source of the `listing_sellers` mapping |
| `KAFKA_LISTING_CONSUMER_GROUP` | `team-analytics-listing-sellers` | Own group, earliest offset |
| `WAREHOUSE_DRIVER` | `duckdb` | `duckdb` or `bigquery` |
| `DUCKDB_PATH` | `/data/analytics.duckdb` | Required for `duckdb` |
| `BIGQUERY_PROJECT` | empty | Required for `bigquery` |
| `BIGQUERY_DATASET` | `analytics` | Required for `bigquery` |
| `BIGQUERY_TABLE` | `tracking_events` | Tracking table; order facts go to `order_facts` in the same dataset |
| `PARQUET_EXPORT_PATH` | empty | Export disabled when empty |
| `PARQUET_EXPORT_INTERVAL_SECONDS` | `0` | 0 disables; must be >= 0 |
| `BATCH_MAX_SIZE` | `500` | Must be > 0 |
| `BATCH_FLUSH_INTERVAL_SECONDS` | `2` | 0 disables the interval flush (size and shutdown flush only) |
| `TRACKING_STALE_AFTER_SECONDS` | `900` | Report is `DEGRADED` (`stale`) when the latest ingest is older; must be > 0 |
| `TRACKING_LAG_P95_MAX_SECONDS` | `300` | `DEGRADED` (`lagging`) when p95 ingest lag exceeds it; must be > 0 |
| `TRACKING_MISSING_LISTING_MAX_RATIO` | `0.05` | `DEGRADED` (`incomplete`) when a listing-scoped type's share of events without `listing_id` exceeds it; 0 to 1 |
| `OTEL_ENABLED` | `false` | Inert: no tracing code reads it |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty in code (`.env.example` sets `http://localhost:4317`) | Inert |
| `OTEL_SERVICE_NAME` | `team-analytics` | Inert |

## 5. Run locally

Root stack (preferred), from the repo root:

```bash
docker compose up -d --build                                # whole stack
docker compose --profile jobs run --rm platform-recsys      # trains from the Parquet export
```

The `team-analytics` service in `docker-compose.services.yaml` sets `KAFKA_ENABLED=true`, `KAFKA_BROKERS=redpanda:9092`, `DUCKDB_PATH=/data/analytics.duckdb`, `PARQUET_EXPORT_PATH=/data/tracking_events.parquet`, `PARQUET_EXPORT_INTERVAL_SECONDS=300` on the `analytics_data` volume. It publishes no host port; the gateway reaches it at `team-analytics-svc:50059`. The root compose does not set `KAFKA_ORDER_TOPIC`, so the default `order.events` applies.

Standalone: `docker-compose.local.yaml` runs only this worker against the platform-core Redpanda on the external `platform-core_default` network (DuckDB on the `analytics-data` volume; no Parquet export configured). Without Docker: `cp .env.example .env`, `make proto`, then `KAFKA_ENABLED=true make consumer` (needs CGO and a C toolchain for DuckDB, and a writable `DUCKDB_PATH`; the default `/data/...` usually is not).

## 6. Build, test and lint

| Command | What it does |
|---|---|
| `make proto` | `buf generate` into `generated/` (needs `buf`) |
| `make tidy` | `go mod tidy` (the Dockerfile also runs it in the image) |
| `make check` | Merge gate: `check-env`, `gofmt -l` (fails on unformatted files), `go vet ./...`, `go test ./...` |
| `make check-env` | `.env.example` vs `internal/config` drift gate |
| `make test` | `go test ./...` |
| `docker build -t team-analytics:latest .` | `golang:1.23`, `CGO_ENABLED=1`, runs on `distroless/cc-debian12:nonroot`; `/data` is pre-created and owned by nonroot |

There is no CI workflow in this directory; `make check` is the gate. Go is 1.23 (`go.mod`).

## 7. Spec and verification

- Feature manifest: `FEATURES.yaml` (owner `analytics-team`). The warehouse-sink, driver-swap and batch-write features are `not-testable` (no UI; covered by unit tests and data-layer checks). The order-facts, revenue, demand-forecast and seller-access features are `automated`.
- e2e: `platform-e2e/tests/e2e/features/analytics/order_facts.feature` and `seller_analytics_access.feature`. Verify with `make -C platform-e2e features-check` and, for an OpenSpec change, `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC. The access rules here come from `secure-seller-analytics-and-admin-seed`.

## 8. Gotchas

- `generated/` is gitignored. Run `make proto` (then `go mod tidy`) before building or testing; builds fail without it.
- `proto/` is vendored from platform-core (ADR-0001). Never edit it here; it also contains protos this service does not use. Regenerate with `make proto`.
- DuckDB needs CGO. The image is glibc-based distroless, not the static image the pure-Go services use. The data volume must be writable by `nonroot`.
- Only one process can open the DuckDB file; read the Parquet export instead of opening `analytics.duckdb` from outside.
- `*.duckdb` and `*.parquet` are gitignored.
- `internal/query/memory.go` is an in-memory repository used by unit tests only; it is not wired into the binary.

## 9. Known gaps

- Trusted principal metadata: any caller that reaches `:50059` can set `x-principal-*` and act as a seller or admin. Protection relies on network policy (ADR-0010); this service does no token or mTLS check.
- BigQuery has no query path and no Parquet export; with `WAREHOUSE_DRIVER=bigquery` the five RPCs are not registered (Health only).
- Deduplication on `event_id` is exact in DuckDB only; BigQuery relies on best-effort insert-id deduplication.
- `OTEL_*`, `ENV` and `SHUTDOWN_GRACE_SECONDS` are declared but unused; the `internal/interceptor` package comment mentions a `tracing.go` that does not exist.
- The demand forecast is a flat baseline with no seasonality or recency; its mean and std-dev are per order line over all history.
- Root `AGENTS.md`, `.env.example` and `proto/README.md` still say this service has "no business RPC, health only"; the code serves `AnalyticsQueryService`. Out of scope for this README and needs a separate fix.
- An undecodable record is skipped after a warning; there is no dead-letter topic.

## 10. Links

- Root rules: `../AGENTS.md`, `../README.md` (ASDLC).
- ADRs in `platform-core/docs/ADR/`: `0001-proto-distribution.md`, `0002-async-broker.md`, `0003-auth-model.md`, `0010-service-zero-trust.md`, `0013-seller-demand-forecasting.md`.
