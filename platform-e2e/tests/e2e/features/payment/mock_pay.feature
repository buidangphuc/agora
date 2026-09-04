@buyer @needsBuyer @needsListing @needsOrder
Feature: Mock payment for an order
  As a buyer, I can complete a demo payment so that the order becomes PAID.

  Scenario: Buyer completes mock payment for an order
    Given a buyer has a pending order
    When the buyer completes the demo payment
    Then the order status becomes PAID
