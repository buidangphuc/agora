@buyer
Feature: Search and Discovery Experience
  As a buyer
  I want to search products with filters and sorting
  So that I can easily discover deals and items

  @smoke @needsListing
  Scenario: Buyer discovers products through search and filter options
    When the buyer searches for "iPhone"
    Then the search results page displays matching products
