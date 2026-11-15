@analytics @admin
Feature: Admin reads an experiment readout per variant
  AnalyticsQueryService/GetExperimentReadout counts each visitor once in the variant of their first
  exposure and the paid orders within 7 days after it. Each scenario uses a fresh flag key.
  Scenario titles equal the `#### Scenario:` titles of change analytics-experiment-reports.

  Scenario: Conversion within seven days of first exposure
    Given the seeded admin for experiment reports
    When a logged-in buyer is exposed to variant "control" of a fresh flag and then pays an order, and another visitor is exposed to the same variant and never pays
    Then the readout of that flag has variant "control" with exposed 2, converted 1 and rate 0.5 inside its Wilson interval and window_days 7

  Scenario: A visitor is counted once in the first variant
    Given the seeded admin for experiment reports
    When one anonymous visitor is exposed to variant "a" of a fresh flag and then to variant "b"
    Then the readout of that flag has variant "a" with exposed 1 and no row for variant "b"

  Scenario: Unknown flag returns no variants
    Given the seeded admin for experiment reports
    When the admin reads the readout of a flag that was never exposed
    Then the readout response succeeds with 0 variants and window_days 7
