@buyer @needsBuyer @needsListing @needsOrder
Feature: Refund on approved return
  As a buyer, an approved return triggers a payment refund.

  Scenario: Refund payment is processed on approved return
    Given a return request is approved
    When the refund is processed
    Then the buyer is refunded and the transaction is recorded
