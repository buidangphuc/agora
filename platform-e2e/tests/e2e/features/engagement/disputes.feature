@buyer @needsBuyer @needsListing @needsOrder
Feature: Dispute resolution
  As a buyer, I can open a dispute and have an admin resolve it.

  Scenario: Buyer opens a dispute and admin resolves it
    Given a buyer has an issue with an order
    When the buyer opens a dispute
    Then an admin can resolve it and the status updates
