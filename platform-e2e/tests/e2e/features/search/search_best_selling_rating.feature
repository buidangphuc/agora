@search @buyer
Feature: Search best-selling sort, minimum rating and projected sales and ratings
  As a buyer
  I want to sort by units sold and filter by rating
  So that I find popular and well-reviewed listings

  Scenario: A sales event sets the sold count within 30 seconds
    Given a published listing "L" that is indexed
    When a ListingSalesChanged for "L" with sold_count 5 and changed_at T1 is published to order.events
    Then within 30 seconds the hit of "L" carries sold_count 5

  Scenario: A rating event sets the average and the review count
    Given a published listing "L" that is indexed
    When a ListingRatingChanged for "L" with average_rating 4.5, review_count 2 and changed_at T1 is published to engagement.events
    Then within 30 seconds the hit of "L" carries rating_avg 4.5 and review_count 2

  Scenario: A later lower sold count replaces the earlier value
    Given a published listing "L" that is indexed
    And the listing "L" has sold_count 5 from a sales event at T1
    When a ListingSalesChanged for "L" with sold_count 3 and changed_at T2 is published to order.events
    Then within 30 seconds the hit of "L" carries sold_count 3

  Scenario: A redelivered or older event changes nothing
    Given a published listing "L" that is indexed
    And the listing "L" has sold_count 5 from a sales event at T2
    When a ListingSalesChanged for "L" with sold_count 9 and changed_at T1 is published and the T2 event is delivered again
    Then after the indexer has consumed the published events, the hit of "L" still carries sold_count 5

  Scenario: Values for a not-yet-indexed listing are merged on creation
    Given a listing id "G" that has no search document and a control listing "C" that is indexed
    When a ListingSalesChanged for "G" with sold_count 4 and changed_at T1 is published to order.events
    And a ListingRatingChanged for "G" with average_rating 4, review_count 1 and changed_at T1 is published to engagement.events
    Then after the indexer has consumed the published events, a keyword search returns no hit for "G" and counts only "C"
    When a ListingChanged CREATED for "G" is published to listing.events
    Then within 30 seconds the hit of "G" carries sold_count 4
    And within 30 seconds the hit of "G" carries rating_avg 4 and review_count 1

  Scenario: A listing update does not reset the projected values
    Given a published listing "L" that is indexed
    And the listing "L" has sold_count 5 from a sales event at T1
    And the listing "L" has rating_avg 4.5 and review_count 2 from a rating event at T1
    When a ListingChanged UPDATED for "L" with a new title is published to listing.events
    Then within 30 seconds the read-model title of "L" is the new title
    And within 30 seconds the hit of "L" carries sold_count 5
    And within 30 seconds the hit of "L" carries rating_avg 4.5 and review_count 2

  Scenario: A malformed event is parked and not applied
    Given a published listing "L" that is indexed
    And the listing "L" has sold_count 2 from a sales event at T1
    And the listing "L" has rating_avg 4 and review_count 1 from a rating event at T1
    When a ListingSalesChanged with sold_count -1 for "L" is published to order.events and a ListingRatingChanged with average_rating 7 for "L" is published to engagement.events
    Then within 60 seconds the sales event is on order.events.dlq and the rating event is on engagement.events.dlq
    And the hit of "L" still carries sold_count 2 and rating_avg 4

  Scenario: An event with an empty listing id or no changed_at is parked
    Given a published listing "L" that is indexed
    And the listing "L" has rating_avg 4 and review_count 1 from a rating event at T1
    When a ListingSalesChanged with an empty listing_id and a ListingRatingChanged for "L" with no changed_at are published
    Then within 60 seconds the sales event is on order.events.dlq and the rating event is on engagement.events.dlq
    And the hit of "L" still carries rating_avg 4

  Scenario: An event not later than the delete is ignored
    Given a published listing "L" and a control listing "C" that are indexed
    And "L" is deleted through the gateway
    When a ListingSalesChanged for "L" with sold_count 2 and a ListingRatingChanged for "L" with average_rating 4 and review_count 1, both with changed_at earlier than the delete, are published
    Then after the indexer has consumed the published events, "L" is absent from the hits, the total and the facets of a keyword search
    When a ListingChanged CREATED for "L" with occurred_at after the delete is published to listing.events
    Then within 30 seconds the hit of "L" carries sold_count 0 and review_count 0

  Scenario: Best-selling lists the highest sold count first
    Given published listings "A", "B" and "C" matching one keyword with sold counts 7, 0 and 3
    When a buyer calls SearchListings for the keyword with SORT_BY_BEST_SELLING
    Then the hits are A, C, B in that order

  Scenario: Ties are broken by creation time then id
    Given listings "D", "E" and "F" with sold_count 2, where D was created at 10:00:00, E and F at 10:05:00, and the id of E sorts before the id of F
    When a buyer calls SearchListings for the keyword with SORT_BY_BEST_SELLING
    Then the hits are E, F, D in that order

  Scenario: Best-selling holds in semantic and hybrid mode
    Given published listings "A", "B" and "C" matching one keyword with sold counts 7, 0 and 3
    And a published listing "N" that does not match the keyword and has sold_count 50
    When a buyer calls SearchListings for the keyword with SORT_BY_BEST_SELLING and SEARCH_MODE_SEMANTIC, then with SEARCH_MODE_HYBRID
    Then the hits are A, C, B in that order

  Scenario: Best-selling does not show a draft to a stranger
    Given a draft listing "D" owned by seller S with sold_count 100 and a published listing "P" with sold_count 1
    When a buyer calls SearchListings for the keyword with SORT_BY_BEST_SELLING
    Then "D" is not among the hits and "P" is

  Scenario: A minimum-rating search returns only listings at or above it
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer calls SearchListings for the keyword with min_rating 4, then with min_rating 3
    Then the first response returns only A and the second returns only A, B

  Scenario: A rating at the boundary is included
    Given a published listing "L" with rating_avg 4 and review_count 1
    When a buyer calls SearchListings for the keyword with min_rating 4
    Then "L" is among the hits

  Scenario: Unreviewed listings are excluded by any rating filter
    Given a published listing "U" with rating_avg 0 and review_count 0
    And a published listing "R" with rating_avg 4 and review_count 1
    When a buyer calls SearchListings for the keyword with min_rating 3
    Then "U" is not among the hits and the total counts only R

  Scenario: The rating filter works in semantic and hybrid mode and in a saved search
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    And a saved search for the keyword with min_rating 4
    When a buyer calls SearchListings with min_rating 4 and SEARCH_MODE_SEMANTIC, then with SEARCH_MODE_HYBRID, and runs the saved search
    Then every response returns only A

  Scenario: An out-of-range minimum rating is rejected
    When a buyer calls SearchListings through the gateway with min_rating 6, then with min_rating -1
    Then both calls fail with invalid_argument
    And a call with min_rating 5 and a call with min_rating 0 are accepted

  Scenario: The ratings facet ignores its own filter
    Given published listings "A", "B", "C" and "D" matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer calls SearchListings for the keyword with min_rating 4
    Then the response returns only A
    And the ratings facet is 4 with count 1 and 3 with count 2
    And the categories facet counts 1 listing

  Scenario: A hit shows the indexed values
    Given a published listing "L" with sold_count 5, rating_avg 4.5 and review_count 2
    When a buyer calls SearchListings for the title of "L"
    Then the hit of "L" has sold_count 5, rating_avg 4.5 and review_count 2

  Scenario: A listing without sales or reviews shows zeros
    Given a published listing "L" that no event has touched and a published listing "M" with sold_count 1
    When a buyer calls SearchListings for the title of "L"
    Then the hit of "L" has sold_count 0, rating_avg 0 and review_count 0 while the hit of "M" has sold_count 1
