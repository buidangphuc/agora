@analytics @admin
Feature: The three analytics reports are served through the gateway to admins only
  GetRetentionCohorts, GetExperimentReadout and GetAiSurfaceReport answer 200 to the seeded admin
  and 403 to a buyer or a seller. Scenario titles equal the `#### Scenario:` titles of change
  analytics-experiment-reports.

  Scenario: Admin reads each report through the gateway
    Given the seeded admin for experiment reports
    When the admin calls the three report procedures through the gateway
    Then each procedure answers HTTP 200 with its response field

  Scenario: Buyer or seller is refused at the edge
    Given a buyer and a seller for experiment reports
    When the buyer, then the seller, call each of the three report procedures through the gateway
    Then every one of those calls answers HTTP 403

  Scenario: Report latency within budget
    Given the seeded admin for experiment reports
    When the admin calls each report 30 times over its widest window
    Then the p95 latency of each report is at most 2 seconds
