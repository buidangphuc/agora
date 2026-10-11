@buyer @search
Feature: Search page offers no rating filter
  As a buyer
  I want the search page to offer only filters that work
  So that I am not shown a rating filter while no rating is indexed

  Scenario: The search page shows no rating filter
    Given seeded listings across categories and price ranges are indexed
    When the buyer opens the search results for the seeded listings
    Then the filter sidebar shows the category and price groups and no rating filter, and no sort or pagination link contains rating=

  Scenario: An old rating link renders the results without error
    Given a published listing whose title carries a unique keyword is indexed for the rating link
    When a buyer opens the search for that keyword with rating=4 and without it
    Then the rating link lists that listing exactly as the plain one does, with no error, empty state or rating chip, and its sort links do not contain rating=
