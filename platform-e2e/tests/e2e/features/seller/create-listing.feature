@seller
Feature: Seller listing management
  As a seller, I can publish a listing so that buyers can find my product.

  @needsSeller
  Scenario: Seller creates a new listing
    Given a seeded seller is logged in
    When the seller creates a new listing
    Then the new listing appears in the seller's listings

  @needsSeller
  Scenario: A required-field error blocks the save
    Given a seeded seller is logged in
    When the seller submits the empty new listing form
    Then the title field shows a required error and no listing is created

  @needsSeller
  Scenario: A saved listing shows a success toast and a link to the list
    Given a seeded seller is logged in
    When the seller creates a new listing
    Then a success toast confirms the save
    And a success state offers a link to the seller's listings
