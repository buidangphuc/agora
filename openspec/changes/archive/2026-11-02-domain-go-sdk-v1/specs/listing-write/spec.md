## REMOVED Requirements

### Requirement: Relayer failure handling and back-compat gating

**Reason**: The relayer is the SDK `pkg/outbox` relayer. `KAFKA_ENABLED` and `OUTBOX_ENABLED` are deleted, so "Kafka disabled records but does not relay" cannot happen; the attempt-limited `failed` parking (`OUTBOX_MAX_ATTEMPTS`) is replaced by retry-forever for transient errors and parking for permanent ones.

**Migration**: Replaced by "The outbox relayer retries transient failures and parks permanent ones". Operators requeue parked rows with `outbox.Store.Requeue` or `RequeueParked`.

## ADDED Requirements

### Requirement: The outbox relayer retries transient failures and parks permanent ones

The relayer of `team-domain` SHALL always run (no enable variable) and SHALL publish `outbox_events` rows through the SDK relayer keyed by `aggregate_id` (the listing id) on `KAFKA_LISTING_TOPIC`. A transient publish error SHALL leave the row pending and SHALL retry it with capped exponential backoff and no attempt limit. A permanent record error (`outbox.Permanent`, a broker record-size error) SHALL park the row in `status = 'failed'` with its last error. Rows SHALL be published in `seq` order per listing id: a parked or backing-off row SHALL hold back the later rows of the same listing id only; other listing ids SHALL NOT be held back.

#### Scenario: Transient Kafka failure keeps the row and retries it
- **WHEN** a listing write commits while the publisher returns a transient error for 3 passes and then succeeds
- **THEN** the row stays `pending` with `attempts` 3 during the failures and is `published` on the fourth pass, and exactly one record with that `event_id` is produced
- **VERIFIED BY**: team-domain `internal/store` `TestRelayerRetriesTransientFailureWithoutLimit`

#### Scenario: A poison row is parked without blocking other listings
- **WHEN** one listing's row fails with a permanent error while rows of two other listing ids are pending
- **THEN** that row is `failed` with its error recorded and both other rows are `published` in the same pass
- **VERIFIED BY**: team-domain `internal/store` `TestPoisonRowParkedOthersPublished`

#### Scenario: A parked row holds back only its own listing
- **WHEN** a listing id has a `failed` row followed by a newer `pending` row of the same listing id
- **THEN** the newer row is not published until the failed row is requeued, and then both are published in `seq` order
- **VERIFIED BY**: team-domain `internal/store` `TestParkedRowHoldsBackSameListingOnly`

#### Scenario: Pending rows are delivered with the same wire bytes
- **WHEN** the relayer publishes a `ListingChanged` row
- **THEN** the record key is the listing id, the value equals the stored `payload` bytes and the `EventEnvelope.event_id` equals the row `event_id`
- **VERIFIED BY**: team-domain `internal/store` `TestRelayedRecordKeepsEnvelopeBytes`

#### Scenario: Relayer runs without any enable variable
- **WHEN** the service starts with `ENV=local` and no `OUTBOX_ENABLED` or `KAFKA_ENABLED` and a listing is created
- **THEN** the row is `published` and the record is on `listing.events` within 10 s
- **VERIFIED BY**: team-domain `internal/app` `TestRunRelaysOutbox`

### Requirement: Published outbox rows are purged after 7 days

The service SHALL purge `outbox_events` rows with `status = 'published'` older than 7 days by calling the SDK `outbox.Store.Purge` on a schedule inside the service process (a lifecycle component started by `app.Run`, with no enable variable). Rows that are `pending` or `failed` SHALL never be purged, whatever their age.

#### Scenario: Outbox purge: Published rows older than 7 days are deleted
- **WHEN** the purge runs over `published` rows aged 8 days and 6 days and a `pending` row and a `failed` row, both aged 8 days
- **THEN** only the 8-day `published` row is deleted and the other 3 rows remain
- **VERIFIED BY**: team-domain `internal/store` `TestPurgeDeletesOnlyOldPublishedRows`

#### Scenario: Outbox purge: The service runs the purge on its schedule
- **WHEN** the service runs with the purge interval set by the test seam to 200 ms and an 8-day-old `published` row exists
- **THEN** the row is deleted within 5 s without any call from the test
- **VERIFIED BY**: team-domain `internal/app` `TestRunPurgesPublishedOutboxRows`
