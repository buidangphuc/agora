Feature: team-gateway refuses edge reflection in production
  Black box: the real team-gateway image is run with `docker run --rm` on the stack network
  with the compose environment, ENV=production and EDGE_REFLECTION_ENABLED=true.

  @destructive
  Scenario: The gateway refuses reflection in production
    When the team-gateway image is started with ENV=production and EDGE_REFLECTION_ENABLED=true
    Then the gateway process exits non-zero and its log names EDGE_REFLECTION_ENABLED
