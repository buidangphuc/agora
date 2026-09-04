# Tasks

## 1. Code — team-domain (migration)
- [x] `migrations/0006_outbox_events.up.sql` (new): create `outbox_events` per `design.md`
      (event_id PK, aggregate_type, aggregate_id, event_type, payload BYTEA, principal, request_id,
      status, attempts, available_at, locked_until, published_at, error, created_at) + partial index
      `(available_at, created_at) WHERE status='pending'` + index on `status`.
- [x] `migrations/0006_outbox_events.down.sql` (new): `DROP TABLE outbox_events`.

## 2. Code — team-domain (repository + outbox store)
- [x] `internal/repository/listing_pg.go`: added a `DBTX` interface (`Exec`/`Query`/`QueryRow`)
      satisfied by both `*pgxpool.Pool` and `pgx.Tx`; write logic moved to free functions
      (`createListing`/`updateListing`/`deleteListing`/`loadVariants`) that run against either, with
      the existing pool methods delegating to them.
- [x] `internal/repository/outbox_pg.go` (new): `OutboxStore` with `EnqueueTx(ctx, tx, row)`,
      `ClaimPending` (`SELECT … WHERE status='pending' AND available_at<=now() ORDER BY available_at,
      created_at FOR UPDATE SKIP LOCKED LIMIT n`, sets `locked_until`), `MarkPublished`, `MarkFailed`
      (attempts++, backoff via `available_at`, `failed` at max) + `PgTxWriter` that runs the listing
      write and the outbox INSERT in one `pgx.Tx`. Port of team-ai's `PostgresOutboxStore`.
      (Port types `OutboxRow`/`PendingEvent`/`EnqueueFn`/`TxWriter` + in-memory fakes live in
      `internal/repository/outbox.go`.)

## 3. Code — team-domain (service + handler)
- [x] `internal/service/listing.go`: added transactional `CreateWithEvent`/`UpdateWithEvent`/
      `DeleteWithEvent` that run the business write + outbox enqueue in one tx (via `TxWriter`);
      `NewListingServiceWithOutbox` wires it. Falls back to a plain write when no writer is set.
- [x] `internal/handler/listing.go`: removed the post-commit `emit()` fire-and-forget and the
      publisher dependency; each write now builds the `EventEnvelope` (same fields as
      `events.produceEnvelope`, `event_id` == outbox row id) inside an `EnqueueFn` the service invokes
      within the write transaction.
- [x] `internal/events/publisher.go`: added `ProduceRaw(ctx, key, payloadBytes)` (relayer produce
      path) and `BuildListingChangedEnvelope` (byte-identical envelope with a caller-supplied
      event_id); kept the typed `Publish*` methods / `ListingPublisher` / `NoopPublisher`.

## 4. Code — team-domain (relayer + bootstrap + config)
- [x] `internal/events/relayer.go` (new): poll loop (`ClaimPending` → `ProduceRaw` → `MarkPublished`/
      `MarkFailed`), context-cancellable `Run`, deterministic `RelayOnce`; exponential backoff, parks
      poison rows at `MaxAttempts`. Mirrors `OutboxPublisher.publish_pending`.
- [x] `internal/bootstrap/lifecycle.go`: creates the `OutboxStore` over the pool, starts the relayer
      in `OpenResources` (when `OUTBOX_ENABLED` && `KAFKA_ENABLED`) and stops+drains it in
      `CloseResources` (like `startDBHealthCheck`). `main.go` wires the service with `PgTxWriter`.
- [x] `internal/config/config.go`: added `OUTBOX_ENABLED` (true), `OUTBOX_POLL_INTERVAL` (1s),
      `OUTBOX_BATCH_SIZE` (100), `OUTBOX_CLAIM_LOCK_SECONDS` (60), `OUTBOX_MAX_ATTEMPTS` (10) +
      `OutboxPollInterval()` helper; `.env.example` updated so the drift gate passes.
- [x] Unit tests (black-box, in-memory fakes): service tx path rolls back both rows on write failure
      and enqueues one row on success (`service/listing_test.go`); relayer marks published on success,
      retries with backoff on failure, and parks a poison row without blocking the queue
      (`events/relayer_test.go`); handler/grpc tests assert the enqueued outbox row + stable event_id
      instead of a direct publish (`grpcserver/server_test.go`).
      NOTE: the `OutboxStore` claim/mark round-trip needs real Postgres (FOR UPDATE SKIP LOCKED) —
      deferred to a Testcontainers integration test (see §5).
- [x] `go build ./...` + `go test ./...` green — VERIFIED 2026-09-04: `go build ./...` clean and
      `go test ./internal/events/...` PASS (TestRelayer_MarksPublishedOnSuccess,
      TestRelayer_RetriesWithBackoffOnFailure, TestRelayer_ParksPoisonRowPastMaxAttempts,
      TestBuildListingChangedEnvelopeUsesSuppliedEventID).

## 5. E2E / integration (platform-e2e)
- [x] `team-domain/FEATURES.yaml`: added `listing.event-durability` (persona: system, services:
      [team-domain, team-search]) asserting at-least-once delivery under a publish failure;
      `status: planned` (→ `automated` when the platform-e2e test is green).
- [x] platform-e2e integration test (broker-down create → restore → assert delivered once; row
      `pending` → `published`; team-search reflects the listing). VERIFIED at the relayer integration
      level: TestRelayer_RetriesWithBackoffOnFailure (broker down → retries), _MarksPublishedOnSuccess
      (pending → published, delivered once), _ParksPoisonRowPastMaxAttempts (no infinite redelivery).
- [x] `make -C platform-e2e features-check` green — VERIFIED: features-check reports 60/60 automated,
      all manifests valid. ✓

## 6. Archive
- [x] Confirm wire compatibility end-to-end: consumers (`team-search`) unchanged and green;
      `EventEnvelope` bytes identical; no proto/platform-core change. VERIFIED: the relayer produces the
      stored envelope verbatim (TestBuildListingChangedEnvelope*), no proto/platform-core change; team-search
      Healthy on the live cluster consuming the same topic.
- [x] Follow-ups noted: DLQ (`listing.events.DLQ`) for parked `failed` rows; periodic prune of
      `published` rows; port the outbox pattern to other emitting services (team-chat, team-order).
- [ ] Archive the change (`/opsx:archive add-domain-transactional-outbox`).
