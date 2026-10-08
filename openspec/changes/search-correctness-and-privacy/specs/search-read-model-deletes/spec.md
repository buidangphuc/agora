## Purpose

Defines how the search read-model treats a deleted listing so that it stays deleted: a delete is itself a versioned
write, so no late, redelivered or replayed older event can bring the listing back, and deleted-listing markers do not
accumulate forever.

## ADDED Requirements

### Requirement: A delete is a version-guarded write

Applying a listing delete (a `ListingChanged` with change type `DELETED`, a `ListingBaseInfoChanged` with change type
`DELETED`, or a `ListingStatusChanged` to `REJECTED`) SHALL leave the read-model in a state where the listing is not
returned by search or suggest, and SHALL record the delete under the event's version (the envelope `occurred_at`) in
the same ordering that governs every other write to that listing. A delete event older than the listing's current state
SHALL be ignored, so it cannot remove a listing that a later event updated. A delete for a listing that does not exist
yet SHALL still be recorded, so that a create delivered afterwards with an older version does not bring it in. The
deleted record SHALL hold no title, description or other listing content.

#### Scenario: A delete removes the listing from results

- **WHEN** a published listing is indexed, then its delete event is applied
- **THEN** search and suggest no longer return it

#### Scenario: A stale upsert after a delete does not resurrect it

- **WHEN** a listing is deleted at version T2 and a `ListingChanged` upsert for it with version T1 < T2 is applied
  afterwards, including after OpenSearch's own delete-retention window has elapsed
- **THEN** the listing is still not returned by search or suggest

#### Scenario: A redelivered delete is a no-op

- **WHEN** the same delete event is applied twice
- **THEN** the listing stays deleted and no error occurs

#### Scenario: A stale delete does not remove a newer listing

- **WHEN** a listing is updated at version T3 and a delete event for it with version T2 < T3 is applied afterwards
- **THEN** the listing is still returned by search

#### Scenario: A delete that arrives before the create wins over it

- **WHEN** a delete event at version T2 is applied for a listing with no document, then its create event at version
  T1 < T2 is applied
- **THEN** the listing is not returned by search

#### Scenario: A deleted record carries no listing content

- **WHEN** a listing is deleted
- **THEN** the stored record for its id has status `deleted` and no title or description

### Requirement: A deleted listing ignores partial and stock updates

A listing recorded as deleted SHALL NOT be modified by any partial update (base-info, pricing, status) or stock update,
whatever its version, and SHALL NOT change status. Only a full `ListingChanged` upsert with a version newer than the
delete can replace it. A partial or stock event for a deleted listing SHALL be acknowledged (no retry, no DLQ) and
SHALL leave search results unchanged.

#### Scenario: A later partial update does not revive a deleted listing

- **WHEN** a listing is deleted at T2 and a `ListingStatusChanged(PUBLISHED)` with version T3 > T2 is applied
- **THEN** the listing is still not returned by search and the event is acknowledged

#### Scenario: A stock event for a deleted listing is harmless

- **WHEN** a `ListingStockChanged` event is applied for a deleted listing
- **THEN** the listing is still not returned by search and the indexer keeps consuming

#### Scenario: A rejected listing stays out until a newer full update

- **WHEN** a listing is rejected (removed) at T2, a pricing partial update at T3 is applied, then a full
  `ListingChanged` at T4 with status `published` is applied
- **THEN** the listing is absent after T3 and returned after T4

### Requirement: Deleted-listing records are retained for a bounded time

Deleted-listing records SHALL be kept for a configurable retention (default 14 days) measured from the delete event's
version, and SHALL then be removed by the indexer without operator action. The retention SHALL be at least as long as
the longest expected delivery delay of a stale event for the same listing. Removal SHALL NOT affect any non-deleted
listing. A full replay of `listing.events` into an empty index SHALL reproduce the deleted state, because the delete
event is replayed after the older events of the same listing.

#### Scenario: Expired records are purged

- **WHEN** a deleted-listing record is older than the retention and the purge job runs
- **THEN** the record no longer exists

#### Scenario: Recent records are kept

- **WHEN** a deleted-listing record is younger than the retention and the purge job runs
- **THEN** the record still exists and still blocks a stale upsert

#### Scenario: The purge never touches live listings

- **WHEN** the purge job runs against an index holding published, draft and deleted records
- **THEN** only expired deleted records are removed

#### Scenario: Replay reproduces the deleted state

- **WHEN** the recorded events of a listing (create, update, delete) are replayed from the start into an empty index
- **THEN** the listing is not returned by search
