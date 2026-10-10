Feature: Chat attempts are traced when Langfuse is enabled
  The standing e2e stack runs with Langfuse off. This feature recreates team-ai with
  compose/llm-fake-langfuse.override.yaml, so Langfuse's ingestion endpoint is the fake
  provider's own, and restores the standing configuration in teardown. Destructive: it
  reconfigures team-ai for every other scenario while it runs. Change: ai-path-resilience
  (llm-inference-resilience, requirement 8).

  @destructive
  Scenario: A chat trace carries the request id
    Given team-ai runs with Langfuse enabled against the fake provider's ingestion endpoint
    When a buyer streams a chat message with a random X-Request-Id starting "e2e-trace-"
    Then the fake ingestion endpoint receives a trace containing that request id and the buyer's id
