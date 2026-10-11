@buyer @search
Feature: Dynamic facets on the search page
  As a buyer
  I want the search page to offer the variant options of the results
  So that I can narrow by color or capacity and share the URL

  Scenario: Selecting a Dynamic Facet Narrows Results and Reflects in the URL
    Given published listings with different variant colors indexed for a keyword
    When a buyer opens /search for that keyword and selects the xanh-navy bucket of the color facet
    Then the URL contains sku.color=xanh-navy, only listings with an in-stock navy variant remain, and the selected value is shown as an active filter chip
