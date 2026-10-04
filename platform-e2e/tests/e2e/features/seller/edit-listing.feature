@seller
Feature: Seller edits a listing
  As a seller, I can update a listing so its details stay accurate.

  @needsSeller
  Scenario: Seller renames a listing
    Given a seeded seller is logged in
    And the seller has a listing
    When the seller renames the listing
    Then the renamed listing appears in the seller's listings

  @needsSeller
  Scenario: Editing someone else's listing is refused
    Given a seeded seller is logged in
    And another seller owns a listing
    When the seller opens that listing's edit page
    Then a 403 result with a link back to the seller area is shown and the form is not rendered
