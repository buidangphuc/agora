Feature: team-order refuses to boot without a database in a shared environment
  Black box: the real team-order image (team-order:local) is started on its own with docker run --rm,
  without touching the running stack, with every other required production setting present so the
  database setting is what is refused.

  Scenario: team-order refuses to start without a database in production
    Given the team-order image
    When it is started with ENV production and no DATABASE_URL
    Then the process exits non-zero
    And its log names DATABASE_URL
