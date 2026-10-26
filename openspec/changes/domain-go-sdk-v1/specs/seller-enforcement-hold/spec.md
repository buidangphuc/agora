## REMOVED Requirements

### Requirement: Event handling is idempotent and fail-safe

**Reason**: The requirement contains the scenario "Consumer disabled does nothing", which describes `IDENTITY_CONSUMER_ENABLED`; that variable is deleted (importing Kafka means using it). `openspec` refuses to drop one scenario inside MODIFIED, so the requirement is renamed.

**Migration**: Replaced by "Event handling is idempotent, fail-safe and always on", which keeps every other scenario with its title and replaces the disabled scenario by "Seller hold: Consumer is always running".

## ADDED Requirements

### Requirement: Event handling is idempotent, fail-safe and always on

THE SYSTEM SHALL apply each `identity.events` envelope id at most once, record it in `processed_events` in the same transaction as the listing changes, skip a `UserStatusChanged` whose `changed_at` is not newer than the last one applied for that `user_id` (stored in `seller_status_applied` in the same transaction) while still recording its event id, retry a failing handler with backoff and park the record on the dead-letter topic `identity.events.dlq` after the maximum attempts (5), skip retries for a malformed record, run the consumer in every `ENV` with no enable variable, and acknowledge envelopes of other types without action.

#### Scenario: Seller hold: Redelivered event changes nothing twice
- **WHEN** the same `UserStatusChanged(SUSPENDED)` envelope (same event id) is delivered 2 times
- **THEN** the second delivery returns success, the listing rows are unchanged since the first delivery and the outbox holds 1 row per listing
- **VERIFIED BY**: team-domain `internal/events` `TestUserStatusChangedRedeliveryIsIdempotent`

#### Scenario: Seller hold: Re-driven older suspension does not re-hold a reinstated seller
- **WHEN** `UserStatusChanged(ACTIVE)` with `changed_at` T2 has been applied for a seller with a published listing, and then a `UserStatusChanged(SUSPENDED)` with a different event id and `changed_at` T1 earlier than T2 is consumed (a DLQ re-drive)
- **THEN** the handler returns success, the listing stays `published`, 0 outbox rows are written, the event id is recorded as processed and the stored `changed_at` for the seller stays T2
- **VERIFIED BY**: team-domain `internal/store` `TestSellerStatusStaleEventSkipped`

#### Scenario: Seller hold: Event with the same changed_at is stale
- **WHEN** a `UserStatusChanged(SUSPENDED)` arrives with a new event id and a `changed_at` equal to the last applied value for that seller
- **THEN** no listing changes and 0 outbox rows are written
- **VERIFIED BY**: team-domain `internal/store` `TestSellerStatusEqualChangedAtSkipped`

#### Scenario: Seller hold: Other event types are ignored
- **WHEN** a `SessionRevoked` envelope is consumed
- **THEN** the handler returns success and no listing or `processed_events` row changes
- **VERIFIED BY**: team-domain `internal/events` `TestIdentityConsumerIgnoresOtherTypes`

#### Scenario: Seller hold: Malformed event is parked, not applied
- **WHEN** an envelope with an unparsable payload, or a `UserStatusChanged` with an empty `user_id`, no `changed_at`, UNSPECIFIED status or a status number the consumer does not know (for example 9), is consumed
- **THEN** no listing changes and the record is parked on the DLQ after the permanent-error rule skips retries
- **VERIFIED BY**: team-domain `internal/events` `TestUserStatusChangedMalformedGoesToDLQ`

#### Scenario: Seller hold: Database failure retries then parks
- **WHEN** the database returns an error on every attempt for a `UserStatusChanged(SUSPENDED)` event
- **THEN** the handler is attempted 5 times, the transaction leaves all listings unchanged and no `processed_events` row, and the record is produced to the DLQ topic
- **VERIFIED BY**: team-domain `internal/events` `TestUserStatusChangedDBFailureRetriesThenDLQ`

#### Scenario: Seller hold: Consumer is always running
- **WHEN** the service starts with no variable that selects the consumer (`IDENTITY_CONSUMER_ENABLED` is not read) and a `UserStatusChanged(SUSPENDED)` envelope is produced to `identity.events`
- **THEN** the seller's published listings are `held` within 10 s
- **VERIFIED BY**: team-domain `internal/app` `TestRunConsumesIdentityEvents`
