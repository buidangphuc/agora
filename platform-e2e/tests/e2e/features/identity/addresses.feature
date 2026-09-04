@buyer
Feature: Delivery addresses
  As a buyer, I can save a delivery address for checkout.

  Scenario: Buyer adds a delivery address
    Given a buyer is logged in
    When I navigate to the "addresses" page
    And the buyer adds a delivery address
    Then the new address appears in the address list
