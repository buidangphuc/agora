@shop @needsSeller
Feature: Shop display name
  As a buyer
  I want shops to show their real name instead of an id
  So that I can recognise a shop in its page and my cart

  Scenario: The name is returned for a storefront
    Given the seller sets the shop display name to "  Tiem Hoa Nho  "
    When a visitor opens the seller's shop page
    Then the shop header shows "Tiem Hoa Nho"
    And the storefront API returns the display name "Tiem Hoa Nho"

  Scenario: A name change is reflected
    Given the seller sets the shop display name to "Tiem Hoa Nho"
    And the seller sets the shop display name to "Tiem Hoa Lon"
    When a visitor opens the seller's shop page
    Then the shop header shows "Tiem Hoa Lon"
    And the batch lookup returns the display name "Tiem Hoa Lon" for the seller

  Scenario: A cart with two sellers returns both names in one batch
    Given the seller sets the shop display name to "Shop Alpha"
    And a second seller sets the shop display name to "Shop Beta"
    And a buyer has one listing from each seller in the cart
    When the cart sellers are resolved with a single batch lookup
    Then the batch lookup returns "Shop Alpha" and "Shop Beta" for the two cart sellers

  Scenario: Fallback when the seller has no name
    Given the seller has no storefront
    When a visitor opens the seller's shop page
    Then the shop header shows the fallback label "Shop #" followed by the first 6 characters of the seller id
