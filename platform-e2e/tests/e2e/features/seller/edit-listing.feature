@seller
Feature: Seller edits a listing
  As a seller, I can update a listing so its details stay accurate.

  @needsSeller
  Scenario: Seller renames a listing
    Given a seeded seller is logged in
    And the seller has a listing
    When the seller renames the listing
    Then the renamed listing appears in the seller's listings
