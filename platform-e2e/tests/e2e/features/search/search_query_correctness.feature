@search @buyer
Feature: Search query correctness
  As a buyer
  I want the newest sort and the rating controls to do what they say
  So that the order and the filters I see are honest

  Scenario: Newest-first lists the most recently created listing first
    Given a seller creates and publishes listing A, then listing B, then updates listing A's description, all with a shared unique keyword in the title
    When a buyer searches for that keyword with SORT_BY_NEWEST through the gateway
    Then within 30 seconds the hits are B then A

  Scenario: Newest-first holds in hybrid search
    Given a seller creates and publishes listing A, then listing B, then updates listing A's description, all with a shared unique keyword in the title
    When a buyer searches for that keyword with SORT_BY_NEWEST and SEARCH_MODE_HYBRID, then with SORT_BY_NEWEST and no search mode
    Then both responses return B then A

  Scenario: A late create event still records the creation time
    Given a seller has created and published a real listing whose title carries a unique keyword, and a listing id that the read-model does not hold
    When a ListingChanged UPDATED for that id with occurred_at T2 is published to listing.events, followed by its ListingChanged CREATED with occurred_at T1, where the real listing's creation is before T1 and T1 is before T2
    Then within 30 seconds a SORT_BY_NEWEST search for that keyword returns the out-of-order listing first, with the UPDATED event's title, and the real listing second

  Scenario: A minimum-rating search is accepted
    Given published listings matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer calls SearchListings for the keyword with min_rating 4, then with min_rating 4 and SEARCH_MODE_SEMANTIC
    Then both calls succeed and return only the listing with rating_avg 4.5

  Scenario: The ratings facet counts the floors
    Given published listings matching one keyword with rating_avg 4.5, 3.5, 2 and no review
    When a buyer calls SearchListings for the keyword with min_rating 0
    Then the ratings facet is 4 with count 1 and 3 with count 2 while the categories facet is not empty
