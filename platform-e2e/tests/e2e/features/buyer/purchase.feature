@buyer
Feature: Buyer purchase journey
  As a shopper, I can find a product and reach checkout so that I can buy it.

  @smoke
  Scenario: Buyer searches, opens a product and reaches checkout
    Given a buyer is logged in
    When the buyer searches for "laptop"
    And the buyer opens the first search result
    And the buyer adds the product to the cart
    And the buyer opens the cart
    And the buyer proceeds to checkout
    Then the checkout page is displayed

  @needsListing
  Scenario: A seeded product is purchasable
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the seeded listing
    Then the add-to-cart action is available
