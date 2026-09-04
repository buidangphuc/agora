@buyer @needsBuyer @needsListing @needsOrder
Feature: Distributed purchase saga to PAID
  As a buyer, placing an order executes the distributed saga.

  Scenario: Placing an order runs the purchase saga to PAID
    Given a buyer has a pending order
    When the buyer places the order
    Then the order reaches PAID via the saga
