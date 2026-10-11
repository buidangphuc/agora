Feature: AI generation calls are sent once and reflection is off by default
  team-gateway forwards the AI generation procedures exactly once (no retry on unavailable)
  and reports the upstream attempt count on its edge.request log line; gRPC reflection is
  not served unless EDGE_REFLECTION_ENABLED is set. Change: port-edge-authz-residuals
  (edge-stream-and-http-policy).

  # Stops team-ai for real; it is started again and awaited in teardown.
  @destructive
  Scenario: A generation call is not retried when team-ai is down
    Given a logged-in seller for the edge AI checks
    And team-ai is stopped for the edge AI checks
    When the seller calls MagicListing through the gateway with a random X-Request-Id starting "e2e-ai-"
    Then the call fails with unavailable
    And the gateway's edge.request log line for that request id reports one upstream attempt

  Scenario: Reflection is not served by default
    When a client posts a reflection request to /grpc.reflection.v1.ServerReflection/ServerReflectionInfo on the local gateway
    Then the gateway answers HTTP 404
