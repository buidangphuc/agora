@fintech @buyer
Feature: The local stack settles checkouts through the opt-in mock payment
  ProcessMockPayment is refused unless team-payment runs with MOCK_PAYMENTS=true. The
  local compose stack enables it, so a checkout settles end to end.

  Scenario: The local stack settles a checkout through the mock payment
    Given a commerce seller with a published listing priced 1000000 with stock 10
    And a commerce buyer with a saved address
    When the buyer checks out 1 of the listing and pays with the mock payment
    Then the payment succeeds
    And the order becomes paid
