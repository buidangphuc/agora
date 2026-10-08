Feature: Token usage is metered against quota and chat RPCs are rate limited per principal
  team-ai logs the provider-reported token usage of each completed reply, reserves and
  refunds a per-principal quota around the model call, and limits StreamChat per forwarded
  principal. The e2e stack's quota and limit come from compose/llm-fake.override.yaml; every
  scenario registers its own buyers. Change: ai-path-resilience (llm-inference-resilience,
  requirements 4 and 5).

  Scenario: Usage is recorded for a completed reply
    Given a freshly registered buyer for the LLM checks
    When the buyer's chat reply completes and the fake provider reports 11 input and 7 output tokens
    Then team-ai's log has one usage line for that request id with 11 input tokens, 7 output tokens and the target

  Scenario: An exhausted quota refuses the call
    Given a buyer who has used up the per-principal chat quota
    When the buyer streams another chat message
    Then the call fails with "resource_exhausted"
    And the fake provider received no request for that message

  Scenario: A failed call does not consume quota
    Given a buyer with quota for exactly one more reply
    When the buyer streams a message for which every target answers 500, then streams a message with healthy targets
    Then the first call fails with "unavailable" and the second reply completes

  Scenario: One buyer over the limit does not affect another
    Given a freshly registered buyer for the LLM checks
    And a second freshly registered buyer for the LLM checks
    When the buyer streams more chat messages within a minute than the per-principal limit
    Then the calls past the limit fail with "resource_exhausted" without reaching the provider
    And a different buyer's chat message right after succeeds
