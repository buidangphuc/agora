## Why

`team-domain` currently emits Kafka events with a **best-effort fire-and-forget dual write**:
`ListingHandler.emit()` (`internal/handler/listing.go:360`) calls
`publisher.PublishListingChanged(...)` *after* the DB write has already committed, and a publish
failure is only logged — the comment there admits "production uses a transactional outbox; see
ADR-0002". This is the classic dual-write hazard called out in
`platform-core/docs/DATA_ARCHITECTURE.md` §2: if the process crashes (or Kafka is down) between the
DB commit and the produce, the event is **lost** and `team-search`'s read-model silently diverges
from the write-model, with no way to notice.

ADR-0002 states the intended fix — a transactional outbox — and notes team-ai already ships an
`outbox/` store while "the relay process is future work". This change ports that pattern to Go for
`team-domain`: write the event to an `outbox_events` row **in the same DB transaction** as the
listing write, and run a relayer that publishes pending rows to Kafka `listing.events` and marks
them sent. The `EventEnvelope` on the wire is unchanged, so `team-search` and any other consumer
are unaffected.

## What Changes

- **team-domain (migrations)** — add `0006_outbox_events.up.sql` / `.down.sql`: an `outbox_events`
  table (event_id, aggregate id/type, event_type, key, payload BYTES, principal/request-id,
  status, attempts, available_at, locked_until, published_at, error, created_at) with indexes for
  the relayer's claim query. Follows the schema sketched in DATA_ARCHITECTURE §2, extended with the
  team-ai store's lease/retry columns.
- **team-domain (repository)** — add a transactional write path: the listing write (Create/Update/
  Delete + stock changes) and the outbox INSERT run under **one** `pgx` transaction on the shared
  pool, so they commit or roll back together. Introduce an `OutboxStore` (enqueue within a tx;
  `ClaimPending` via `SELECT … FOR UPDATE SKIP LOCKED`; `MarkPublished`; `MarkFailed`).
- **team-domain (handler/service)** — replace the post-commit `emit()` fire-and-forget with an
  in-transaction outbox enqueue. The handler stops calling the Kafka publisher directly on the
  request path; it builds the same `EventEnvelope` payload and enqueues it. The existing
  `ListingPublisher`/`KafkaPublisher` is reused by the relayer, not the request path.
- **team-domain (relayer)** — add a background relayer (started in `bootstrap` lifecycle,
  `internal/bootstrap/lifecycle.go`) that on a poll interval claims a batch of pending outbox rows,
  produces each to `listing.events` keyed by `listing_id` (per-entity ordering), marks published on
  success and failed/retry (with backoff) on error, and honours graceful shutdown. Gated by config
  so `KAFKA_ENABLED=false` keeps the current no-op behaviour (rows still written, never relayed).
- **team-domain (config)** — add outbox/relayer settings to `internal/config/config.go`
  (`OUTBOX_ENABLED`, `OUTBOX_POLL_INTERVAL`, `OUTBOX_BATCH_SIZE`, `OUTBOX_CLAIM_LOCK_SECONDS`,
  `OUTBOX_MAX_ATTEMPTS`) with defaults + `.env.example` drift-gate entries.

## Non-goals

- **Applying the outbox to other services** (team-chat, team-order, etc.) — this change is
  team-domain only; other services keep their current emit path until their own change.
- **CDC / Debezium** — DATA_ARCHITECTURE names Debezium-over-WAL as an alternative; we use an
  application-level poller instead (see `design.md` for why). No WAL/logical-replication setup.
- **Exactly-once delivery** — the outbox gives **at-least-once**; consumers are already idempotent
  (upsert/delete by id, ADR-0002), and `event_id` enables consumer-side dedupe. No 2PC/XA.
- **Changing the `EventEnvelope` or any proto** — the wire contract is byte-identical; no
  platform-core/proto change, no consumer change.
- **A DLQ for un-relayable rows** — poison rows park in `status=failed` after `OUTBOX_MAX_ATTEMPTS`
  for an operator to inspect; wiring `listing.events.DLQ` is a follow-up.
