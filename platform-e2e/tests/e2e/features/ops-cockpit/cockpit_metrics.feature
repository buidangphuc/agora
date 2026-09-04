@admin @observability
Feature: Admin Cockpit shows live per-service metrics from Prometheus
  The Admin Cockpit HUD (/admin/cockpit) polls GET /api/admin/metrics, which the
  gateway serves from real Prometheus data (per-service RPS / p95 / p99 /
  error-rate) instead of the old math/rand stub. The gateway runs a fixed,
  hardcoded PromQL set server-side and shapes it into the frozen
  CockpitMetricsResponse — raw Prometheus is never exposed to the browser, and
  orders/revenue are labelled derived pending a real order-domain metric.

  @needsAdmin
  Scenario: Cockpit reflects real request activity
    Given an admin is logged in
    And traffic has been driven through the gateway to the search service
    When the admin opens /admin/cockpit
    Then the HUD shows a non-zero, Prometheus-sourced RPS and latency for that service
    And the values reflect the real activity rather than a fixed or random baseline

  @needsAdmin
  Scenario: Idle stack trends toward zero, not a random baseline
    Given an admin is logged in
    And no traffic is flowing through the gateway
    When the admin opens /admin/cockpit
    Then the exercised service's RPS trends toward zero as Prometheus reports
    And the HUD does not show the previous "~124 + rand()" stub baseline

  @needsAdmin
  Scenario: Browser cannot reach raw Prometheus through the gateway
    Given an admin is logged in
    When the cockpit endpoint GET /api/admin/metrics is called
    Then the gateway returns only the shaped CockpitMetricsResponse JSON
    And no endpoint forwards arbitrary PromQL or raw Prometheus responses to the browser

  @needsAdmin
  Scenario: Prometheus unavailable degrades gracefully
    Given PROMETHEUS_URL is unset or Prometheus is unreachable
    When GET /api/admin/metrics is called
    Then it still returns a valid response in the expected CockpitMetricsResponse shape
    And the metric values are cleared or zeroed, never re-introducing random data

  @needsAdmin
  Scenario: Orders/revenue are not presented as Prometheus-sourced truth
    Given an admin is logged in
    When the cockpit response is produced
    Then total_orders_24h and total_revenue_24h are populated from a derived source, not a Prometheus business counter
    And they are documented as estimated, pending a future order-domain metric
