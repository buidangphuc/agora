@ai @tracking
Feature: Buyer AI surfaces show the grounded unary assistant answer and report provenance
  The assistant page sends one ShoppingAssistant call per question, no stream route exists, cards report
  impressions with the placement and the reply request id, and the home entry opens the assistant page.
  team-ai runs in mock mode with ASSISTANT_BACKEND=mock; needs ai-assistant-grounding and
  analytics-experiment-reports deployed on the stack (change buyer-ai-surfaces-ui).

  @needsBuyer
  Scenario: One call per question and no stream
    Given a buyer is logged in
    And the browser records assistant replies
    When the buyer opens the assistant page and asks "tai nghe dưới 500k"
    Then the assistant page made exactly 1 assistant action request and 0 stream requests
    And the assistant bubble text equals the reply text of that response

  @needsBuyer
  Scenario: The stream route is gone
    Given a buyer is logged in
    When a client posts to "/api/assistant/stream"
    Then the response status is 404

  @needsBuyer
  Scenario: Card impressions carry the placement and the request id
    Given a buyer is logged in
    And the browser records AI beacons
    And the browser records assistant replies
    When the buyer opens the assistant page and asks "tai nghe dưới 500k"
    Then every assistant card has an impression beacon with placement "assistant.suggestions"
    And every assistant impression carries an impressionId equal to the reply request id and a non-empty modelVersion
    And the assistant impression positions are 1 up to the number of cards


  Scenario: The home entry points to the assistant page
    When a visitor opens the home page
    Then the assistant entry is a link to "/assistant"
    When the visitor activates the assistant entry
    Then the visitor lands on the assistant page
