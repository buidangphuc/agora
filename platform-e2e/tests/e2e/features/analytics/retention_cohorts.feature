@analytics @admin
Feature: Admin reads weekly retention cohorts anchored on the first sign_up week
  AnalyticsQueryService/GetRetentionCohorts returns one cohort per non-empty signup week, admin only.
  Scenario titles equal the `#### Scenario:` titles of change analytics-experiment-reports.

  Scenario: Cohort is anchored on the first sign_up week
    Given the seeded admin for experiment reports
    When 3 buyers sign up through the gateway, one of them posting a second sign_up beacon
    Then the admin's retention report for the current week has a cohort whose size grew by 3 and whose cohort_week is this week's UTC Monday

  Scenario: An empty warehouse returns an empty list
    Given the seeded admin for experiment reports
    When the admin requests 1 retention cohort starting at a Monday two years ago
    Then the retention response succeeds with 0 cohorts
