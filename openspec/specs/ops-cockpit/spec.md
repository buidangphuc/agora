# ops-cockpit Specification

## Purpose
TBD - created by archiving change replace-cockpit-mock-metrics. Update Purpose after archive.

## Requirements

### Requirement: The cockpit HUD shows live per-service metrics sourced from Prometheus

The system SHALL serve the Admin Cockpit HUD (`/admin/cockpit`) per-service
RPS, p95/p99 latency and error-rate from real Prometheus data via
`GET /api/admin/metrics`, replacing the `math/rand` stub. The values SHALL reflect
real request activity through the gateway and SHALL NOT be randomly generated. The
response SHALL keep the exact JSON shape the existing `CockpitView` consumes
(`CockpitMetricsResponse` with `services[]`, `total_rps`, `avg_latency_ms`,
`recent_traces[]`, etc.) so the frontend is unchanged.

#### Scenario: Cockpit reflects real request activity

- **WHEN** an admin opens `/admin/cockpit` after traffic has been driven through
  the gateway to a service (e.g. search reads)
- **THEN** the HUD shows a non-zero, Prometheus-sourced RPS and latency for that
  service, reflecting the real activity rather than a fixed or random baseline

#### Scenario: Idle stack trends toward zero, not a random baseline

- **WHEN** no traffic is flowing through the gateway
- **THEN** the exercised service's RPS trends toward zero (as Prometheus reports),
  rather than the previous `~124 + rand()` stub baseline

### Requirement: The gateway is a thin read-only proxy over Prometheus

The gateway SHALL query Prometheus server-side with a fixed, hardcoded PromQL query
set and shape the results into the cockpit response. It SHALL NOT expose raw
Prometheus, accept arbitrary PromQL from the browser, or embed business logic
(rule 2). Prometheus SHALL be reached via a `PROMETHEUS_URL` config; when it is
unset or unreachable the handler SHALL degrade gracefully (`prometheus_available:false`, null
latency/error-rate, `UNKNOWN` status; never random or placeholder values) and still return the expected shape.

#### Scenario: Browser cannot reach raw Prometheus through the gateway

- **WHEN** the cockpit endpoint is called
- **THEN** the gateway returns only the shaped `CockpitMetricsResponse` JSON, and
  no endpoint forwards arbitrary PromQL or raw Prometheus responses to the browser

#### Scenario: Prometheus unavailable degrades gracefully

- **WHEN** `PROMETHEUS_URL` is unset or Prometheus is unreachable
- **THEN** `GET /api/admin/metrics` still returns a valid response in the expected
  shape with cleared/zeroed metric values (never re-introducing random data)

### Requirement: The cockpit never shows fabricated data

Every figure on the cockpit SHALL come from a real source or be shown as an
honest unavailable/empty state ("Chưa có dữ liệu" / a dash). Because no order-domain
metric, trace feed or `order.events` publisher into the `ops:orders` SSE room
exists yet, `total_orders_24h` and `total_revenue_24h` SHALL be `null`,
`recent_traces` SHALL be empty, and the live-orders ticker SHALL render only real
`OrderPlaced` events (empty until they flow). Latency and error-rate with no
sample SHALL be `null`, not `0`. The page SHALL NOT contain hard-coded numbers,
order rows, buyer addresses or trace ids as fallbacks.

#### Scenario: Orders, revenue and traces are never fabricated

- **WHEN** the cockpit response is produced
- **THEN** `total_orders_24h` and `total_revenue_24h` are `null` and no fabricated
  traces are returned

#### Scenario: Missing data renders as an empty state

- **WHEN** the gateway or Prometheus is unreachable, or a figure is `null`
- **THEN** the HUD shows a dash / "Chưa có dữ liệu" and no placeholder number
