## Context

`team-domain` owns `listing_db` (Rule 3) and emits `platform.events.v1.EventEnvelope` on
`listing.events`, keyed by `listing_id` for per-entity ordering (`internal/events/publisher.go`).
Today the emit is a post-commit best-effort call (`internal/handler/listing.go:360`), the exact
dual-write hazard `platform-core/docs/DATA_ARCHITECTURE.md` §2 and ADR-0002 warn against. team-ai
already ships the target pattern in Python (`team-ai/app/modules/messaging/outbox/`): an
`outbox_events` table, a `PostgresOutboxStore` that claims rows with `FOR UPDATE SKIP LOCKED`, and
an `OutboxPublisher` loop with a retry policy. This change ports that to Go.

The write path today: `handler.CreateListing` → `svc.Create` → `repo.Create` (single `INSERT` via
`pgxpool.Pool`, no explicit tx) → **then** `handler.emit()` produces to Kafka. The outbox requires
the business write and the outbox insert to share one transaction, so the repository gains a
tx-aware write path.

## Decisions

### Outbox table schema

`outbox_events` (migration `0006`), combining DATA_ARCHITECTURE §2's sketch with team-ai's
lease/retry columns:

| column         | type          | notes |
|----------------|---------------|-------|
| `event_id`     | TEXT PRIMARY KEY | UUID; becomes `EventEnvelope.event_id` — the **dedupe key** consumers use |
| `aggregate_type` | TEXT NOT NULL | `'Listing'` |
| `aggregate_id` | TEXT NOT NULL | `listing_id` — also the Kafka partition key (ordering) |
| `event_type`   | TEXT NOT NULL | discriminator, e.g. `platform.listing.v1.ListingChanged` |
| `payload`      | BYTEA NOT NULL | the **fully-marshalled `EventEnvelope`** protobuf bytes (see below) |
| `principal`    | JSONB / BYTEA NULL | carried so the relayed envelope keeps the acting principal |
| `request_id`   | TEXT NOT NULL DEFAULT '' | trace continuity |
| `status`       | TEXT NOT NULL DEFAULT 'pending' | `pending` \| `published` \| `failed` |
| `attempts`     | INT NOT NULL DEFAULT 0 | incremented on each relay failure |
| `available_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | backoff gate; claim only rows `available_at <= now()` |
| `locked_until` | TIMESTAMPTZ NULL | lease held by a claiming relayer |
| `published_at` | TIMESTAMPTZ NULL | set on success |
| `error`        | TEXT NULL | last failure reason |
| `created_at`   | TIMESTAMPTZ NOT NULL DEFAULT now() | ordering / observability |

Indexes: partial index on `(available_at)` `WHERE status = 'pending'` for the claim query; index on
`status`. `event_id` PK gives the relayer idempotency and gives consumers a stable dedupe id.

**Payload = the whole `EventEnvelope`, not just the inner event.** Today `produceEnvelope` builds the
envelope (`event_id`, `type`, `occurred_at`, `principal`, `request_id`, `payload`) at produce time.
To keep the wire bytes identical and decouple the relayer from event-type specifics, we marshal the
`EventEnvelope` at **enqueue** time and store those bytes; the relayer just does
`ProduceSync(topic, key=aggregate_id, value=payload)`. Crucially `EventEnvelope.event_id` is set to
the row's `event_id` (not a fresh UUID at produce time) so a re-delivered row carries the **same**
id every time — that is what makes consumer dedupe work under at-least-once.

### Transactional write

Add a tx-aware repository seam. `pgxpool.Pool` and `pgx.Tx` both satisfy a small `execer`/`querier`
interface (`Exec`/`Query`/`QueryRow`), so the existing repo methods can run against either. The
service gains a `CreateWithEvent(ctx, listing, envelope)` style path that:

```
tx := pool.Begin(ctx)
defer tx.Rollback(ctx)          // no-op after Commit
repo.CreateTx(ctx, tx, listing) // business write
outbox.EnqueueTx(ctx, tx, row)  // INSERT INTO outbox_events (...)
tx.Commit(ctx)                  // atomic: both or neither
```

If commit fails, nothing is written and the caller gets an error (unchanged handler error mapping).
The handler no longer calls Kafka on the request path at all — it constructs the `EventEnvelope`
(same fields as `produceEnvelope` builds today) and hands it to the service for enqueue. This is the
core fix: there is **no window** between commit and "event recorded", because they are the same commit.

### Relayer loop (at-least-once)

A background goroutine started in `bootstrap.OpenResources` (alongside the existing DB health check
in `lifecycle.go`), stopped by `CloseResources`:

```
ticker(OUTBOX_POLL_INTERVAL):
  rows = ClaimPending(batch=OUTBOX_BATCH_SIZE, lock=OUTBOX_CLAIM_LOCK_SECONDS)
         # SELECT ... WHERE status='pending' AND available_at<=now()
         #   ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT batch;  set locked_until
  for row in rows:
    err = kafkaPublisher.ProduceSync(topic, key=aggregate_id, value=payload)
    if err: MarkFailed(event_id, err, backoff)   # attempts++, available_at=now+backoff,
                                                  # status back to 'pending' (or 'failed' at max)
    else:   MarkPublished(event_id, now)          # status='published', published_at set
```

- **At-least-once**, not exactly-once: a crash *after* `ProduceSync` succeeds but *before*
  `MarkPublished` re-delivers the row on the next claim. That is acceptable and intended — consumers
  are idempotent by id (ADR-0002) and dedupe on the stable `EventEnvelope.event_id`.
- **Ordering.** `key = aggregate_id (listing_id)` preserves per-listing partition ordering exactly
  as today. Claiming `ORDER BY created_at` and a modest batch keeps same-listing events in insert
  order; strict global ordering is neither offered today nor required. (A single relayer instance is
  the simple default; `SKIP LOCKED` makes >1 relayer safe but can interleave across listings — call
  out that per-key order still holds because Kafka orders by partition, and same-key rows differ in
  `created_at`.)
- **Poll interval / batch.** Defaults: `OUTBOX_POLL_INTERVAL=1s`, `OUTBOX_BATCH_SIZE=100`,
  `OUTBOX_CLAIM_LOCK_SECONDS=60`, `OUTBOX_MAX_ATTEMPTS=10`. The 1s poll adds ≤1s tail latency vs the
  old inline produce — an acceptable trade for not losing events.
- **Failure/retry.** Exponential backoff via `available_at`; after `OUTBOX_MAX_ATTEMPTS` the row is
  parked `status='failed'` with `error` set for an operator (DLQ wiring is a non-goal follow-up). A
  crashed relayer's lease (`locked_until`) simply expires and the row is re-claimable.

### Why outbox-poller over CDC/Debezium

DATA_ARCHITECTURE §2 lists "Debezium CDC **or** Outbox Poller". We choose the poller: it needs no
Postgres logical-replication/WAL configuration, no Debezium/Kafka-Connect deployment, and no new
operational surface; it lives inside team-domain next to the code that owns the data, reuses the
existing `KafkaPublisher`, and is trivially testable. CDC's throughput edge is irrelevant at this
scale. The `outbox_events` row shape is CDC-compatible, so a later switch to Debezium's outbox-event
router remains open.

### Migration / back-compat with the existing publisher

- `KafkaPublisher`/`ListingPublisher` stay; only their **caller** moves from the request path to the
  relayer. `NoopPublisher` + `KAFKA_ENABLED=false` still means "no relay" — rows accumulate as
  `pending` and are never produced (matches today's "nothing emitted" behaviour), so existing local
  runs are unaffected.
- Migration `0006` is additive (new table only); `.down.sql` drops it. No change to `listings` /
  `listing_variants`.
- Wire compatibility: because the stored bytes are a normal `EventEnvelope`, `team-search` and other
  consumers see byte-identical messages — no consumer or proto change.

## Risks / Trade-offs

- **Added latency:** events now flow DB→poller→Kafka, adding up to `OUTBOX_POLL_INTERVAL`. Acceptable.
- **Duplicates on the happy-crash path:** by design; requires consumer dedupe on `event_id` — already
  satisfied by idempotent consumers, called out so it stays true.
- **Relayer is a new moving part:** mitigated by gating on config, graceful start/stop in the existing
  lifecycle, and lease expiry so no row is stuck if a relayer dies mid-batch.
- **Table growth:** `published` rows accumulate; a periodic prune (delete `published` older than N
  days) is a small follow-up, not required for correctness.
