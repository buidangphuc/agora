Feature: team-ai refuses a per-process rate limiter in production
  Black box: the real team-ai image built by the stack is started with docker run. Change:
  ai-path-resilience (llm-inference-resilience, requirement 5).

  @destructive
  Scenario: Production refuses a per-process rate limiter
    Given the team-ai image
    When it is started with ENV=production, GRPC_RATE_LIMIT_ENABLED=true and RATE_LIMIT_BACKEND=memory
    Then the process exits non-zero and its log names RATE_LIMIT_BACKEND
    And the same image with ENV=local does not trip the guard
