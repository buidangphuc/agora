## ADDED Requirements

### Requirement: The indexer sets sold count and rating from source events

The team-search indexer SHALL consume `order.events/ListingSalesChanged` and `engagement.events/ListingRatingChanged`
and SHALL set `sold_count`, or `rating_avg` and `review_count`, on the listing's search document by partial update,
using the event's absolute values. It SHALL apply an event only when its `changed_at` is newer than the one already
stored for that field group, so a redelivered or older event changes nothing. For a listing whose document does not
exist yet it SHALL keep the values as pending and merge them when the listing is first upserted. It SHALL ignore an event for a deleted listing whose `changed_at` is not later than the deletion time, and SHALL keep one with a later `changed_at` as pending values to merge on the listing's next upsert, and SHALL reject an event with an empty `listing_id`, a negative `sold_count` or
`review_count`, an `average_rating` outside 0 to 5, or no `changed_at` by returning an error so that it is retried and
parked on the DLQ, leaving the document untouched.

#### Scenario: BR-1 A sales event sets the sold count within 30 seconds

- **GIVEN** a published listing L is indexed with `sold_count` 0
- **WHEN** `ListingSalesChanged{listing_id: L, sold_count: 5}` is published to `order.events`
- **THEN** within 30 seconds a `SearchListings` call that returns L carries `sold_count` 5 on the hit of L
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "A sales event sets the sold count within 30 seconds"

#### Scenario: BR-2 A rating event sets the average and the review count

- **GIVEN** a published listing L is indexed
- **WHEN** `ListingRatingChanged{listing_id: L, average_rating: 4.5, review_count: 2}` is published to `engagement.events`
- **THEN** within 30 seconds the hit of L carries `rating_avg` 4.5 and `review_count` 2
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "A rating event sets the average and the review count"

#### Scenario: BR-3 A later lower sold count replaces the earlier value

- **GIVEN** L has `sold_count` 5 from an event at time T1
- **WHEN** `ListingSalesChanged{sold_count: 3}` with `changed_at` T2 later than T1 is published
- **THEN** within 30 seconds the hit of L carries `sold_count` 3
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "A later lower sold count replaces the earlier value"

#### Scenario: BR-4 A redelivered or older event changes nothing

- **GIVEN** L has `sold_count` 5 from an event at T2
- **WHEN** `ListingSalesChanged{sold_count: 9}` with `changed_at` T1 earlier than T2 is published, and the T2 event is delivered again
- **THEN** the hit of L still carries `sold_count` 5 after 30 seconds
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "A redelivered or older event changes nothing"

#### Scenario: BR-5 Values for a not-yet-indexed listing are merged on creation

- **GIVEN** a listing id L that has no search document
- **WHEN** `ListingSalesChanged{sold_count: 4}` and `ListingRatingChanged{average_rating: 4, review_count: 1}` for L are published, and then a `ListingChanged` CREATED for L (published) is published
- **THEN** within 30 seconds the hit of L carries `sold_count` 4, `rating_avg` 4 and `review_count` 1, and between the events and the creation a search for the keyword of L's future title returns no hit and the total does not count L
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "Values for a not-yet-indexed listing are merged on creation"

#### Scenario: BR-6 A listing update does not reset the projected values

- **GIVEN** L has `sold_count` 5 and `rating_avg` 4.5
- **WHEN** a `ListingChanged` UPDATED for L with a new title is published
- **THEN** within 30 seconds the hit of L carries the new title and still `sold_count` 5 and `rating_avg` 4.5
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "A listing update does not reset the projected values"

#### Scenario: BR-7 A malformed event is parked and not applied

- **WHEN** a `ListingSalesChanged` with `sold_count` -1 is published to `order.events` and a `ListingRatingChanged` with `average_rating` 7 is published to `engagement.events`, for a published listing L
- **THEN** within 60 seconds on the e2e stack (5 handler attempts, the go-sdk consumer default `MaxAttempts`, with the stack's consumer backoff) the first record is on `order.events.dlq` and the second on `engagement.events.dlq`, and L's `sold_count` and `rating_avg` are unchanged
- **VERIFIED BY**: `team-search/internal/events` unit tests for the new handlers and `platform-e2e` `search/search_best_selling_rating.feature` "A malformed event is parked and not applied"

#### Scenario: BR-7a An event with an empty listing id or no changed_at is parked

- **WHEN** a `ListingSalesChanged` with an empty `listing_id` and a `ListingRatingChanged` for a published listing L with no `changed_at` are published
- **THEN** within 60 seconds on the e2e stack both reach their `.dlq` topic after 5 attempts and L's `rating_avg` is unchanged
- **VERIFIED BY**: `team-search/internal/events` unit tests and `platform-e2e` `search/search_best_selling_rating.feature` "An event with an empty listing id or no changed_at is parked"

#### Scenario: BR-4a An event with the same changed_at as the stored one changes nothing

- **GIVEN** L has `rating_avg` 4.5 and `review_count` 2 from an event at T
- **WHEN** a `ListingRatingChanged{average_rating: 1, review_count: 9}` with `changed_at` exactly T is published
- **THEN** the hit of L still carries `rating_avg` 4.5 and `review_count` 2 after 30 seconds
- **VERIFIED BY**: `team-search/internal/events` unit tests "equal changed_at is a no-op"

#### Scenario: BR-8 An event not later than the delete is ignored

- **GIVEN** listing L was deleted from the index (tombstoned) at time T
- **WHEN** `ListingSalesChanged{sold_count: 2}` and `ListingRatingChanged{average_rating: 4, review_count: 1}` for L with `changed_at` earlier than T are published
- **THEN** L stays absent from search results, totals and facets after 30 seconds, and a later `ListingChanged` CREATED for L with `occurred_at` after T shows `sold_count` 0 and `review_count` 0
- **VERIFIED BY**: `team-search/internal/events` unit tests and `platform-e2e` `search/search_best_selling_rating.feature` "An event not later than the delete is ignored"

#### Scenario: BR-8a A listing recreated after a delete takes the pending values

- **GIVEN** L was deleted and tombstoned at time T
- **WHEN** a newer `ListingChanged` CREATED for L arrives after `ListingSalesChanged{sold_count: 6}` and `ListingRatingChanged{average_rating: 5, review_count: 1}` for L were published with `changed_at` later than T
- **THEN** within 30 seconds the hit of L carries `sold_count` 6, `rating_avg` 5 and `review_count` 1, as with a new listing
- **VERIFIED BY**: `team-search/internal/index` integration test "recreate after delete merges pending values"

### Requirement: Best-selling sort orders by sold count

`SearchListings` and `RunSavedSearch` with `SORT_BY_BEST_SELLING` SHALL order hits by `sold_count` descending, then
`created_at` descending, then `id` ascending, in every search mode, running only the lexical leg like the other
explicit sorts. A listing with no recorded sales SHALL count as `sold_count` 0 and sort after listings with sales.

#### Scenario: BR-9 Best-selling lists the highest sold count first

- **GIVEN** published listings A, B and C matching one keyword with `sold_count` 7, 0 and 3
- **WHEN** a buyer calls `SearchListings` for the keyword with `SORT_BY_BEST_SELLING`
- **THEN** the hits are returned in the order A, C, B
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "Best-selling lists the highest sold count first"

#### Scenario: BR-10 Ties are broken by creation time then id

- **GIVEN** listings D, E and F, all with `sold_count` 2; D was created at 10:00:00, E and F at 10:05:00, and the id of E sorts before the id of F
- **WHEN** a buyer calls `SearchListings` with `SORT_BY_BEST_SELLING`
- **THEN** the order is E, F, D
- **VERIFIED BY**: `team-search/internal/index` unit test `sortClause` and `platform-e2e` `search/search_best_selling_rating.feature` "Ties are broken by creation time then id"

#### Scenario: BR-11 Best-selling holds in semantic and hybrid mode

- **GIVEN** the listings of BR-9
- **WHEN** a buyer calls `SearchListings` with `SORT_BY_BEST_SELLING` and `SEARCH_MODE_SEMANTIC`, then with `SEARCH_MODE_HYBRID`
- **THEN** both calls return exactly the matching listings A, C, B, in that order, and no non-matching listing
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "Best-selling holds in semantic and hybrid mode"

#### Scenario: BR-12 Best-selling does not show a draft to a stranger

- **GIVEN** a draft listing with the highest `sold_count` owned by seller S
- **WHEN** a buyer who is not S calls `SearchListings` with `SORT_BY_BEST_SELLING`
- **THEN** the draft is not among the hits, as with every other sort
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "Best-selling does not show a draft to a stranger"

### Requirement: Search accepts a minimum rating and fills the ratings facet

`SearchListings` and `RunSavedSearch` SHALL accept `min_rating` from 0 to 5 in every search mode and SHALL return only
listings whose `rating_avg` is at least `min_rating`; a listing with `review_count` 0 SHALL be excluded by any
`min_rating` above 0, and `min_rating` 0 SHALL apply no filter. A `min_rating` below 0 or above 5 SHALL fail with
`INVALID_ARGUMENT` before any retrieval. `facets.ratings` SHALL hold the buckets `"4"` and `"3"`, each counting the
listings that match every other filter and the query, with `rating_avg` at or above that floor; the facet SHALL ignore
`min_rating` itself. `RunSavedSearch` SHALL apply the same rules to the saved search's `min_rating`, and an out-of-range
value there SHALL fail with `INVALID_ARGUMENT`.

#### Scenario: BR-13 A minimum-rating search returns only listings at or above it

- **GIVEN** published listings matching one keyword with `rating_avg` 4.5, 3.5, 2 and one with no review
- **WHEN** a buyer calls `SearchListings` for the keyword with `min_rating` 4, then with `min_rating` 3
- **THEN** the first call returns only the 4.5 listing and the second returns the 4.5 and 3.5 listings
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "A minimum-rating search returns only listings at or above it"

#### Scenario: BR-14 A rating at the boundary is included

- **GIVEN** a published listing with `rating_avg` exactly 4
- **WHEN** a buyer calls `SearchListings` with `min_rating` 4
- **THEN** the listing is among the hits
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "A rating at the boundary is included"

#### Scenario: BR-15 Unreviewed listings are excluded by any rating filter

- **GIVEN** a published listing with `review_count` 0
- **WHEN** a buyer calls `SearchListings` with `min_rating` 3
- **THEN** the listing is not among the hits and is not counted in `total`
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "Unreviewed listings are excluded by any rating filter"

#### Scenario: BR-16 The rating filter works in semantic and hybrid mode and in a saved search

- **GIVEN** the listings of BR-13 and a saved search for the keyword with `min_rating` 4
- **WHEN** a buyer calls `SearchListings` with `min_rating` 4 and `SEARCH_MODE_SEMANTIC`, then `SEARCH_MODE_HYBRID`, and runs the saved search
- **THEN** each of the three responses returns only the 4.5 listing
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "The rating filter works in semantic and hybrid mode and in a saved search"

#### Scenario: BR-17 An out-of-range minimum rating is rejected

- **WHEN** a buyer calls `SearchListings` through the gateway with `min_rating` 6, then with `min_rating` -1
- **THEN** both calls fail with `invalid_argument`
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "An out-of-range minimum rating is rejected"

#### Scenario: BR-18 The ratings facet counts the floors

- **GIVEN** the listings of BR-13
- **WHEN** a buyer calls `SearchListings` for the keyword with `min_rating` 0
- **THEN** `facets.ratings` is `[{key "4", count 1}, {key "3", count 2}]`
- **VERIFIED BY**: `team-search/internal/index` integration test and `platform-e2e` `search/search_query_correctness.feature` "The ratings facet is filled"

#### Scenario: BR-18a The ratings facet ignores its own filter

- **GIVEN** the listings of BR-13
- **WHEN** a buyer calls `SearchListings` for the keyword with `min_rating` 4
- **THEN** `hits` holds only the 4.5 listing and `facets.ratings` is still `[{key "4", count 1}, {key "3", count 2}]`, while `facets.categories` counts only the 4.5 listing
- **VERIFIED BY**: `team-search/internal/index` integration test and `platform-e2e` `search/search_best_selling_rating.feature` "The ratings facet ignores its own filter"

#### Scenario: BR-18b An invalid minimum rating fails a saved-search run

- **GIVEN** a saved search whose stored `min_rating` is 7
- **WHEN** a buyer runs it with `RunSavedSearch`
- **THEN** the call fails with `invalid_argument` and no retrieval runs
- **VERIFIED BY**: `team-search/internal/rpc` unit test "RunSavedSearch rejects an out-of-range stored min_rating"

### Requirement: Search hits carry sold count and rating

Every `SearchHit` returned by `SearchListings` and `RunSavedSearch` SHALL carry `sold_count`, `rating_avg` and
`review_count` taken from the search document, and SHALL carry 0 for each of them when no sales or review value has
been indexed.

#### Scenario: BR-19 A hit shows the indexed values

- **GIVEN** L has `sold_count` 5, `rating_avg` 4.5 and `review_count` 2
- **WHEN** a buyer calls `SearchListings` for L's title
- **THEN** the hit of L has `sold_count` 5, `rating_avg` 4.5 and `review_count` 2
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "A hit shows the indexed values"

#### Scenario: BR-20 A listing without sales or reviews shows zeros

- **GIVEN** a published listing that no event has touched
- **WHEN** a buyer calls `SearchListings` for its title
- **THEN** its hit has `sold_count` 0, `rating_avg` 0 and `review_count` 0, and the call does not fail
- **VERIFIED BY**: `team-search` tests and `platform-e2e` `search/search_best_selling_rating.feature` "A listing without sales or reviews shows zeros"
