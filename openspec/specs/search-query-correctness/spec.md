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
