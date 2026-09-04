@buyer @needsBuyer @needsListing @needsOrder
Feature: RMA return and refund
  As a buyer, I can request an RMA return and get refunded on approval.

  Scenario: Buyer submits return request and seller updates status
    Given a buyer has a pending order
    When the buyer submits a return request
    And the seller approves it
    Then the payment is refunded and stock restored
