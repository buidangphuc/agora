@seller
Feature: Seller listing management
  As a seller, I can publish a listing so that buyers can find my product.

  @needsSeller
  Scenario: Seller creates a new listing
    Given a seeded seller is logged in
    When the seller creates a new listing
    Then the new listing appears in the seller's listings
