@analytics @admin @ai
Feature: Admin reads AI-surface interaction counts per surface and model version
  AnalyticsQueryService/GetAiSurfaceReport counts shown, accepted, edited and dismissed
  ai_interaction events. Each scenario posts beacons under its own fresh model_version.
  Scenario titles equal the `#### Scenario:` titles of change analytics-experiment-reports.

  Scenario: Counts per surface and model version
    Given the seeded admin for experiment reports
    When visitors post 5 shown, 2 accepted, 1 edited and 1 dismissed assistant events under one fresh model version and 3 shown chat_copilot events under another
    Then the AI surface report has exactly the assistant row 5, 2, 1, 1 and the chat_copilot row 3, 0, 0, 0 for those model versions

  Scenario: Empty window returns no rows
    Given the seeded admin for experiment reports
    When the admin reads the AI surface report over a window two years ago lasting one hour
    Then the AI surface response succeeds with 0 rows
