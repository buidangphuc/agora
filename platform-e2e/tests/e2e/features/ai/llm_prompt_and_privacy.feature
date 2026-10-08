Feature: The model input carries a system prompt and bounded session history, and personal data is redacted
  What the model receives is read from the fake provider's record of each request. Calls go
  through the gateway with Connect streaming on ChatService/StreamChat; the assistant log
  scenario uses ShoppingAssistant. Change: ai-path-resilience (llm-inference-resilience,
  requirements 6 and 7).

  Scenario: The system prompt is always first
    Given a freshly registered buyer for the LLM checks
    When the buyer streams a chat message
    Then the request the fake provider received starts with a system message equal to the configured system prompt

  Scenario: A follow-up carries the session history
    Given a freshly registered buyer for the LLM checks
    And a chat session id for the history checks
    When the buyer streams "first question" and then "second question" with the same session_id
    Then the second request the fake provider received contains, after the system message, the first user turn, the first assistant reply and the second user turn, in that order

  Scenario: History is bounded
    Given a freshly registered buyer for the LLM checks
    And a chat session id for the history checks
    When the buyer streams more messages in one session than CHAT_HISTORY_MAX_TURNS
    Then the last request the fake provider received contains only the most recent CHAT_HISTORY_MAX_TURNS earlier turns

  Scenario: A failed reply is not kept as history
    Given a freshly registered buyer for the LLM checks
    And a chat session id for the history checks
    When the buyer streams a message for which every target answers 500, then streams a follow-up with the same session_id
    Then the follow-up request the fake provider received contains no turn from the failed message

  Scenario: Phone and citizen ID are masked before the model
    Given a freshly registered buyer for the LLM checks
    When the buyer streams "gọi 0912 345 678, CCCD 079123456789"
    Then the request the fake provider received contains redaction markers and neither "0912 345 678" nor "079123456789"

  Scenario: Prices and order numbers are not masked
    Given a freshly registered buyer for the LLM checks
    When the buyer streams "giá 850000000, order id 123456789"
    Then the request the fake provider received contains "850000000" and "123456789"

  Scenario: Assistant logs carry redacted text
    Given a freshly registered buyer for the LLM checks
    When the buyer asks the Shopping Assistant through the gateway a question containing "a.b@example.com"
    Then no team-ai log line contains "a.b@example.com"
