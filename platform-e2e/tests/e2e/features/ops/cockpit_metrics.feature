@observability @admin
Feature: Admin cockpit shows live per-service metrics from Prometheus
  As an admin, the /admin/cockpit HUD reflects real per-service RED metrics
  sourced from Prometheus through the gateway's shaped, read-only proxy —
  never the old math/rand stub and never raw Prometheus in the browser.

  Scenario: Cockpit reflects real request activity
    Given traffic has been driven through the gateway to the search service
    When an admin opens the "admin cockpit" page
    Then the observability HUD is visible
    And the cockpit metrics show a non-zero Prometheus-sourced RPS for the search service

  Scenario: Idle stack trends toward zero, not a random baseline
    When the cockpit metrics are read twice with no traffic in between
    Then the search service RPS is stable across the two reads rather than a random baseline
    And no service reports the legacy random stub baseline near 124 RPS

  Scenario: Browser cannot reach raw Prometheus through the gateway
    When the cockpit endpoint is called
    Then only the shaped cockpit metrics response is returned
    And the gateway does not expose a raw Prometheus query endpoint

  Scenario: Prometheus unavailable degrades gracefully
    When the cockpit endpoint is called
    Then the response keeps the expected cockpit shape with numeric, non-random metric values

  Scenario: Orders/revenue are not presented as Prometheus-sourced truth
    When the cockpit endpoint is called
    Then total_orders_24h and total_revenue_24h are present as derived, non-authoritative values
