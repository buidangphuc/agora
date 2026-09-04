@buyer @search
Feature: Faceted search filters
  As a buyer
  I want the search results page to show facets with counts and let me filter by them
  So that I can narrow a large result set to the products I care about

  Background:
    Given seeded listings across categories and price ranges are indexed

  Scenario: Facet buckets render with counts
    When the buyer opens the search results for the seeded listings
    Then the facet sidebar shows category and price buckets with counts

  Scenario: Selecting a price-range facet narrows the results
    When the buyer opens the search results for the seeded listings
    And the buyer selects the "100000-500000" price-range facet
    Then the results narrow to the listings in that price range
    And the search URL reflects the selected price filter
    And the facet counts update to reflect the narrowed set
