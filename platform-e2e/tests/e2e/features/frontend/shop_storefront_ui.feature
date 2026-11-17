@shop @needsSeller
Feature: Shop page featured listings and theme
  The public shop page (change shop-page-featured-theme) shows the storefront's featured listings
  above the grid and applies its theme to the hero header. Everything is seeded through the real
  gateway API and read back in the browser.

  Scenario: Featured listings appear first in the chosen order
    Given the seller owns 4 published listings
    And the seller features the listings in the order "B,A,C"
    When a visitor opens the shop page once the shop grid lists listing "D"
    Then the featured block shows the listings "B,A,C" in that order
    And the main grid contains none of the listings "A,B,C"

  Scenario: A listing unpublished after being featured is skipped
    Given the seller owns 3 published listings
    And the seller features the listings in the order "A,B,C"
    And the seller moves listing "B" to draft
    When a visitor opens the shop page once the shop grid lists listing "A"
    Then the shop page responded 200 without error text
    And the featured block shows the listings "A,C" in that order
    And listing "B" appears nowhere on the shop page

  Scenario: No featured listings means no block
    Given the seller owns 2 published listings
    When a visitor opens the shop page once the shop grid lists listing "A"
    Then the shop page has no featured block
    And the main grid contains all of the listings "A,B"

  Scenario: A themed shop renders its theme
    Given the seller owns 1 published listings
    And the seller sets the storefront theme to "festive"
    And a second seller with a default theme storefront
    When a visitor opens the shop page
    Then the shop header card has data-theme "festive"
    When a visitor opens the second seller's shop page
    Then the shop header card has data-theme "default"
    And the two shop header backgrounds differ
