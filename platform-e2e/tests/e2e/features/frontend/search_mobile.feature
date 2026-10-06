@buyer @search
Feature: Search filters on a phone
  At 375px the filter column collapses behind a "Bộ lọc" button that opens a Drawer, and
  the result grid stays at two columns without horizontal scrolling.

  Background:
    Given seeded listings across categories and price ranges are indexed

  Scenario: Filters open in a drawer at 375px
    Given the viewport is a 375px wide phone
    When the buyer opens the search results with two filters active
    And the buyer taps the Bộ lọc button
    Then a Drawer opens with the filters and focus moves into it
    When the buyer presses Escape
    Then the Drawer closes and focus returns to the Bộ lọc button

  Scenario: Active count badge
    Given the viewport is a 375px wide phone
    When the buyer opens the search results with two filters active
    Then the Bộ lọc button shows a badge with 2

  Scenario: Card layout works at 375px
    Given the viewport is a 375px wide phone
    When the buyer opens the search results with two filters active
    Then every product card is laid out in two columns
    And the page has no horizontal scroll
