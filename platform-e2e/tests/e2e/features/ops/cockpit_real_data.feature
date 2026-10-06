@observability @admin
Feature: Only admins read the cockpit, and its order figures come from real paid orders
  GET /api/admin/metrics requires the `admin` scope (401 without a token, 403 without
  admin, before any upstream call) and /admin/cockpit checks the session server-side.
  For admins, total_orders_24h / total_revenue_24h / recent_orders come from
  team-analytics order_facts (fed by order.events), never from a constant.

  Scenario: Anonymous request is rejected
    Given no one is logged in
    When GET /api/admin/metrics is called
    Then the gateway answers 401

  @needsBuyer
  Scenario: A buyer cannot open the cockpit
    Given I am logged in as a buyer via API
    When GET /api/admin/metrics is called
    Then the gateway answers 403
    When the buyer opens the "admin cockpit" page
    Then the page shows the admin-required 403 result and no metrics

  @needsBuyer @needsSeller
  Scenario: A paid order shows up in the 24h figures
    Given an admin is logged in
    And the 24h order figures are noted
    When a buyer places and pays an order of 2 x 150000 VND
    Then within 30 seconds total_orders_24h has increased by at least 1
    And total_revenue_24h has increased by at least 300000
    And the recent orders include that order with a total of 300000
