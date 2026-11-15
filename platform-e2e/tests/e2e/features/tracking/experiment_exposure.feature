@tracking @analytics
Feature: The collector stores experiment exposure beacons
  POST /api/track accepts `experiment_exposure` with exactly the properties flag and variant and
  team-analytics stores it with event_type experiment_exposure. Scenario titles equal the
  `#### Scenario:` titles of change analytics-experiment-reports.

  Scenario: A valid exposure is stored
    When a visitor posts an experiment_exposure beacon with a fresh flag and variant "treatment"
    Then the warehouse holds one row with event_type "experiment_exposure" and exactly those two properties
