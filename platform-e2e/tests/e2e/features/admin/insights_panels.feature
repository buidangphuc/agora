@admin @analytics
Feature: Admin cockpit insight panels
  The cockpit at /admin/cockpit shows four aggregate panels (placements, retention, experiments,
  AI surfaces) read from the analytics reports through the gateway. Data is seeded through the
  real tracking collector and real orders; each scenario uses its own model version or flag key.
  Scenario titles for the e2e-verified spec scenarios belong to change admin-insights-panels.

  Scenario: Admin sees all four panels
    Given an admin is logged in
    When the admin opens the insights cockpit
    Then the four insight panels are present on the page

  Scenario: Admin reads recommendation rows per model version
    Given an admin is logged in
    And home_feed impressions served by two different model versions were tracked
    When the admin opens the insights cockpit
    Then the placements panel lists one row per model version with its stored value verbatim

  Scenario: Admin reads weekly cohort rows
    Given an admin is logged in
    And 3 buyers signed up this week
    When the admin opens the insights cockpit
    Then the retention panel shows a row for this week's cohort with a size of at least 3 and percentage cells

  Scenario: Admin reads variant rows with interval
    Given an admin is logged in
    And the seeded admin for experiment reports
    When a logged-in buyer is exposed to variant "control" of a fresh flag and then pays an order, and another visitor is exposed to the same variant and never pays
    And the admin opens the insights cockpit for that flag
    Then the experiments panel shows a control row with 2 exposed, 1 converted, "50.0%" and an interval

  Scenario: Admin reads surface rows with shares
    Given an admin is logged in
    And assistant events were tracked as 10 shown, 4 accepted, 1 edited and 2 dismissed under a fresh model version
    When the admin opens the insights cockpit
    Then the AI surfaces panel shows that row with 10, 4, 1, 2, "40.0%" and "20.0%"

  Scenario: Flag with no exposures shows the empty state
    Given an admin is logged in
    When the admin opens the insights cockpit for a flag that was never exposed
    Then the experiments panel says there are no exposures and shows no variant rows

  @needsBuyer
  Scenario: Non-admin sees the 403 result and none of the insight panels
    Given I am logged in as a buyer via API
    When the buyer opens the insights cockpit
    Then the page shows the admin-required 403 result and none of the insight panels
