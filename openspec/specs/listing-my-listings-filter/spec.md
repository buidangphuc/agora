# listing-my-listings-filter Specification

## Purpose
Defines how `ListingService.ListMyListings` filters a seller's listings by status and title query and returns the total of the filtered set, so the seller app can count and page server-side.

## Requirements

### Requirement: ListMyListings filters by status

`ListingService.ListMyListings` SHALL return only the caller's own listings whose status is one of
`ListMyListingsRequest.statuses` when that list is non-empty, and every own listing when it is empty. The owner SHALL
come from the principal and the caller SHALL hold the `listing.write` scope. `LISTING_STATUS_UNSPECIFIED` in the list
SHALL be rejected with INVALID_ARGUMENT. `LISTING_STATUS_DELETED` SHALL match no row because team-domain never
stores it.

#### Scenario: A single status returns only that status

- **WHEN** a seller owning 3 published, 2 draft and 1 rejected listing calls `ListMyListings` with `statuses = [PUBLISHED]`
- **THEN** the response holds exactly the 3 published listings and `page.total` is 3

#### Scenario: Several statuses are OR-ed

- **WHEN** the same seller calls `ListMyListings` with `statuses = [DRAFT, REJECTED]`
- **THEN** the response holds the 2 draft and the 1 rejected listing, and `page.total` is 3

#### Scenario: Empty filter keeps today's behaviour

- **WHEN** a seller calls `ListMyListings` without `statuses` and without `query`
- **THEN** the response holds all 6 own listings ordered by id with `page.total` 6, identical to the response before this change

#### Scenario: An unspecified status is rejected

- **WHEN** a seller calls `ListMyListings` with `statuses = [LISTING_STATUS_UNSPECIFIED]`
- **THEN** the call fails with INVALID_ARGUMENT and no listing is returned

#### Scenario: The DELETED status matches nothing

- **WHEN** a seller calls `ListMyListings` with `statuses = [DELETED]`
- **THEN** the response holds 0 listings, `page.total` is 0 and `page.next_cursor` is empty

#### Scenario: Another seller's listings are never returned

- **WHEN** seller B, who owns no listing, calls `ListMyListings` with `statuses = [PUBLISHED]` while seller A owns 3 published listings
- **THEN** the response holds 0 listings and `page.total` is 0

#### Scenario: A caller without the listing.write scope is denied

- **WHEN** a buyer principal without the `listing.write` scope calls `ListMyListings` with any filter
- **THEN** the call fails with PERMISSION_DENIED and no listing is returned

### Requirement: ListMyListings searches by title

`ListMyListings` SHALL return only own listings whose title contains `query` as a case-insensitive substring when
`query` is non-empty, after trimming surrounding whitespace. `%`, `_` and `\` in `query` SHALL match literally. A
`query` longer than 100 runes SHALL be rejected with INVALID_ARGUMENT. `query` and `statuses` SHALL combine with AND.
The `query` value SHALL NOT appear in any log line.

#### Scenario: Substring match ignores case

- **WHEN** a seller owning listings titled "Áo thun Đỏ", "ÁO THUN xanh" and "Quần jean" calls `ListMyListings` with `query = "áo thun"`
- **THEN** the response holds the two "áo thun" listings and `page.total` is 2

#### Scenario: Query and status combine

- **WHEN** the seller owns a published and a draft listing both titled "Áo thun" and calls `ListMyListings` with `query = "áo"` and `statuses = [DRAFT]`
- **THEN** the response holds only the draft listing and `page.total` is 1

#### Scenario: LIKE wildcards match literally

- **WHEN** a seller owning "Giảm 50% áo" and "Giảm 500 áo" calls `ListMyListings` with `query = "50%"`
- **THEN** the response holds only "Giảm 50% áo" and `page.total` is 1

#### Scenario: A query of exactly 100 runes is accepted, 101 is rejected

- **WHEN** a seller calls `ListMyListings` with a `query` of 100 runes, then with one of 101 runes
- **THEN** the first call succeeds with 0 listings and the second fails with INVALID_ARGUMENT

#### Scenario: No match returns an empty page

- **WHEN** a seller calls `ListMyListings` with `query = "zzzz-no-such-title"`
- **THEN** the response holds 0 listings, `page.total` is 0 and `page.next_cursor` is empty

#### Scenario: A blank query is ignored

- **WHEN** a seller calls `ListMyListings` with `query = "   "`
- **THEN** the response equals the response for an empty `query`

#### Scenario: The query text is not logged

- **WHEN** a seller calls `ListMyListings` with `query = "secret-needle-title"`
- **THEN** no log line of `team-domain-svc` for that call contains "secret-needle-title"

### Requirement: Filtered pages are cursor-paged and counted

`ListMyListings` SHALL apply `statuses` and `query` to every page, keep the id-ordered keyset cursor, and return in
`page.total` the number of rows matching the filter (not the seller's whole catalogue). Page size SHALL default to the
existing default and SHALL be clamped to 100. Paged calls SHALL answer with p95 at most 150 ms locally for a seller with
2,000 listings at page size 20; index `listings (seller_id, status, id)` SHALL exist.

#### Scenario: Filtered pages cover the filtered set exactly once

- **WHEN** a seller owning 250 published and 50 draft listings pages `statuses = [PUBLISHED]` with `page_size = 100` following `next_cursor` until it is empty
- **THEN** the three pages hold 100, 100 and 50 listings, no id repeats, no draft appears, and every page reports `page.total` 250

#### Scenario: Page size above the maximum is clamped

- **WHEN** a seller owning 150 listings calls `ListMyListings` with `page_size = 1000`
- **THEN** the response holds 100 listings and a non-empty `next_cursor`

#### Scenario: Filtered page latency at 2,000 listings

- **WHEN** a seller owning 2,000 listings calls `ListMyListings` 50 times with `statuses = [PUBLISHED]`, `query = "áo"` and `page_size = 20`
- **THEN** the 95th percentile of the call durations is at most 150 ms

#### Scenario: A cursor from a different filter yields no foreign rows

- **WHEN** a seller reuses a `next_cursor` obtained for `statuses = [DRAFT]` on a call with `statuses = [PUBLISHED]`
- **THEN** the response holds only the seller's own published listings with id greater than the cursor id
