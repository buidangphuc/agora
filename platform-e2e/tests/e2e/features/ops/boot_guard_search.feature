Feature: team-search refuses to boot on in-memory storage in production
  Black box: the real team-search:local image is started with docker run.

  Scenario: team-search refuses in-memory storage in production
    Given the team-search image
    When it is started with ENV production and DATABASE_ENABLED false
    Then the process exits non-zero
    And its log names the storage setting
    And the same image with ENV local does not trip the storage guard
