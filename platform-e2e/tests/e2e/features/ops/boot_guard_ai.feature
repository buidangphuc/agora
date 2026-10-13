Feature: team-ai refuses unsafe settings in production
  Black box: the real team-ai image built by the stack is started with docker run. The
  rate-limit and quota boot guard of change ai-path-resilience is gone (flag-trim: the limits
  are always on, in Redis); what stays is the production-only settings validation.

  @destructive
  Scenario: Production refuses the API docs
    Given the team-ai image
    When it is started with ENV=production and DOCS_ENABLED=true
    Then the process exits non-zero and its log names DOCS_ENABLED
    And the same image with ENV=local does not trip the guard
