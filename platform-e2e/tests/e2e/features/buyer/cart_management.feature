@buyer
Feature: Cart Item Management
  As a buyer
  I want to adjust item quantities and clear items in my shopping cart
  So that I can control my purchases before checkout

  @needsListing
  Scenario: Buyer modifies item quantity in cart
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the seeded listing
    And the buyer adds the product to the cart
    And the buyer opens the cart
    Then the buyer can increase the item quantity

  @needsListing
  Scenario: Buyer clears all items from cart
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the seeded listing
    And the buyer adds the product to the cart
    And the buyer opens the cart
    When the buyer clears all items from the cart
    Then the cart displays the empty state
