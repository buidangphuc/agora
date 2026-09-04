@buyer @search
Feature: Saved searches
  As a logged-in buyer
  I want to save my current search from the search results page
  So that I can revisit it later without re-entering the query

  Scenario: A buyer saves the current search and sees it listed
    Given a logged-in buyer viewing the search results for a fresh keyword
    Then the "Tìm kiếm đã lưu" panel shows no saved searches yet
    When the buyer saves the current search
    Then the saved search for that keyword appears in the saved list

  Scenario: A saved search persists across a page reload
    Given a logged-in buyer viewing the search results for a fresh keyword
    When the buyer saves the current search
    And the buyer reloads the search results page
    Then the saved search for that keyword appears in the saved list
