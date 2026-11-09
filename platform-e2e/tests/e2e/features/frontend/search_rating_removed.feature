@buyer @search
Feature: Search page rating filter
  As a buyer
  I want to narrow the search results to well-rated listings
  So that I can find products other buyers were happy with

  Scenario: The search page shows the rating filter
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer opens the search results for the rating keyword
    Then the filter sidebar shows a rating group with "Từ 4 sao" counted 1 and "Từ 3 sao" counted 2, and no link contains rating=

  Scenario: Choosing a rating filters the results by URL
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer opens the search results for the rating keyword
    And the buyer selects "Từ 4 sao" in the rating group
    Then the URL contains minRating=4 and the results list only "A"
    And "Từ 4 sao" is marked selected
    And opening the same URL in a new tab shows the same results and selection

  Scenario: The rating group keeps both options while one is selected
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer opens the search results for the rating keyword with minRating=4
    Then the rating group still shows "Từ 4 sao" and "Từ 3 sao" with the "Từ 3 sao" count equal to 2

  Scenario: The rating chip removes the filter
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer opens the search results for the rating keyword with minRating=4 on a 375 px wide viewport
    Then the drawer trigger shows 1 active filter
    When the buyer removes the rating chip
    Then the URL no longer carries minRating and the results include "C"
    And the drawer trigger shows 0 active filters

  Scenario: The rating filter is offered in the mobile drawer
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer opens the search results for the rating keyword on a 375 px wide viewport
    And the buyer opens the filter drawer and selects "Từ 3 sao"
    Then the URL contains minRating=3 and the results list "A" and "B" but not "C" or "D"

  Scenario: An unsupported minRating value is ignored
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer opens the search for the rating keyword with minRating=9, with minRating=abc and without it
    Then each page lists the same results as the plain one, with no error, empty state or rating chip

  Scenario: A repeated minRating uses the first value
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer opens the search results for the rating keyword with minRating=4 and minRating=3
    Then one rating chip is shown for "Từ 4 sao", the results list only "A" and no error is shown

  Scenario: A rating filter with no match shows the empty state
    Given published listings "A" and "C" matching one keyword with rating_avg 3.5 and 2
    When a buyer opens the search results for the rating keyword with minRating=4
    Then the search empty state is shown with the rating chip and zero results, and no error

  Scenario: An old rating link renders the results without error
    Given a published listing whose title carries a unique keyword is indexed for the rating link
    When a buyer opens the search for that keyword with rating=4 and without it
    Then the rating link lists that listing exactly as the plain one does, with no error, empty state or rating chip, and its sort links do not contain rating=
