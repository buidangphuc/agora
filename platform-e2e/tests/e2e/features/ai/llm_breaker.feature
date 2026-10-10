Feature: Each target has a breaker that counts transient failures and recovers
  team-ai keeps a circuit breaker per target. It counts 429, 5xx, timeouts and connection
  errors, ignores request-caused 4xx, and recovers through one half-open probe after the
  cooldown. The breaker state lives in the team-ai process and the primary is driven through
  the fake's global mode, so these scenarios are destructive (serial lane). Change:
  ai-path-resilience (llm-inference-resilience, requirement 2).

  @destructive
  Scenario: Repeated 5xx opens the primary's breaker
    Given a freshly registered buyer for the LLM checks
    And every fake target is healthy and the primary's breaker is closed
    When the primary answers 500 for LLM_BREAKER_THRESHOLD consecutive chat requests and then becomes healthy
    Then the next chat request made within the cooldown is served by the first fallback without the primary being called

  @destructive
  Scenario: A request-caused 400 does not open the breaker
    Given a freshly registered buyer for the LLM checks
    And every fake target is healthy and the primary's breaker is closed
    When the primary answers 400 for LLM_BREAKER_THRESHOLD consecutive chat requests and then becomes healthy
    Then the next chat request is served by the primary

  @destructive
  Scenario: A successful probe closes the breaker
    Given a freshly registered buyer for the LLM checks
    And every fake target is healthy and the primary's breaker is closed
    And the primary's breaker has been opened by consecutive 500 answers
    When the cooldown elapses and the primary is healthy again
    Then the next chat request is served by the primary, and so is the one after it
