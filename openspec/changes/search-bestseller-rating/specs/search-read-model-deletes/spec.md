## ADDED Requirements

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
