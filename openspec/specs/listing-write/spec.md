# listing-write Specification

## Purpose
TBD - created by archiving change add-domain-transactional-outbox. Update Purpose after archive.

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

### Requirement: Relayer failure handling and back-compat gating

The relayer SHALL apply bounded retries with backoff and SHALL be gated by configuration. When Kafka
is disabled, listing writes SHALL still record outbox rows but no relay SHALL occur (matching the
prior no-emit behaviour). A row that exceeds the maximum attempts SHALL be parked in `status =
'failed'` with its last error, without blocking other rows.

#### Scenario: Kafka disabled records but does not relay

- **WHEN** `KAFKA_ENABLED=false` (or the outbox relayer is disabled) and a listing is written
- **THEN** an `outbox_events` row is committed but nothing is produced to Kafka

#### Scenario: A poison row is parked without blocking the queue

- **WHEN** a single row's produce keeps failing past the configured maximum attempts
- **THEN** that row is set to `status = 'failed'` with its error recorded, and the relayer continues
  publishing the other pending rows
