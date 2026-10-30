Feature: team-search refuses to boot without a reachable database outside local
  Black box: the real team-search image built by the stack is started with docker run.

  Scenario: team-search refuses to boot without a reachable database in production
    Given the team-search image
    When it is started with ENV production and no DATABASE_URL
    Then the process exits non-zero
    And its log names the database setting
    And the same image with ENV production and an unreachable database exits non-zero
    And the same image with ENV local and an unreachable database exits non-zero

  Scenario: team-search refuses in-memory storage in production
    Given the team-search image
    When it is started with ENV production and an empty DATABASE_URL
    Then the process exits non-zero
    And its log names the database setting
