## ADDED Requirements

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
unset or unreachable the handler SHALL degrade gracefully (zeroed/last-known,
clearly non-random) and still return the expected shape.

#### Scenario: Browser cannot reach raw Prometheus through the gateway

- **WHEN** the cockpit endpoint is called
- **THEN** the gateway returns only the shaped `CockpitMetricsResponse` JSON, and
  no endpoint forwards arbitrary PromQL or raw Prometheus responses to the browser

#### Scenario: Prometheus unavailable degrades gracefully

- **WHEN** `PROMETHEUS_URL` is unset or Prometheus is unreachable
- **THEN** `GET /api/admin/metrics` still returns a valid response in the expected
  shape with cleared/zeroed metric values (never re-introducing random data)

### Requirement: Orders and revenue are labelled derived until an order-domain metric exists

Because no Prometheus business metric for orders/revenue exists and the gateway
must not compute business numbers, `total_orders_24h` and `total_revenue_24h` SHALL
be treated as derived/estimated and documented as non-authoritative, while keeping
the response field shape unchanged so the HUD renders.

#### Scenario: Orders/revenue are not presented as Prometheus-sourced truth

- **WHEN** the cockpit response is produced
- **THEN** `total_orders_24h` and `total_revenue_24h` are populated from a derived
  source (not a Prometheus business counter) and are documented as estimated,
  pending a future order-domain metric
