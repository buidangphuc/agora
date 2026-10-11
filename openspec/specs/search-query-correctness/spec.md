# search-query-correctness Specification

## Purpose
Defines what "newest" means in search ordering and how search treats rating while no rating is indexed, so that the
sort and filter controls a buyer sees do what they say.

## Requirements

### Requirement: Newest-first orders by creation time in every search mode

`team-search` SHALL record each listing's creation time from the `occurred_at` of its `ListingChanged` CREATED event,
SHALL keep it unchanged across later updates, and SHALL record it even when the CREATED event arrives after a newer
update. `SORT_BY_NEWEST` SHALL order hits by creation time, most recent first, with ties broken by listing id, and
listings without a recorded creation time last. An explicit `SORT_BY_NEWEST`, `SORT_BY_PRICE_ASC` or
`SORT_BY_PRICE_DESC` SHALL determine the order of the returned hits in every search mode, including
`SEARCH_MODE_HYBRID` and the default mode with a non-empty query.

#### Scenario: Newest-first lists the most recently created listing first

- **GIVEN** a seller creates and publishes listing A, then listing B, then updates listing A's description, all with a shared unique keyword in the title
- **WHEN** a buyer searches for that keyword with `SORT_BY_NEWEST` through the gateway
- **THEN** within 30 seconds the hits are B then A

#### Scenario: Newest-first holds in hybrid search

- **GIVEN** the listings A and B of the previous scenario
- **WHEN** a buyer searches for that keyword with `SORT_BY_NEWEST` and `SEARCH_MODE_HYBRID`, then with `SORT_BY_NEWEST` and no search mode
- **THEN** both responses return B then A

#### Scenario: A late create event still records the creation time

- **GIVEN** a seller has created and published a real listing whose title carries a unique keyword, and a listing id that the read-model does not hold
- **WHEN** a `ListingChanged` UPDATED for that id (status published, a title with the same keyword) with `occurred_at` T2 is published to `listing.events`, followed by its `ListingChanged` CREATED with `occurred_at` T1, where the real listing's creation is before T1 and T1 is before T2
- **THEN** within 30 seconds a `SORT_BY_NEWEST` search for that keyword returns the out-of-order listing first, with the UPDATED event's title, and the real listing second

### Requirement: Rating is not offered while no rating is indexed

Because no listing event carries a rating, `SearchListings` SHALL reject a `min_rating` other than 0 with
`INVALID_ARGUMENT` before running any retrieval, in every search mode. `SearchListings` and `RunSavedSearch` SHALL
return an empty `facets.ratings` list. A request with `min_rating` 0 SHALL behave as if no rating was given.

#### Scenario: A minimum-rating search is rejected

- **WHEN** a buyer calls `SearchListings` through the gateway with `min_rating` 4, then with `min_rating` 4 and `SEARCH_MODE_SEMANTIC`
- **THEN** both calls fail with `invalid_argument`

#### Scenario: The ratings facet is empty

- **GIVEN** published listings that match a query
- **WHEN** a buyer calls `SearchListings` for that query with `min_rating` 0, and runs a saved search with the same query
- **THEN** both responses return the matching listings and an empty `facets.ratings` list, while `facets.categories` is not empty
