## ADDED Requirements

### Requirement: An erased seller's listings are deleted

WHEN team-domain consumes `platform.identity.v1.UserErased` from `identity.events`, THE SYSTEM SHALL, in one transaction, delete every listing whose `seller_id` equals the event `user_id` in any status together with its variants, and write one `ListingChanged` outbox row with `change_type` DELETED per deleted listing, attributed to principal `service-team-domain`. THE SYSTEM SHALL NOT keep the listings as archived, held or banned.

#### Scenario: Erased seller's listings in every status are deleted
- **WHEN** a seller owning 2 `published`, 1 `pending_review`, 1 `draft`, 1 `held` and 1 `banned` listing (one with 2 variants) is the subject of `UserErased`
- **THEN** 0 listings and 0 variants remain for that seller and exactly 6 `ListingChanged(DELETED)` outbox rows exist, one per deleted listing id
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Deleted listings leave the store
- **WHEN** the erasure of a seller with a published listing has been applied
- **THEN** within 60 s a search for the listing title returns 0 results and `GetListing` for its id returns `NotFound`
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Delete events match the existing delete path
- **WHEN** a listing is deleted by the erasure
- **THEN** its `ListingChanged` envelope has `change_type` DELETED, carries the listing id and its pre-delete snapshot, and principal `service-team-domain`
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Other sellers' listings are not touched
- **WHEN** seller A is erased while seller B owns published listings
- **THEN** seller B's listings keep status `published` and no outbox row is written for them
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Erasure has no RPC entry point and owner checks stay
- **WHEN** seller B calls `DeleteListing` on a listing owned by seller A, or an anonymous caller calls any team-domain RPC naming seller A
- **THEN** the call is denied (`PermissionDenied` or `Unauthenticated`) and seller A's listing still exists
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Roles in the event do not widen the delete
- **WHEN** `UserErased` carries `roles` empty or without `seller` for a user who owns 1 listing
- **THEN** that listing is deleted, and when the user owns none the handler succeeds and writes 0 rows
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Batch boundary of 500 listings
- **WHEN** a seller owning 501 listings is erased
- **THEN** all 501 listings are deleted and 501 outbox rows exist, from one committed transaction
- **VERIFIED BY**: TBD by e2e-author

### Requirement: Erasure also removes the seller's shop data

WHEN the erasure of a seller is applied, THE SYSTEM SHALL in the same transaction delete the user's `storefronts` row, every `bundles` row owned by the user and the user's `seller_status_applied` row, and SHALL leave other sellers' storefronts and bundles unchanged.

#### Scenario: Storefront and bundles are deleted
- **WHEN** an erased seller has 1 storefront and 2 bundles
- **THEN** `GetStorefront` for the former slug returns `NotFound` and 0 bundles exist for that seller
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Failed shop-data delete rolls back the listings too
- **WHEN** deleting the bundles fails after the listings were deleted in the transaction
- **THEN** the transaction is rolled back, all listings and the storefront still exist, and the handler returns an error that is not permanent
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Another seller's storefront and bundles survive
- **WHEN** seller A is erased while seller B has a storefront and 3 bundles
- **THEN** seller B's storefront and 3 bundles are unchanged
- **VERIFIED BY**: TBD by e2e-author

### Requirement: Erasure is idempotent and atomic

THE SYSTEM SHALL record the event id in `processed_events` in the erasure transaction and SHALL treat a redelivered event, or a second `UserErased` for the same user, as a no-op that deletes 0 rows and writes 0 outbox rows. IF any step of the transaction fails, THEN THE SYSTEM SHALL roll back every change and return the error so the record is retried.

#### Scenario: Redelivered event is a no-op
- **WHEN** the same `UserErased` event id is consumed twice
- **THEN** the second delivery succeeds, deletes 0 rows and adds 0 outbox rows
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Second event for the same user is a no-op
- **WHEN** a `UserErased` with a different event id for an already erased user is consumed
- **THEN** the handler succeeds and writes 0 outbox rows
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Store failure rolls back and retries
- **WHEN** the outbox write fails after 3 of 6 listings were deleted in the transaction
- **THEN** all 6 listings still exist after the rollback, the handler returns an error that is not permanent, and the next delivery deletes all 6
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Erasure of a user with no listings
- **WHEN** `UserErased` is consumed for a user who owns 0 listings
- **THEN** the handler succeeds, 0 outbox rows are written and the event id is recorded as processed
- **VERIFIED BY**: TBD by e2e-author

### Requirement: The erasure consumer is isolated and fails safe

THE SYSTEM SHALL consume `UserErased` with consumer group `team-domain-erasure` and DLQ topic `identity.events.team-domain.dlq`, SHALL ignore envelopes of any other type, and SHALL route a malformed envelope, a payload that cannot be unmarshalled, an empty `user_id` or a missing `erased_at` to the DLQ as a permanent error without deleting anything. THE SYSTEM SHALL NOT log the raw user id or listing ids.

#### Scenario: Malformed event goes to the DLQ
- **WHEN** a record on `identity.events` has an undecodable envelope, an empty `user_id` or no `erased_at`
- **THEN** the handler returns a permanent error, the record appears on `identity.events.team-domain.dlq`, and 0 listings are deleted
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Other event types are ignored
- **WHEN** a `UserStatusChanged(BANNED)` or `UserRegistered` envelope is read by the erasure consumer
- **THEN** it returns success without deleting any listing, and the existing hold consumer still holds the listings
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A poison erasure event does not block seller hold
- **WHEN** a malformed `UserErased` sits in the erasure group while a `UserStatusChanged(SUSPENDED)` arrives
- **THEN** the suspended seller's listings are held within 60 s because the groups are independent
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Logs carry no raw identifiers
- **WHEN** an erasure of a seller is applied
- **THEN** the log lines of the consumer contain a hashed user key and counts, and contain neither the raw user id nor any listing id
- **VERIFIED BY**: TBD by e2e-author
