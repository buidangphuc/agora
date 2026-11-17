@seller
Feature: ListMyListings filters the caller's own listings by status and title
  ListingService.ListMyListings returns only the caller's own listings, optionally narrowed by a set of
  statuses (OR) and a case-insensitive title substring (AND with the statuses). Pages stay id-ordered and
  cursor-paged, page.total counts the filtered set, and the query text is never logged.
  (domain-my-listings-filter)

  Scenario: A single status returns only that status
    Given a seller owns 3 published, 2 draft and 1 rejected listings
    When the seller lists their listings with statuses "PUBLISHED"
    Then the response holds exactly the published listings and page total is 3

  Scenario: Several statuses are OR-ed
    Given a seller owns 3 published, 2 draft and 1 rejected listings
    When the seller lists their listings with statuses "DRAFT, REJECTED"
    Then the response holds exactly the draft and rejected listings and page total is 3

  Scenario: Empty filter keeps today's behaviour
    Given a seller owns 3 published, 2 draft and 1 rejected listings
    When the seller lists their listings without a filter
    Then the response holds all 6 own listings ordered by id with page total 6

  Scenario: An unspecified status is rejected
    Given a seller owns 3 published, 2 draft and 1 rejected listings
    When the seller lists their listings with statuses "LISTING_STATUS_UNSPECIFIED"
    Then the call fails with INVALID_ARGUMENT
    And no listing is returned

  Scenario: The DELETED status matches nothing
    Given a seller owns 3 published, 2 draft and 1 rejected listings
    When the seller lists their listings with statuses "DELETED"
    Then the response holds 0 listings, page total is 0 and the next cursor is empty

  Scenario: Another seller's listings are never returned
    Given a seller owns 3 published, 0 draft and 0 rejected listings
    And another seller owns no listing
    When the other seller lists their listings with statuses "PUBLISHED"
    Then the response holds 0 listings, page total is 0 and the next cursor is empty

  Scenario: A caller without the listing.write scope is denied
    Given a seller owns 3 published, 2 draft and 1 rejected listings
    And a buyer is signed in
    When the buyer lists their listings with statuses "PUBLISHED"
    Then the call fails with PERMISSION_DENIED
    And no listing is returned

  Scenario: Substring match ignores case
    Given a seller owns published listings titled "Áo thun Đỏ", "ÁO THUN xanh", "Quần jean"
    When the seller lists their listings with query "áo thun"
    Then the response holds the listings titled "Áo thun Đỏ", "ÁO THUN xanh" and page total is 2

  Scenario: Query and status combine
    Given the seller owns a published and a draft listing both titled "Áo thun"
    When the seller lists their listings filtered by query "áo" and statuses "DRAFT"
    Then the response holds exactly the draft listings and page total is 1

  Scenario: LIKE wildcards match literally
    Given a seller owns published listings titled "Giảm 50% áo", "Giảm 500 áo"
    When the seller lists their listings with query "50%"
    Then the response holds the listings titled "Giảm 50% áo" and page total is 1

  Scenario: A query of exactly 100 runes is accepted, 101 is rejected
    Given a seller owns 3 published, 0 draft and 0 rejected listings
    When the seller lists their listings with a query of 100 runes
    And the seller lists their listings with a query of 101 runes
    Then the first call succeeds with 0 listings and the second fails with INVALID_ARGUMENT

  Scenario: No match returns an empty page
    Given a seller owns 3 published, 2 draft and 1 rejected listings
    When the seller lists their listings with query "zzzz-no-such-title"
    Then the response holds 0 listings, page total is 0 and the next cursor is empty

  Scenario: A blank query is ignored
    Given a seller owns 3 published, 2 draft and 1 rejected listings
    When the seller lists their listings with query "   "
    And the seller lists their listings without a filter
    Then the last two responses are identical and hold 6 listings

  Scenario: The query text is not logged
    Given a seller owns published listings titled "secret-needle-title", "unrelated item"
    When the seller lists their listings with query "secret-needle-title"
    Then the response holds the listings titled "secret-needle-title" and page total is 1
    And no team-domain log line since the call contains "secret-needle-title"

  Scenario: Filtered pages cover the filtered set exactly once
    Given a seller owns 250 published, 50 draft and 0 rejected listings
    When the seller pages through listings with statuses "PUBLISHED" at page size 100 until the cursor is empty
    Then the pages hold 100, 100 and 50 listings with no repeated id, no draft and every page total 250

  Scenario: Page size above the maximum is clamped
    Given a seller owns 150 published, 0 draft and 0 rejected listings
    When the seller lists their listings with page size 1000
    Then the response holds 100 listings and a non-empty next cursor

  Scenario: Filtered page latency at 2,000 listings
    Given a seller owns 2000 published listings of which every second title contains "áo"
    When the seller lists listings 50 times with statuses "PUBLISHED", query "áo" and page size 20
    Then every response holds only published listings whose title contains "áo"
    And the 95th percentile of the call durations is at most 150 ms

  Scenario: A cursor from a different filter yields no foreign rows
    Given a seller owns 4 published, 4 draft and 0 rejected listings
    And another seller owns 3 published, 0 draft and 0 rejected listings
    When the seller obtains a next cursor for statuses "DRAFT" at page size 2
    And the seller reuses that cursor with statuses "PUBLISHED"
    Then the response holds only own published listings with id greater than the cursor id
