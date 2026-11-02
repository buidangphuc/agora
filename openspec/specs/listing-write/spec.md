# listing-write Specification

## Purpose
Defines how listing writes publish their events reliably: each write records its event in the same transaction (outbox), and a relayer publishes pending events at least once, with failure handling.

## Requirements

### Requirement: Listing writes record their event in the same transaction

`team-domain` SHALL persist the outgoing domain event to an `outbox_events` row **inside the same
database transaction** as the listing write (create, update, delete, and stock changes), so the
business row and the event row commit or roll back atomically. The request path SHALL NOT publish to
Kafka directly; there SHALL be no window in which the listing is committed but its event is not
recorded.

#### Scenario: Event row commits atomically with the listing

- **WHEN** a seller creates or updates a listing and the transaction commits
- **THEN** exactly one `outbox_events` row for that change exists with `status = 'pending'`, written
  under the same transaction as the `listings` row

#### Scenario: A rolled-back write leaves no event

- **WHEN** the listing write transaction fails and rolls back
- **THEN** no `outbox_events` row is left behind for that change and no event is ever produced

### Requirement: A relayer publishes pending events at least once

`team-domain` SHALL run a background relayer that periodically claims pending `outbox_events` rows,
produces each as a `platform.events.v1.EventEnvelope` to the `listing.events` topic keyed by
`listing_id`, and marks the row published on success. Delivery SHALL be **at-least-once**: a row is
marked published only after a successful produce, and a failure leaves the row to be retried. The
produced `EventEnvelope` SHALL be byte-compatible with the current format (same fields, same
`event_id` as stored on the row) so existing consumers are unaffected.

#### Scenario: Pending events are delivered exactly as before on the wire

- **WHEN** the relayer runs with `outbox_events` rows in `status = 'pending'`
- **THEN** each row is produced to `listing.events` with `key = listing_id` and an `EventEnvelope`
  identical in shape to the pre-change format, and the row moves to `status = 'published'`

#### Scenario: Event survives a Kafka publish failure and is delivered once the relayer retries

- **WHEN** a listing write commits while the Kafka produce is failing (broker unavailable)
- **THEN** the event is retained as a `pending` `outbox_events` row (not lost), and once Kafka is
  reachable the relayer produces it and marks it `published` — the event is delivered at least once

#### Scenario: A relayed event carries a stable id for consumer dedupe

- **WHEN** the relayer re-delivers a row after a crash between produce and mark-published
- **THEN** the re-delivered `EventEnvelope` carries the **same** `event_id` as the first delivery, so
  an idempotent consumer deduplicates it

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
