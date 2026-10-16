## ADDED Requirements

### Requirement: Suspended or banned seller's listings are held

WHEN team-domain consumes `UserStatusChanged` with `status` SUSPENDED or BANNED from `identity.events`, THE SYSTEM SHALL, in one transaction, change every listing of `user_id` with stored status `published` or `pending_review` to `held`, store the prior status in `held_prior_status`, set the moderation reason code to `seller_enforcement`, and write one `ListingChanged(UPDATED)` outbox row per changed listing carrying status HELD. Listings in `draft`, `rejected`, `banned` or `held` SHALL be left unchanged.

#### Scenario: Seller hold: Suspension holds published and pending listings
- **WHEN** a seller owning 2 `published`, 1 `pending_review`, 1 `draft` and 1 `rejected` listing is the subject of `UserStatusChanged(SUSPENDED)`
- **THEN** exactly the 3 published/pending listings have status `held` with `held_prior_status` `published`, `published`, `pending_review`, the draft and rejected listings are unchanged, and 3 `ListingChanged(UPDATED)` outbox rows with status HELD exist
- **VERIFIED BY**: team-domain `internal/repository` `TestHoldSellerListingsHoldsPublishedAndPending`

#### Scenario: Seller hold: Ban holds the same way
- **WHEN** an active seller's `UserStatusChanged(BANNED)` is consumed
- **THEN** every published listing of that seller has status `held` with `held_prior_status` `published`
- **VERIFIED BY**: team-domain `internal/consumer` `TestUserStatusChangedBannedHolds`

#### Scenario: Seller hold: Suspension to ban keeps the original prior status
- **WHEN** `UserStatusChanged(SUSPENDED)` then `UserStatusChanged(BANNED)` (different event ids) are consumed for a seller with one published listing
- **THEN** the listing stays `held` with `held_prior_status` `published` and only 1 `ListingChanged` outbox row exists
- **VERIFIED BY**: team-domain `internal/repository` `TestHoldSellerListingsKeepsOriginalPriorStatus`

#### Scenario: Seller hold: Admin-banned listing is not touched
- **WHEN** a seller owning one listing with status `banned` is suspended
- **THEN** that listing keeps status `banned` and no outbox row is written for it
- **VERIFIED BY**: team-domain `internal/repository` `TestHoldSellerListingsSkipsBannedListing`

#### Scenario: Seller hold: Hold event carries the status for de-indexing
- **WHEN** a listing is held
- **THEN** its `ListingChanged` envelope has `change_type` UPDATED, listing status `LISTING_STATUS_HELD`, moderation reason code `seller_enforcement`, no moderation note, no screen flags and principal `service-team-domain`
- **VERIFIED BY**: team-domain `internal/consumer` `TestHoldEventCarriesHeldStatus`

#### Scenario: Seller hold: Seller with no listings is a no-op
- **WHEN** `UserStatusChanged(SUSPENDED)` is consumed for a user who owns 0 listings
- **THEN** the handler returns success, 0 outbox rows are written and the event id is recorded as processed
- **VERIFIED BY**: team-domain `internal/consumer` `TestUserStatusChangedUnknownSellerNoop`

#### Scenario: Seller hold: Held listings leave the store
- **WHEN** a seller is suspended and the hold is applied
- **THEN** within 60 s a search for the listing title returns 0 results and GetListing by a non-owner returns `NotFound`
- **VERIFIED BY**: platform-e2e `seller_enforcement_hold.feature` "Suspended seller's listings leave the store"

#### Scenario: Seller hold: Other sellers' listings are not touched
- **WHEN** seller A is suspended while seller B owns published listings
- **THEN** seller B's listings keep status `published` and no outbox row is written for them
- **VERIFIED BY**: team-domain `internal/repository` `TestHoldSellerListingsOnlyTargetsUser`

### Requirement: Reinstated seller's listings return to their prior status

WHEN team-domain consumes `UserStatusChanged` with `status` ACTIVE or RESTRICTED, THE SYSTEM SHALL, in one transaction, change every `held` listing of `user_id` back to `held_prior_status`, clear `held_prior_status` and the `seller_enforcement` reason code, and write one `ListingChanged(UPDATED)` outbox row per restored listing. Restore SHALL NOT re-screen or re-queue a listing; a listing restored to `pending_review` appears in ListModerationQueue again.

#### Scenario: Seller hold: Reinstatement restores prior statuses
- **WHEN** a seller with held listings whose `held_prior_status` are `published` and `pending_review` is the subject of `UserStatusChanged(ACTIVE)`
- **THEN** the first listing has status `published`, the second `pending_review`, both have empty `held_prior_status` and no `seller_enforcement` reason code, and 2 `ListingChanged(UPDATED)` rows exist
- **VERIFIED BY**: team-domain `internal/repository` `TestRestoreSellerListingsReturnsPriorStatus`

#### Scenario: Seller hold: Restricted also restores
- **WHEN** a suspended seller is moved to RESTRICTED (`UserStatusChanged(RESTRICTED)`)
- **THEN** their held published listing returns to `published`
- **VERIFIED BY**: team-domain `internal/consumer` `TestUserStatusChangedRestrictedRestores`

#### Scenario: Seller hold: Restored listing is visible again
- **WHEN** an admin sets a suspended seller back to `active`
- **THEN** within 60 s the seller's previously published listing is returned by search and by GetListing for a non-owner
- **VERIFIED BY**: platform-e2e `seller_enforcement_hold.feature` "Reinstated seller's listings return"

#### Scenario: Seller hold: Admin-banned listing is not resurrected
- **WHEN** a seller owning a `banned` listing and a held listing is reinstated
- **THEN** the held listing returns to its prior status and the `banned` listing stays `banned`
- **VERIFIED BY**: team-domain `internal/repository` `TestRestoreSellerListingsLeavesBannedListing`

#### Scenario: Seller hold: Restore with nothing held is a no-op
- **WHEN** `UserStatusChanged(ACTIVE)` is consumed for a seller with only published listings
- **THEN** no listing changes and 0 outbox rows are written
- **VERIFIED BY**: team-domain `internal/repository` `TestRestoreSellerListingsNothingHeldNoop`

#### Scenario: Seller hold: Restored pending listing is back in the moderation queue
- **WHEN** a seller whose held listing has `held_prior_status` `pending_review` is reinstated and an admin calls ListModerationQueue
- **THEN** the listing has status `pending_review`, appears in the queue, and no screen call was made for it during restore
- **VERIFIED BY**: team-domain `internal/service` `TestRestoreSellerListingsPendingReturnsToQueueWithoutScreen`

#### Scenario: Seller hold: Suspension lifted at login restores listings
- **WHEN** a suspension passes its end time and the seller logs in, so team-identity emits `UserStatusChanged(ACTIVE)`
- **THEN** within 60 s of that login the seller's held listing is `published` again
- **VERIFIED BY**: platform-e2e `seller_enforcement_hold.feature` "Expired suspension lifts the hold at login"

#### Scenario: Seller hold: Restore does not touch other sellers' listings
- **WHEN** seller A is reinstated while seller B's listing is `held`
- **THEN** seller B's listing stays `held` and no outbox row is written for it
- **VERIFIED BY**: team-domain `internal/repository` `TestRestoreSellerListingsOnlyTargetsUser`

### Requirement: Restriction alone holds nothing

WHEN team-domain consumes `UserStatusChanged` with `status` RESTRICTED and `previous_status` ACTIVE, THE SYSTEM SHALL leave every listing of the user unchanged and write no outbox row.

#### Scenario: Seller hold: Restricted seller keeps listings published
- **WHEN** an active seller with 2 published listings is restricted
- **THEN** both listings stay `published` and 0 outbox rows are written
- **VERIFIED BY**: team-domain `internal/consumer` `TestUserStatusChangedRestrictedFromActiveHoldsNothing`

#### Scenario: Seller hold: Restriction of a held seller is not a new hold
- **WHEN** `UserStatusChanged(RESTRICTED)` with `previous_status` SUSPENDED is consumed for a seller with a held listing
- **THEN** the held listing is restored to its prior status and is not held again
- **VERIFIED BY**: team-domain `internal/consumer` `TestUserStatusChangedRestrictedFromSuspendedRestores`

### Requirement: Event handling is idempotent and fail-safe

THE SYSTEM SHALL apply each `identity.events` envelope id at most once, record it in `processed_events` in the same transaction as the listing changes, skip a `UserStatusChanged` whose `changed_at` is not newer than the last one applied for that `user_id` (stored in `seller_status_applied` in the same transaction) while still recording its event id, retry a failing handler with backoff and park the record on the dead-letter topic after the maximum attempts, and acknowledge envelopes of other types without action.

#### Scenario: Seller hold: Redelivered event changes nothing twice
- **WHEN** the same `UserStatusChanged(SUSPENDED)` envelope (same event id) is delivered 2 times
- **THEN** the second delivery returns success, the listing rows are unchanged since the first delivery and the outbox holds 1 row per listing
- **VERIFIED BY**: team-domain `internal/consumer` `TestUserStatusChangedRedeliveryIsIdempotent`

#### Scenario: Seller hold: Re-driven older suspension does not re-hold a reinstated seller
- **WHEN** `UserStatusChanged(ACTIVE)` with `changed_at` T2 has been applied for a seller with a published listing, and then a `UserStatusChanged(SUSPENDED)` with a different event id and `changed_at` T1 earlier than T2 is consumed (a DLQ re-drive)
- **THEN** the handler returns success, the listing stays `published`, 0 outbox rows are written, the event id is recorded as processed and the stored `changed_at` for the seller stays T2
- **VERIFIED BY**: team-domain `internal/repository` `TestSellerStatusStaleEventSkipped`

#### Scenario: Seller hold: Event with the same changed_at is stale
- **WHEN** a `UserStatusChanged(SUSPENDED)` arrives with a new event id and a `changed_at` equal to the last applied value for that seller
- **THEN** no listing changes and 0 outbox rows are written
- **VERIFIED BY**: team-domain `internal/repository` `TestSellerStatusEqualChangedAtSkipped`

#### Scenario: Seller hold: Other event types are ignored
- **WHEN** a `SessionRevoked` envelope is consumed
- **THEN** the handler returns success and no listing or `processed_events` row changes
- **VERIFIED BY**: team-domain `internal/consumer` `TestIdentityConsumerIgnoresOtherTypes`

#### Scenario: Seller hold: Malformed event is parked, not applied
- **WHEN** an envelope with an unparsable payload, or a `UserStatusChanged` with an empty `user_id`, no `changed_at`, UNSPECIFIED status or a status number the consumer does not know (for example 9), is consumed
- **THEN** no listing changes and the record is parked on the DLQ after the permanent-error rule skips retries
- **VERIFIED BY**: team-domain `internal/consumer` `TestUserStatusChangedMalformedGoesToDLQ`

#### Scenario: Seller hold: Database failure retries then parks
- **WHEN** the database returns an error on every attempt for a `UserStatusChanged(SUSPENDED)` event
- **THEN** the handler is attempted 5 times, the transaction leaves all listings unchanged and no `processed_events` row, and the record is produced to the DLQ topic
- **VERIFIED BY**: team-domain `internal/consumer` `TestUserStatusChangedDBFailureRetriesThenDLQ`

#### Scenario: Seller hold: Consumer disabled does nothing
- **WHEN** `IDENTITY_CONSUMER_ENABLED` is false
- **THEN** the service starts without a Kafka consumer and every other RPC behaves as before
- **VERIFIED BY**: team-domain `internal/bootstrap` `TestIdentityConsumerAddonDisabled`

### Requirement: Held listings are owner-only and write-locked

WHILE a listing has status `held`, THE SYSTEM SHALL return `NotFound` on GetListing to any caller other than the owner, omit it from public listing queries, show it to the owner in ListMyListings with status HELD, refuse owner UpdateListing and DeleteListing with `FAILED_PRECONDITION`, and refuse `ReserveStock` with a new `reservation_id` with `FAILED_PRECONDITION` without changing stock.

#### Scenario: Seller hold: Non-owner cannot read a held listing
- **WHEN** a buyer calls GetListing on a `held` listing
- **THEN** the RPC returns `NotFound`
- **VERIFIED BY**: team-domain `internal/handler` `TestGetListingHeldHiddenFromNonOwner`

#### Scenario: Seller hold: Owner sees the held listing and the reason
- **WHEN** the suspended seller calls ListMyListings
- **THEN** the held listing is returned with status `LISTING_STATUS_HELD` and moderation reason code `seller_enforcement`
- **VERIFIED BY**: team-domain `internal/handler` `TestListMyListingsShowsHeldWithReason`

#### Scenario: Seller hold: Owner cannot edit a held listing
- **WHEN** the owner calls UpdateListing on a `held` listing with `status=PUBLISHED`
- **THEN** the RPC returns `FAILED_PRECONDITION` and the stored status stays `held`
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingHeldRefusedFailedPrecondition`

#### Scenario: Seller hold: Another seller cannot edit a held listing
- **WHEN** a different seller without the `admin` scope calls UpdateListing on a `held` listing
- **THEN** the RPC returns `PermissionDenied` and the listing is unchanged
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingHeldNonOwnerDenied`

#### Scenario: Seller hold: Owner cannot delete a held listing
- **WHEN** the owner calls DeleteListing on a `held` listing
- **THEN** the RPC returns `FAILED_PRECONDITION` and the listing still exists with status `held`
- **VERIFIED BY**: team-domain `internal/handler` `TestDeleteListingHeldRefusedFailedPrecondition`

#### Scenario: Seller hold: Stock cannot be reserved on a held listing
- **WHEN** the order service calls ReserveStock with a new `reservation_id` for a `held` listing with stock 5
- **THEN** the RPC returns `FAILED_PRECONDITION`, the stock stays 5 and no reservation row is created
- **VERIFIED BY**: team-domain `internal/grpcserver` `TestReserveStock_HeldListingFailedPrecondition`

#### Scenario: Seller hold: Reservation retry on a held listing stays a no-op
- **WHEN** a listing with an active reservation R is held and the order service retries ReserveStock with the same id R and the same listing, variant and quantity
- **THEN** the RPC returns success and the stock is unchanged by the retry
- **VERIFIED BY**: team-domain `internal/grpcserver` `TestReserveStock_HeldListingExistingReservationNoop`
