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

  # The gateway cannot be slowed from the browser, so the streaming guarantee is asserted
  # through its observable effect: the hero paints first and nothing shifts once the
  # skeletons are replaced by the real blocks.
  Scenario: Slow block does not block the page
    Given the "home" page is open
    Then the home page settles without layout shift

  Scenario: No fabricated markers on the home page
    Given the "home" page is open
    Then the home page shows no fabricated commerce markers

  Scenario: Selecting a category navigates by URL
    Given the "home" page is open
    When the visitor activates the first category on the home grid
    Then the browser navigates to the search page filtered by that category
