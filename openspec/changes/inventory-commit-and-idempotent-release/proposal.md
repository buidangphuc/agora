## Why

Verified by reading the code, stock is restored for orders that were successfully placed. `team-domain`'s
15-minute reservation sweeper re-adds stock for every `active` reservation, and nothing ever tells
`team-domain` that `team-order` committed the reservation — there is no commit RPC and no `committed`
status. Every placed order therefore oversells its stock 15 minutes later. `ReleaseStock` also ignores the
`reservation_id` it is sent (blind `stock = stock + n`), so retried compensations and cancels can
double-restore, and ADR-0008's "release is idempotent" is not true in code. Stock changes emit no event,
so search shows stale stock. Fixing this now matters because every checkout hits it.

## What Changes

- **platform-core**: add an additive `CommitReservation` RPC to `platform/listing/v1/listing.proto`
  (idempotent on `reservation_id`); document the `committed` reservation status.
- **team-domain**:
  - New migration: reservation status gains `committed`; `CHECK (stock >= 0)` on `listings` and
    `listing_variants`; `CHECK (quantity > 0)` on `reservations`.
  - Implement `CommitReservation`; the sweeper keeps skipping anything that is not `active`.
  - `ReleaseStock` becomes idempotent on `reservation_id`: it releases the quantity stored on the
    reservation row exactly once; a repeat, or a release of an already released/swept reservation, is a
    successful no-op.
  - Reserve, release and sweep enqueue `ListingStockChanged` (already defined in proto; `team-search` does not consume it yet —
    see `search-stock-events`) on the existing outbox inside the same transaction.
  - Fix the reserve retry quirk: a retry against a `released` reservation must not report a false success.
- **team-order**:
  - Call `CommitReservation` when the order is persisted (saga commit point).
  - Cancel releases using the order's original reservation id (not the synthetic `cancel:` id) and
    records a failed release durably (`RELEASE_FAILED`, retried by the existing sweep); the `Cancelled`
    claim itself is atomic and owned by `order-integrity-guards`.
- **team-search**: NOT in this change. The existing consumer ignores `ListingStockChanged` and the index has no `stock` field; making search reflect stock is the follow-up change `search-stock-events` (this change only emits the event).
- **Configurability**: reservation TTL and sweeper intervals become configurable (`team-domain`: `RESERVATION_TTL`, `RESERVATION_SWEEP_INTERVAL`; `team-order`: sweep interval), defaulting to today's values, so the TTL scenarios can be proven end to end.
- Test fix: `TestSweepExpiredReservations` currently asserts the buggy behaviour for committed rows and
  is corrected.
- **BREAKING (internal only)**: an empty `reservation_id` on `ReleaseStock` is rejected with
  `InvalidArgument` once callers are migrated (task 2.6); the gateway and frontend are unaffected.

## Capabilities

### New Capabilities
- `inventory-reservations`: stock reservation lifecycle across `team-domain` and `team-order` — reserve,
  commit, idempotent release, TTL expiry, stock invariants, stock-change events, and cancel compensation.

### Modified Capabilities
<!-- listing-write already requires "stock changes" to be recorded in the same transaction as the
     event (spec.md); the code does not do it. That is an implementation gap, not a requirement
     change, so no delta is written for it. -->

## Impact

- Repos: `platform-core` (proto), `team-domain`, `team-order`, `team-search` (out of scope, see above). Gateway and
  frontend do not change (service-to-service RPC).
- Contract: one additive RPC + messages; must pass `buf lint` and `buf breaking`; re-vendor and
  regenerate in `team-domain` and `team-order` (never hand-edit `generated/`).
- Data: one `team-domain` migration (constraints need a data check/backfill first, added `NOT VALID`
  then validated).
- Events: reuse `ListingStockChanged` on `listing.events`; no new topic.
- Architecture rules: Rule 3 (team-order calls team-domain over gRPC only, never its DB), Rule 4 (proto
  is the contract), Rule 5 (Kafka domain events via the existing outbox).

## Non-goals

- No Redis pre-decrement / flash-sale path, no append-only stock ledger, no warehouse or multi-location
  model.
- No order status state machine, checkout Idempotency-Key or in-memory-fallback fail-fast (separate
  change `order-integrity-guards`).
- No new Kafka topic and no `order.events` (ADR-0012 is a separate change).
- No change to gateway, frontend or auth scopes.
