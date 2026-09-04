@buyer
Feature: Marketplace shell
  As a visitor, the storefront shell shows me the catalog and navigation.

  @smoke
  Scenario: Home landing renders the catalog
    Given the "home" page is open
    Then the home landing shows the category bar and products

  Scenario: Global header exposes search and cart
    Given the "home" page is open
    Then the global header shows search and cart
