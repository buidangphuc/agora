## Purpose

Defines how the search read-model keeps a deleted listing deleted: a version-guarded tombstone that no older event,
partial update or stock event can undo, that no read path returns, and that is purged after a configurable retention.

## ADDED Requirements

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
indexer applied the delete, checking every `TOMBSTONE_PURGE_INTERVAL` (default 1 hour). An unparsable or non-positive
value SHALL fall back to the default and log a warning. Purging SHALL remove only tombstones, never a live document.

#### Scenario: An expired tombstone is purged

- **GIVEN** the e2e stack runs the search indexer with a short `TOMBSTONE_TTL` and `TOMBSTONE_PURGE_INTERVAL`
- **WHEN** a seller deletes one of two published listings through the gateway
- **THEN** the read-model holds a tombstone for the deleted listing right after the delete is consumed, holds no document for it once the TTL plus one purge interval has passed, and the other listing still appears in `SearchListings`
