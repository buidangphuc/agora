## MODIFIED Requirements

### Requirement: The cockpit HUD shows live per-service metrics sourced from Prometheus

The system SHALL serve the Admin Cockpit HUD (`/admin/cockpit`) per-service
RPS, p95/p99 latency and error-rate from real Prometheus data via
`GET /api/admin/metrics`, replacing the `math/rand` stub. The values SHALL reflect
real request activity through the gateway and SHALL NOT be randomly generated. The
response SHALL keep the JSON keys the existing `CockpitView` consumes
(`CockpitMetricsResponse` with `services[]`, `total_rps`, `avg_latency_ms`,
`recent_traces[]`, etc.); `total_orders_24h` and `total_revenue_24h` MAY be `null`
and `recent_traces` MAY be empty. The browser SHALL obtain the data through the
frontend's same-origin route (which attaches the admin session token), not by calling
the gateway endpoint directly without credentials.

#### Scenario: Cockpit reflects real request activity

- **WHEN** an admin opens `/admin/cockpit` after traffic has been driven through
  the gateway to a service (e.g. search reads)
- **THEN** the HUD shows a non-zero, Prometheus-sourced RPS and latency for that
  service, reflecting the real activity rather than a fixed or random baseline

#### Scenario: Idle stack trends toward zero, not a random baseline

- **WHEN** no traffic is flowing through the gateway
- **THEN** the exercised service's RPS trends toward zero (as Prometheus reports),
  rather than the previous `~124 + rand()` stub baseline

## ADDED Requirements

### Requirement: Cockpit metrics are visible to admins only

`GET /api/admin/metrics` SHALL require an authenticated principal holding the `admin`
scope (HTTP 401 for no or invalid token, 403 for a principal without the scope).

#### Scenario: Unauthenticated request is rejected

- **WHEN** `GET /api/admin/metrics` is called without a bearer token
- **THEN** the response is 401 and contains no metric data

#### Scenario: Buyer session cannot see the cockpit data

- **WHEN** a buyer's session token is used to call `GET /api/admin/metrics`
- **THEN** the response is 403 and contains no metric data

### Requirement: Orders, revenue and traces are never fabricated

The cockpit response SHALL NOT contain constants presented as live figures. Until a real
order-domain source exists, `total_orders_24h` and `total_revenue_24h` SHALL be `null`,
and `recent_traces` SHALL be an empty list; the HUD SHALL render an explicit "not
available" state for them rather than a number.

#### Scenario: Orders and revenue are null, not estimated

- **WHEN** an admin calls `GET /api/admin/metrics`
- **THEN** `total_orders_24h` and `total_revenue_24h` are `null` and `recent_traces` is `[]`

#### Scenario: The HUD shows an unavailable state

- **WHEN** an admin opens `/admin/cockpit` and the response has null order and revenue totals
- **THEN** the HUD shows "not available" for those tiles and does not show a numeric placeholder

## REMOVED Requirements

### Requirement: Orders and revenue are labelled derived until an order-domain metric exists

**Reason**: The figures were hard-coded constants (`1420` orders, `384500000` revenue) presented to admins as
live data; labelling them "derived" in documentation did not stop the HUD from showing them as facts.

**Migration**: Replaced by "Orders, revenue and traces are never fabricated". A real source (an order-domain
metric or an analytics query) is a separate future change and can restore numeric values.
