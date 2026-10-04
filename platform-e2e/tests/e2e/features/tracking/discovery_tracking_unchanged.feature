@tracking @buyer
Feature: Discovery tracking is unchanged
  The rebuilt listing card and search page still emit the same ecommerce events: one
  view_item_list per card entering the viewport, one select_item on click, and one batched
  search_results impression, all with their position (index = position in the grid).

  Background:
    Given seeded listings across categories and price ranges are indexed

  Scenario: Card impression still fires once
    When the buyer opens the search results for the seeded listings
    And the first result card is scrolled into view
    Then exactly one view_item_list event for that card is recorded with its position

  Scenario: Card click still records select_item
    When the buyer opens the search results for the seeded listings
    And the first result card is scrolled into view
    And the buyer clicks the image of that card
    Then one select_item event for that card is recorded with its position

  Scenario: Search results still emit a batched impression
    When the buyer opens the search results for the seeded listings
    Then exactly one batched search_results impression carries every rendered result
