## Why

The Admin Cockpit HUD (`team-frontend/src/features/admin/CockpitView.tsx`, page
`/admin/cockpit`) polls `GET /api/admin/metrics` every 3s and renders per-service
RED metrics, Golden Signals and "recent traces". The handler behind it,
`HandleCockpitMetrics` (`team-gateway/internal/edge/cockpit.go`), is a **pure
`math/rand` stub**: fake RPS/latency per service, hardcoded `TotalOrders24h: 1420`
and `TotalRevenue24h: 384500000`, and three hardcoded Jaeger trace IDs. Nothing it
shows is real.

Once `add-prometheus-infra` lands, real per-service RPS/latency exist in
Prometheus. This change replaces the stub with real PromQL queries, keeping the
gateway a thin read-only proxy (rule 2) that does **not** expose raw Prometheus to
the browser, and keeps the exact JSON shape `CockpitView` already consumes so the
frontend does not change.

**Depends on:** `add-prometheus-infra` (the metrics source).

## What Changes

- **team-gateway (`internal/edge/cockpit.go`)** — replace the `math/rand` body of
  `HandleCockpitMetrics` with a fixed set of server-side PromQL queries against
  Prometheus, mapped into the **unchanged** `CockpitMetricsResponse` /
  `ServiceHealth` / `TraceSummary` structs. The gateway issues a hardcoded query
  set and shapes the results — it does **not** proxy arbitrary PromQL and does
  **not** return raw Prometheus payloads to the browser (rule 2, thin proxy, no
  business logic).
- **team-gateway (`internal/config`)** — add `PROMETHEUS_URL`
  (default `http://prometheus:9090`). When it is empty or Prometheus is
  unreachable, the handler degrades gracefully to a clearly-zeroed/last-known
  response rather than reintroducing random data, so local-without-Prometheus still
  renders.
- **team-gateway** — `total_orders_24h` / `total_revenue_24h` have **no** business
  metric in Prometheus (team-order emits Kafka events, not a Prometheus counter),
  and the gateway must not compute business numbers (rule 2). These two fields stay
  **derived/estimated and are labelled as such** (documented; the JSON shape is
  unchanged so the HUD keeps rendering). A real order-domain counter is a follow-up
  (non-goal). `recent_traces` continue to link to Jaeger; deriving them from live
  trace data is out of scope here (still constructed, labelled non-authoritative).

### PromQL → response-field mapping (folded in here; no separate design.md)

Metric names follow the `add-prometheus-infra` gateway RED series rendered through
the collector's Prometheus exporter (`marketplace_*` namespace); exact names settle
at apply time. Illustrative mapping:

| Response field | Source |
|---|---|
| `services[].rps` | `sum by (service) (rate(marketplace_rpc_client_duration_ms_count[1m]))` |
| `services[].p95_latency_ms` | `histogram_quantile(0.95, sum by (le,service) (rate(marketplace_rpc_client_duration_ms_bucket[5m])))` |
| `services[].p99_latency_ms` | `histogram_quantile(0.99, sum by (le,service) (...bucket[5m]))` |
| `services[].error_rate` | `sum by (service)(rate(...count{grpc_status!="OK"}[5m])) / sum by (service)(rate(...count[5m]))` |
| `services[].status` | `HEALTHY` when the series is present and error_rate below threshold; else `DEGRADED`/`DOWN` |
| `total_rps` | sum of `services[].rps` |
| `avg_latency_ms` | overall mean latency across services |
| `total_orders_24h`, `total_revenue_24h` | **derived/estimated** — labelled, no Prometheus business metric yet |
| `recent_traces[]` | Jaeger links, non-authoritative (unchanged behaviour) |

## Non-goals

- New HUD widgets or changes to `CockpitView`'s consumed shape (fields stay
  identical).
- The SSE ops-order ticker rework (`/api/events/live`, `RealtimeBroker`) — untouched.
- Auth/authorization changes on the cockpit endpoint.
- A real order-domain orders/revenue business metric (deferred follow-up).
- Deriving `recent_traces` from live Jaeger/trace queries.
- Any Prometheus infra work (owned by `add-prometheus-infra`).
