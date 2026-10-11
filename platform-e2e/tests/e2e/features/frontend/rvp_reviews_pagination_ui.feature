@buyer @engagement
Feature: The product page requests the review page from the server
  A listing with 105 reviews shows every page, including the oldest reviews.

  Scenario: A listing with 105 reviews shows 11 pages
    Given a listing with 105 reviews and a buyer signed in
    When the buyer opens the listing page
    Then the reviews list shows 10 reviews and the pagination offers 11 pages

  Scenario: The last page shows the oldest reviews
    Given a listing with 105 reviews and a buyer signed in
    When the buyer opens the reviews page 11 of the listing
    Then 5 reviews are listed, they are the 5 oldest and page 11 is current
