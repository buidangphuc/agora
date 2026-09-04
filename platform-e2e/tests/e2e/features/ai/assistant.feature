@ai @buyer
Feature: AI shopping assistant
  As a buyer, the assistant answers product questions.

  Scenario: Assistant answers a shopping question
    Given a buyer is logged in
    When I navigate to the "assistant" page
    And the buyer asks the assistant a question
    Then the assistant replies
