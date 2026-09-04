@buyer @search
Feature: Search Results Sorting
  As a buyer
  I want to sort search results by price and freshness
  So that I can quickly discover the most suitable products

  Scenario: Buyer sorts search results by price and criteria
    Given a buyer is logged in
    When the buyer searches for "iPhone"
    Then the search results sort bar displays the sorting controls
    When the buyer sorts search results by newest
    Then the search URL contains the selected sort parameter
