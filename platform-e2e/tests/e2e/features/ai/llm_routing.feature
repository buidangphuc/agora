Feature: The chat path routes each request and falls back only before the first chunk
  With CHAT_BACKEND=llm_router team-ai picks the model target per StreamChat request through
  the ordered chain primary, fb1, fb2. The calls go through the gateway; the fake provider
  (platform-e2e/fakes/llm_fake, compose/llm-fake.override.yaml) is told per request how each
  target should answer and records what it received. Change: ai-path-resilience
  (llm-inference-resilience, requirements 1 and 3).

  Scenario: A healthy primary serves the reply
    Given a freshly registered buyer for the LLM checks
    And the primary target is serving
    When the buyer streams a chat message with the fake provider scripted "primary=ok fb1=ok fb2=ok"
    Then the buyer receives one complete reply from "primary" and no error
    And the fake provider received requests for that message only from "primary"

  Scenario: A primary 429 before the first token falls back
    Given a freshly registered buyer for the LLM checks
    When the buyer streams a chat message with the fake provider scripted "primary=429 fb1=ok fb2=ok"
    Then the buyer receives one complete reply from "fb1" and no error
    And the fake provider did not call "fb2" for that message

  Scenario: The whole chain is tried in order
    Given a freshly registered buyer for the LLM checks
    When the buyer streams a chat message with the fake provider scripted "primary=500 fb1=429 fb2=ok"
    Then the buyer receives one complete reply from "fb2" and no error

  Scenario: A failure after the first chunk does not switch models
    Given a freshly registered buyer for the LLM checks
    When the buyer streams a chat message with the fake provider scripted "primary=break2 fb1=ok fb2=ok"
    Then the buyer receives those two chunks from "primary", then an error status, and no chunk from any other target

  Scenario: An exhausted chain fails cleanly
    Given a freshly registered buyer for the LLM checks
    When the buyer streams a chat message with the fake provider scripted "primary=500 fb1=500 fb2=500"
    Then the call fails with "unavailable"
    And the error message contains no exception or provider text
    And no reply chunk was streamed

  Scenario: A hung primary times out and falls back
    Given a freshly registered buyer for the LLM checks
    When the buyer streams a chat message with the fake provider scripted "primary=hang fb1=ok fb2=ok"
    Then the reply comes from "fb1" within the first-token timeout plus 3 seconds

  Scenario: Attempts are bounded
    Given a freshly registered buyer for the LLM checks
    When the buyer streams a chat message with the fake provider scripted "primary=500 fb1=500 fb2=500"
    Then the fake provider received at most LLM_MAX_ATTEMPTS requests for that message
