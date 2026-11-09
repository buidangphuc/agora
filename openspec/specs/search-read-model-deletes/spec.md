# search-read-model-deletes Specification

## Purpose
Defines how the search read-model keeps a deleted listing deleted: a version-guarded tombstone that no older event,
partial update or stock event can undo, that no read path returns, and that is purged after a configurable retention.

## Requirements

### Requirement: A deleted listing cannot be resurrected by an older event

When `team-search` applies a delete (`ListingChanged` with change type DELETED, `ListingBaseInfoChanged` with change
type DELETED, or `ListingStatusChanged` with status REJECTED), it SHALL keep a tombstone for that listing that records
the delete event's `occurred_at`. A `ListingChanged` whose `occurred_at` is at or before the tombstone's, or that has no
`occurred_at`, SHALL be ignored while the tombstone exists. A `ListingChanged` newer than the tombstone (which only the
domain can produce, for example a re-approved listing) SHALL replace it.

#### Scenario: A stale listing update after delete does not bring it back

- **GIVEN** a seller's published listing that appears in search
- **WHEN** the seller deletes it through the gateway and a `ListingChanged` (UPDATED, status published) for it with an `occurred_at` before the delete is then published to `listing.events`
- **THEN** after the search indexer has consumed past that event, neither `SearchListings` for its title nor `Suggest` for its title prefix returns it

#### Scenario: A redelivered create after delete does not bring it back

- **GIVEN** a listing that was created and then deleted through the gateway
- **WHEN** the original `ListingChanged` (CREATED) record for it is read from `listing.events` and published again unchanged
- **THEN** after the search indexer has consumed past the copy, `SearchListings` for its title does not return it

### Requirement: Partial and stock updates never revive a deleted listing

While a tombstone exists, `ListingBaseInfoChanged`, `ListingPricingChanged`, `ListingStatusChanged` and
`ListingStockChanged` for that listing SHALL leave it deleted, whatever their `occurred_at`.

#### Scenario: A newer status event does not revive a deleted listing

- **GIVEN** a listing deleted through the gateway
- **WHEN** a `ListingStatusChanged` with status PUBLISHED and an `occurred_at` after the delete is published to `listing.events`
- **THEN** after the search indexer has consumed past that event, `SearchListings` for its title does not return it

#### Scenario: A stock event for a deleted listing is ignored

- **GIVEN** a listing deleted through the gateway
- **WHEN** a valid `ListingStockChanged` with stock 5 and an `occurred_at` after the delete is published to `listing.events`
- **THEN** after the search indexer has consumed past that event, `SearchListings` for its title with `filters["in_stock"]="true"` does not return it and the record is not on `listing.events.dlq`

### Requirement: Deleted listings are invisible to every read path

A deleted listing SHALL NOT appear in, or be counted by, `SearchListings` hits, totals or facets in any search mode,
`Suggest`, `RunSavedSearch`, or the owner's draft view (`status=draft` with their own `seller_id`). The tombstone's
status SHALL NOT be requestable as a `status` filter.

#### Scenario: A deleted listing leaves search results, totals and facets

- **GIVEN** a seller with two published listings, so `SearchListings` with `filters["seller_id"]` set to that seller returns total 2 and a sellers facet count of 2
- **WHEN** the seller deletes one of them through the gateway
- **THEN** within 30 seconds the same search returns total 1, only the remaining listing, and a sellers facet count of 1, and a saved search with that filter run by a buyer returns only the remaining listing

#### Scenario: A deleted draft leaves the owner's draft view

- **GIVEN** a seller's draft listing that appears when the seller searches with `status=draft` and their own `seller_id`
- **WHEN** the seller deletes the draft through the gateway
- **THEN** within 30 seconds that owner search no longer returns it, and a search with `filters["status"]="deleted"` fails with `invalid_argument`

### Requirement: Tombstones are retained for a configurable time, then purged

The search indexer SHALL remove tombstones older than `TOMBSTONE_TTL` (default 14 days), measured from when the
indexer applied the delete, checking every `TOMBSTONE_PURGE_INTERVAL` (default 1 hour). Both are Go durations. An
unparsable or non-positive value SHALL make the indexer refuse to start with a message naming the key; an unset value
SHALL take the default. Purging SHALL remove only tombstones, never a live document.

#### Scenario: An expired tombstone is purged

- **GIVEN** the e2e stack runs the search indexer with a short `TOMBSTONE_TTL` and `TOMBSTONE_PURGE_INTERVAL`
- **WHEN** a seller deletes one of two published listings through the gateway
- **THEN** the read-model holds a tombstone for the deleted listing right after the delete is consumed, holds no document for it once the TTL plus one purge interval has passed, and the other listing still appears in `SearchListings`
- **VERIFIED BY**: `platform-e2e` `search/search_read_model_deletes.feature` "An expired tombstone is purged", and `team-search/internal/tombstone` `TestPurge_UsesTTLCutoff` and `team-search/internal/index` `TestIT_PurgeTombstones_RemovesOnlyExpiredTombstones`

#### Scenario: An invalid tombstone duration refuses boot

- **WHEN** the indexer settings are loaded with `TOMBSTONE_TTL=soon` or `TOMBSTONE_PURGE_INTERVAL=0s`
- **THEN** `LoadIndexer` returns an error that names the offending key
- **VERIFIED BY**: `team-search/internal/config` `TestTombstoneDurationsRefuseInvalid`

#### Scenario: Unset tombstone durations take the defaults

- **WHEN** the indexer settings are loaded with neither `TOMBSTONE_TTL` nor `TOMBSTONE_PURGE_INTERVAL` set
- **THEN** the TTL is 336h and the purge interval is 1h
- **VERIFIED BY**: `team-search/internal/config` `TestTombstoneDefaults`

### Requirement: Pending projection documents are invisible and purged after the tombstone TTL

A search document that holds only pending `sold_count` or rating values for a listing that has not been indexed SHALL NOT
appear in `SearchListings` or `RunSavedSearch` hits, totals or facets, and SHALL NOT be created by a stock event. The
indexer sweep SHALL remove pending documents older than `TOMBSTONE_TTL`, measured from its `pending_at` (set on each pending write, cleared by the listing upsert) and carrying no `status`, on every
`TOMBSTONE_PURGE_INTERVAL`, and SHALL NOT remove a document that has been upserted as a listing.

#### Scenario: A pending document never shows in search

- **GIVEN** a listing id L with no listing document
- **WHEN** `ListingSalesChanged{sold_count: 3}` for L is published and a buyer searches for a keyword in L's future title
- **THEN** no hit is returned for L, `total` and every facet bucket count exclude L, and a stock event for L still creates no listing document
- **VERIFIED BY**: `team-search/internal/index` integration test "pending doc is invisible" and `platform-e2e` `search/search_best_selling_rating.feature` "Values for a not-yet-indexed listing are merged on creation"

#### Scenario: An expired pending document is purged

- **GIVEN** the e2e stack runs the indexer with a short `TOMBSTONE_TTL` and `TOMBSTONE_PURGE_INTERVAL`, and a pending document exists for L
- **WHEN** the TTL plus one purge interval has passed without a listing event for L
- **THEN** the index holds no pending document for L, and a listing upserted before the TTL elapsed still has its `sold_count`
- **VERIFIED BY**: `team-search/internal/index` integration test "expired pending docs are purged" and `platform-e2e` `search/search_read_model_deletes.feature` "An expired pending document is purged"

#### Scenario: A listing document is never purged as pending

- **GIVEN** a published listing with projected `sold_count` older than `TOMBSTONE_TTL`
- **WHEN** the sweep runs
- **THEN** the listing is still returned by `SearchListings` with its `sold_count`
- **VERIFIED BY**: `team-search/internal/index` integration test "purge keeps live documents with projected values"

#### Scenario: Pending values on an expired tombstone go with it

- **GIVEN** the e2e stack runs the indexer with a short `TOMBSTONE_TTL` and `TOMBSTONE_PURGE_INTERVAL`, listing L is tombstoned, and a `ListingSalesChanged` for L with `changed_at` later than the delete is held as pending on the tombstone
- **WHEN** the TTL plus one purge interval has passed and then a `ListingChanged` CREATED for L is published
- **THEN** the hit of L carries `sold_count` 0, because the pending values were purged with the tombstone
- **VERIFIED BY**: `team-search/internal/index` integration test "pending values on an expired tombstone go with it"
