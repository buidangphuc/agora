## Purpose

Defines which listings `SearchListings` and `Suggest` may return, in what order "newest" is served, and how the rating
filter and facet behave while no rating source exists, so buyers only ever see published listings and every control
does what its name says.

## ADDED Requirements

### Requirement: Search and suggest return only published listings by default

`SearchListings` and `Suggest` SHALL return only listings whose status is `published` unless the caller is allowed to
see its own drafts (next requirement). This SHALL hold when the request carries no `status` filter, and it SHALL apply
to hits, `page.total` and every facet count, because all of them are computed over the same filtered set. A listing
whose status is `draft`, `rejected`, `unspecified` or `deleted` SHALL never be returned or suggested by default. The
rule SHALL be enforced where the filter clauses are built, so that every search path (including
`RunSavedSearch` and any later retrieval strategy) inherits it. The wire contract SHALL NOT change.

#### Scenario: A draft is not returned when no status filter is sent

- **WHEN** a listing exists with status `draft` and an anonymous caller searches for its title with no filters
- **THEN** the response contains no hit for that listing and `page.total` does not count it

#### Scenario: A draft is not returned when the caller asks for published explicitly

- **WHEN** a listing exists with status `draft` and a caller searches for its title with `filters {"status":"published"}`
- **THEN** the response contains no hit for that listing

#### Scenario: A published listing is returned with or without the status filter

- **WHEN** a listing exists with status `published` and a caller searches for its title once with no filters and once
  with `filters {"status":"published"}`
- **THEN** both responses contain that listing and report the same `page.total`

#### Scenario: Facet counts exclude drafts

- **WHEN** a category holds 2 published and 3 draft listings and a caller searches with no filters
- **THEN** that category's facet count is 2

#### Scenario: Suggest never offers a draft title

- **WHEN** a listing exists with status `draft` and title "Zdraftonly gadget" and a caller calls `Suggest` with query
  "Zdraft"
- **THEN** the suggestions do not include "Zdraftonly gadget"

#### Scenario: A listing that becomes published is found

- **WHEN** a draft listing is published (status changes to `published`) and the index has applied the event
- **THEN** a search for its title with no filters returns it

### Requirement: Only a listing's owner can see its drafts, explicitly

A non-`published` status SHALL be accepted in the `status` filter only for an authenticated **user** principal whose
`filters["seller_id"]` equals that principal's own id, and only the value `draft`. The owner view SHALL be explicit:
omitting `status` still returns published listings only. Any other use SHALL be rejected without returning data:
`status=draft` from an anonymous caller, from a non-owner, from a `service` principal, or without a matching
`seller_id` filter SHALL fail with `PERMISSION_DENIED` (`UNAUTHENTICATED` when there is no principal or the principal is
anonymous); a `status` value that is not `published` or `draft` (including `deleted`, `rejected`, `any`) SHALL fail with
`INVALID_ARGUMENT`. The owner view SHALL return only listings whose `seller_id` is the caller's.

#### Scenario: A seller sees their own draft

- **WHEN** seller A (authenticated user) has a draft listing and searches with
  `filters {"seller_id":"<A>","status":"draft"}`
- **THEN** the response contains that draft and only listings owned by A

#### Scenario: The owner view is explicit

- **WHEN** seller A has one draft and one published listing and searches with `filters {"seller_id":"<A>"}` only
- **THEN** the response contains the published listing and not the draft

#### Scenario: Another user cannot view someone else's drafts

- **WHEN** user B searches with `filters {"seller_id":"<A>","status":"draft"}`
- **THEN** the call fails with `PERMISSION_DENIED` and returns no hits

#### Scenario: An anonymous caller cannot ask for drafts

- **WHEN** an anonymous caller searches with `filters {"status":"draft"}`
- **THEN** the call fails with `UNAUTHENTICATED` and returns no hits

#### Scenario: A draft filter without the caller's own seller_id is rejected

- **WHEN** an authenticated seller searches with `filters {"status":"draft"}` and no `seller_id`
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: An unknown or internal status value is rejected

- **WHEN** any caller searches with `filters {"status":"deleted"}` (or `rejected`, or `any`)
- **THEN** the call fails with `INVALID_ARGUMENT`

### Requirement: Newest sorts by creation time

`SORT_BY_NEWEST` SHALL order results by the time the listing was created, newest first, and SHALL NOT use the listing id
as a proxy. The creation time SHALL be the `occurred_at` of the listing's `CREATED` `ListingChanged` event, SHALL be
preserved across later updates, and SHALL converge to the same value under redelivery, reordering and replay. Listings
with equal creation time SHALL be ordered deterministically. A listing for which no creation time is known SHALL sort
after every listing that has one. Editing a listing SHALL NOT change its position under `SORT_BY_NEWEST`.

#### Scenario: Newest orders by creation time, not by id

- **WHEN** listing X (id "zzz", created at T1) and listing Y (id "aaa", created at T2 > T1) are indexed and a caller
  searches with `SORT_BY_NEWEST`
- **THEN** Y is returned before X

#### Scenario: Editing a listing does not make it newer

- **WHEN** older listing X is updated after newer listing Y was created and a caller searches with `SORT_BY_NEWEST`
- **THEN** Y is still returned before X

#### Scenario: Replaying creation events does not change the order

- **WHEN** the `CREATED` event of a listing is delivered twice, or after a later `UPDATED` event of the same listing
- **THEN** its creation time is the `CREATED` event's `occurred_at` and the `SORT_BY_NEWEST` order is unchanged

#### Scenario: A listing with unknown creation time sorts last

- **WHEN** a listing indexed before creation times were recorded has no creation time and a caller searches with
  `SORT_BY_NEWEST`
- **THEN** it is returned after every listing that has a creation time

### Requirement: The rating filter and facet do not pretend to have data

While no event or store supplies listing ratings, a `SearchListings` request with `min_rating > 0` SHALL fail with
`INVALID_ARGUMENT` and an error that says ratings are not available, instead of returning an empty result. The
`facets.ratings` list SHALL be empty (not four zero-count buckets). `min_rating` of 0 (unset) SHALL behave exactly as
today. A full listing update SHALL NOT write a rating value, so it cannot overwrite a rating supplied later. Facets
other than `ratings` SHALL be unchanged.

#### Scenario: A rating filter is rejected, not silently empty

- **WHEN** a caller searches with `min_rating = 4`
- **THEN** the call fails with `INVALID_ARGUMENT` and the message identifies `min_rating` as unavailable

#### Scenario: No rating filter behaves as before

- **WHEN** a caller searches with `min_rating` unset or 0
- **THEN** the call succeeds and its hits are unaffected by any rating

#### Scenario: The ratings facet is empty

- **WHEN** a caller searches with any query
- **THEN** `facets.ratings` is empty and the categories, price ranges and sellers facets are populated as before

#### Scenario: A listing update does not write a rating

- **WHEN** a `ListingChanged` upsert is applied to a listing
- **THEN** the indexed document has no `rating` value written by that upsert
