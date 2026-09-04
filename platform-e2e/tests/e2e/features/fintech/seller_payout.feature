@seller @needsSeller @fintech
Feature: Request a bank payout
  As a seller, I can withdraw my wallet balance to my bank account.

  Scenario: Seller requests a bank payout from wallet balance
    Given a seller has a positive wallet balance
    When the seller requests a payout
    Then the payout appears in payout history
