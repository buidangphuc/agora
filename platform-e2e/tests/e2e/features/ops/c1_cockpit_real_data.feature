@observability @admin
Feature: The cockpit renders real data for admins and honest empty states otherwise
  ops-cockpit-real-data: the HUD shows what the gateway returns, with real Jaeger traces,
  and says "Chưa có dữ liệu" instead of fabricating a figure.

  Scenario: An admin sees the cockpit
    Given an admin is logged in
    When an admin opens the "admin cockpit" page
    Then the cockpit page renders the figures the gateway returns for that admin

  Scenario: A real request appears as a trace
    Given an admin is logged in
    And traffic has been driven through the gateway to the search service
    When the cockpit endpoint is called as the admin
    Then within 90 seconds recent_traces contains a trace that opens in the Jaeger UI

  # The poll response is stubbed in the browser so the null rendering is checked for every
  # source at once; the real unavailable-source paths are the @destructive scenarios below.
  Scenario: Missing data renders as an empty state
    Given an admin is logged in
    When the cockpit receives a response with every figure null
    Then the HUD shows a dash or "Chưa có dữ liệu" and no placeholder number, row or trace id

  # Stops team-analytics through the compose wrapper; teardown starts it again.
  @destructive
  Scenario: Analytics unavailable leaves the figures empty
    Given an admin is logged in
    And team-analytics is stopped
    When the cockpit endpoint is called as the admin
    Then the response keeps its shape and the figures are empty

  # Stops the Jaeger container; teardown starts it again.
  @destructive
  Scenario: Jaeger unavailable
    Given an admin is logged in
    When Jaeger is stopped
    And the cockpit endpoint is called as the admin
    Then recent_traces is empty
    And the HUD shows "Chưa có dữ liệu" for the traces
