@seller @needsSeller @ai
Feature: Seller chat copilot
  As a seller, AI copilot suggests draft replies to buyer messages.

  Scenario: Seller uses chat copilot to generate reply drafts
    Given a seller is in a buyer conversation
    When the seller invokes the copilot
    Then a suggested reply is drafted
