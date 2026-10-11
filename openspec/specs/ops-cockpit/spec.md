# ops-cockpit Specification

## Purpose
Defines the admin operations cockpit: live per-service metrics from Prometheus, 24h orders and GMV from order facts, and recent traces from Jaeger, read through a thin admin-only gateway proxy that never shows fabricated data.

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
unset or unreachable the handler SHALL degrade gracefully with
`prometheus_available: false`, `null` latency and error-rate values and an `UNKNOWN`
service status, never random, zeroed-as-real or placeholder values, and still
return the expected shape.

#### Scenario: Browser cannot reach raw Prometheus through the gateway

- **WHEN** the cockpit endpoint is called
- **THEN** the gateway returns only the shaped `CockpitMetricsResponse` JSON, and
  no endpoint forwards arbitrary PromQL or raw Prometheus responses to the browser

#### Scenario: Prometheus unavailable degrades gracefully

- **WHEN** `PROMETHEUS_URL` is unset or Prometheus is unreachable
- **THEN** `GET /api/admin/metrics` returns a valid response with
  `prometheus_available: false`, `null` latencies and error rates and `UNKNOWN`
  statuses, and no fabricated value

### Requirement: The cockpit never shows fabricated data

Every figure on the cockpit SHALL come from a real source or render as an honest
unavailable or empty state (a dash or "Chưa có dữ liệu"). The page and the gateway
SHALL NOT contain hard-coded numbers, order rows, buyer addresses or trace ids as
fallbacks. A value with no sample SHALL be `null`, not `0`.

#### Scenario: Missing data renders as an empty state

- **WHEN** the gateway, Prometheus, team-analytics or Jaeger is unreachable, or a
  figure is `null`
- **THEN** the HUD shows a dash or "Chưa có dữ liệu" for that figure and no
  placeholder number, row or trace id

### Requirement: Only admins can read the cockpit

`GET /api/admin/metrics` SHALL require an authenticated principal with the `admin`
scope: no token SHALL return 401 and a non-admin token SHALL return 403, before any
upstream call. The `/admin/cockpit` page SHALL check the session server-side, send a
non-admin or anonymous visitor to `/login` (anonymous) or a 403 result (non-admin), and
fetch the cockpit data server-side with the session token (the browser never needs the
token).

#### Scenario: Anonymous request is rejected

- **WHEN** `GET /api/admin/metrics` is called without a token
- **THEN** the gateway returns 401 and does not query Prometheus, team-analytics or
  Jaeger

#### Scenario: A buyer cannot open the cockpit

- **WHEN** a signed-in buyer (no `admin` scope) opens `/admin/cockpit` or calls
  `/api/admin/metrics`
- **THEN** the page renders a 403 result and the endpoint returns 403

#### Scenario: An admin sees the cockpit

- **WHEN** a signed-in admin opens `/admin/cockpit`
- **THEN** the page renders the metrics returned by the gateway for that admin

### Requirement: Orders and GMV over 24h come from order facts

`team-analytics` SHALL expose an admin-scoped `GetPlatformOrderSummary(window)` RPC that
aggregates `order_facts` (fed by `order.events`) into the number of distinct paid orders
and the GMV (`SUM(quantity * unit_price)`, minor units) for the window, and a
`ListRecentOrders(limit)` RPC returning the latest paid orders (order id, seller id,
total, paid_at; no buyer PII). The gateway cockpit handler SHALL call them with a fixed
24h window and limit and copy the results into `total_orders_24h`,
`total_revenue_24h` and `recent_orders`, without computing business numbers itself.

#### Scenario: A paid order shows up in the 24h figures

- **WHEN** a buyer places and pays an order of 2 × 150,000 ₫ and the event is ingested
- **THEN** within 30 s `total_orders_24h` increases by 1, `total_revenue_24h` increases
  by 300,000 and `recent_orders` lists that order id first

#### Scenario: Analytics unavailable leaves the figures empty

- **WHEN** team-analytics is unreachable
- **THEN** `total_orders_24h` and `total_revenue_24h` are `null`, `recent_orders` is
  empty, and the rest of the response is still returned

### Requirement: Recent traces come from Jaeger

The gateway SHALL query the Jaeger query API server-side (`JAEGER_QUERY_URL`) with a
fixed query (service `team-gateway`, last 1 h, limit 5) and return each trace's id, root
operation, span count, duration and start time in `recent_traces`, linking each to the
Jaeger UI. It SHALL NOT accept a query from the browser.

#### Scenario: A real request appears as a trace

- **WHEN** a request has passed through the gateway in the last hour and Jaeger is up
- **THEN** `recent_traces` contains at least one entry whose id opens in the Jaeger UI

#### Scenario: Jaeger unavailable

- **WHEN** `JAEGER_QUERY_URL` is unset or Jaeger is unreachable
- **THEN** `recent_traces` is empty and the HUD shows "Chưa có dữ liệu"
