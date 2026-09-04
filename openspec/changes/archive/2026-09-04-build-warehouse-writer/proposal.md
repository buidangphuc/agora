## Why

`add-tracking-event-contract` defined the `TrackingEvent` contract and `emit-tracking-events` puts
real events on the Kafka topic `analytics.events`. Nothing consumes that stream yet, so the events
have nowhere to land. This change builds the **warehouse writer**: a Kafka consumer that reads
`analytics.events`, unmarshals the `TrackingEvent` from the `EventEnvelope`, and appends it to an
analytics warehouse behind a `WarehouseWriter` seam — a **DuckDB** adapter (columnar Parquet) for
local/test and a **BigQuery** adapter for prod, chosen by env `WAREHOUSE_DRIVER`. This is the
data-warehouse foundation the recommendation engine (a later P2 Spark job) reads from. It is a new
bounded context that owns its own storage and lifecycle, so per AGENTS.md §6(c) it is a **new
repo**, `team-analytics`.

## What Changes

- **team-analytics** (NEW repo / bounded context, AGENTS.md §6(c)) — a **worker** service (Kafka
  consumer, **no HTTP/gRPC serving surface**) copying the shape of an existing Go service (config
  with `env:`/`default:` tags + `.env.example` drift gate, bootstrap Resources/lifecycle, graceful
  shutdown, gRPC health server only) and reusing the franz-go client style already used by
  `team-domain`/`team-search`. It:
  - consumes `analytics.events` with a consumer group, unmarshals `EventEnvelope` → `TrackingEvent`
    (vendors + generates the `platform.analytics.v1` and `platform.events.v1` proto per ADR-0001);
  - writes through a `WarehouseWriter` interface with two adapters — **DuckDB** (local/test,
    columnar Parquet files) and **BigQuery** (prod) — selected by `WAREHOUSE_DRIVER` (`duckdb` |
    `bigquery`);
  - batches events (~100 evts/s target) before flushing to the warehouse.
- **platform-gitops** — register `team-analytics` as a **worker variant** (Deployment only, no
  Service/Ingress — mirror the `platform/search-indexer` consumer precedent): an ArgoCD
  `Application` (`argocd/apps/team-analytics.yaml`) rendering the shared `charts/service` chart (or a
  raw `platform/team-analytics/` Deployment manifest like search-indexer), plus an
  `images.team-analytics` tag key in `envs/local/values.yaml`. The `gitops-scaffold-service` skill
  covers this scaffold. Worker env: `KAFKA_ENABLED=true`, `KAFKA_ANALYTICS_TOPIC=analytics.events`,
  `KAFKA_CONSUMER_GROUP=team-analytics`, `WAREHOUSE_DRIVER=duckdb` (local).

### Architecture compliance

- Rule 3 (DB-per-service): `team-analytics` owns its warehouse; it never touches another service's
  DB. It reaches nothing over gRPC — the event stream carries everything it needs (the events are
  self-contained, like `team-search`'s read-model rebuild from `listing.events`).
- Rule 4 (contract SoT): no proto change — it **consumes** the existing `platform.analytics.v1`
  contract, vendored and generated in its own tree. platform-core is untouched.
- Rule 5 (right broker): it consumes a **Kafka** state-change stream, not RabbitMQ.

## Non-goals

- **Recommendation model training** — that is the later P2 Spark job that reads from the warehouse;
  out of scope here.
- **Dashboards / BI / query API** over the warehouse.
- **Schema-evolution tooling / migrations framework** for the warehouse tables — a single append
  schema for `TrackingEvent` is enough now.
- Any change to the producer (`emit-tracking-events`) or the contract (platform-core).
- Real recommendation serving, embeddings, or backfill/replay tooling beyond ordinary
  consumer-group replay.
