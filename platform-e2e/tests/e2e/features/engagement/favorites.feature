@engagement @buyer
Feature: Favorites
  As a buyer, I can favorite products so I can find them later.

  Scenario: Buyer favorites a product and finds it in favorites
    Given a buyer is logged in
    When the buyer favorites the first product in search results
    Then the product appears in the buyer's favorites

  Scenario: Unauthenticated visitor accessing favorites is redirected to login
    Given the "favorites" page is open
    Then the user is redirected to the login page
