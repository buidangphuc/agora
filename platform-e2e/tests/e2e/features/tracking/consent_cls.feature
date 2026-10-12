@tracking @analytics
Feature: The consent banner does not shift page layout
  The banner renders in a fixed-position layer so a cold load stays visually stable.

  Scenario: Banner does not shift layout
    When a visitor with no consent cookie cold-loads the home page while observing layout shift
    Then the consent banner is visible with analytics, marketing and personalization groups
    And the cumulative layout shift is below 0.1
