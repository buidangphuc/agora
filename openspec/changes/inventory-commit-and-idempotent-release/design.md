## Context

See `proposal.md` — *Why*. Current state relevant to the approach (verified in code):

- `team-domain` owns stock as `INT` columns on `listings` / `listing_variants`; reserve is a conditional
  `UPDATE ... WHERE stock >= $1` inside a tx that also inserts a `reservations` row (status `active|released`,
  `expires_at` = now+15m). `SweepExpiredReservations` selects `status='active' AND expires_at <= now()`
  with `FOR UPDATE SKIP LOCKED` and restores stock.
- `ReleaseStockRequest` already carries `reservation_id = 4`; handler, service and repo ignore it.
- `team-order`'s saga persists `order_reservations` (`PENDING → RESERVED → COMMITTED / RELEASED /
  RELEASE_FAILED / FAILED`); its own sweep only re-releases `RESERVED` and `RELEASE_FAILED`. Its `COMMITTED`
  is local: `team-domain` never learns of it.
- `team-domain` has a transactional outbox (migration 0006, lease + backoff + `SKIP LOCKED`) used today only
  by create/update/delete. `ListingStockChanged` exists in `listing.proto`. **Correction (found by running the e2e on a live stack):** `team-search` does NOT handle it — its consumer only declares the type constant and ignores the event, and the OpenSearch mapping has no `stock` field. Consuming it is split into the follow-up change `search-stock-events`.
- Constraints: Rule 3 (no cross-service DB), Rule 4 (proto in platform-core, vendored copies, generated
  code never hand-edited), Rule 5 (Kafka events via outbox, `listing.events`, `EventEnvelope`), ADR-0008.

## Goals / Non-Goals

**Goals:**

- A committed reservation is never restored by the TTL sweeper.
- Every stock increment is tied to one reservation row and happens at most once.
- Stock changes are observable as events with no dual-write window.
- Cancel cannot silently lose or double-restore stock.

**Non-Goals (design-level):** no ledger/event-sourced stock, no Redis pre-decrement, no change to the
15-minute TTL value, no change to the reserve request/response shape.

## Decisions

### D1 — Add a `committed` state and a `CommitReservation` RPC (vs. raising the TTL or deleting the sweeper)

`active → committed` is set by a new idempotent RPC called by `team-order` at its commit point. The
sweeper's existing `status='active'` predicate then skips committed rows with no sweeper change.
*Alternatives:* raise TTL (only delays the oversell), drop the sweeper (abandoned checkouts would leak
stock forever), or let `team-order` tell domain via an event (async gap reopens the race: the sweep can
fire before the event lands). A synchronous RPC is the only option with no race window.

Commit outcomes: `active → committed` OK; `committed` → OK (idempotent); `released` →
`FAILED_PRECONDITION`; unknown → `NOT_FOUND`. The proto change is additive (new RPC + messages) so
`buf breaking` passes.

### D2 — Release is keyed by reservation, restores the stored quantity

One statement does the state change and returns what to restore:
`UPDATE reservations SET status='released', released_at=now() WHERE reservation_id=$1 AND status IN ('active','committed') RETURNING listing_id, variant_id, quantity`.
Zero rows → no-op success (already released/swept or unknown-but-previously-released). One row → add the
returned quantity in the same tx. Committed reservations are releasable because cancel-after-payment must
give stock back. The caller-supplied quantity is ignored, removing a second source of over-restore.
*Alternative:* dedupe table keyed `(reservation_id, action)` as in the vault's `inventory_transaction_logs`
pattern. Rejected: the `reservations` row already is the ledger entry; a second table duplicates state.
The sweeper uses the same predicate (`status='active'`), so sweep and release can never both restore.

Unknown `reservation_id` on release returns success no-op rather than an error so a retry after a lost
response is safe; an **empty** id returns `INVALID_ARGUMENT` (rolled out after callers are migrated, task 2.6).

### D3 — Cancel uses the original reservation id

`team-order` currently builds a synthetic `cancel:<itemID>` id, which can never match the reserve row. The
order's reservations are already in `order_reservations`; cancel reads them by `order_id` and releases by
their real ids. Because release is idempotent (D2) a repeated cancel is safe.

### D4 — Cancel: claim first, release by winner, park failures

Aligned with `order-integrity-guards` (D-cancel): `CancelOrder` first claims `Cancelled` with the atomic
compare-and-set; only the winner loads the order's reservations and releases each by its real id. A failed
release marks that `order_reservations` row `RELEASE_FAILED`, which the existing sweep retries; the sweep
also picks up reservations still unreleased on a `Cancelled` order (crash between claim and release).
*Why not release-then-flip:* the order could ship between the read and the flip, and two concurrent cancels
would both release. *Alternative:* a `team-order` outbox — rejected here; that is the larger `order.events`
work (ADR-0012) and unnecessary because the saga table is already durable. This change owns release
idempotency (D2); the claim/CAS itself lives in `order-lifecycle-guards`.

### D5 — Stock events reuse `ListingStockChanged` on the existing outbox

`enqueue` is called inside the reserve / release / sweep transactions (not the handler), right after the
stock update, only when stock actually changed (idempotent no-ops write nothing). Keyed by `listing_id`,
same relayer, same topic `listing.events`. *Alternative:* a new `listing.stock` topic — rejected
(breaks the `<domain>.events` convention and would fork the search consumer).

### D6 — DB invariants added `NOT VALID`, then validated

`CHECK (stock >= 0)` on both stock columns and `CHECK (quantity > 0)` on `reservations`, plus a status
`CHECK` including `committed`. Added `NOT VALID` first so existing rows cannot block the deploy, then
`VALIDATE CONSTRAINT` in a separate step after a data query shows no violations.

### D7 — Reserve retry against a released reservation fails

`ReserveStockIdempotent` today returns success for any existing row. It becomes: `active`/`committed` →
success no-op; `released` → precondition error (the client must reserve under a new id), so `team-order`
cannot place an order on stock it does not hold.

## Risks / Trade-offs

- [Mixed-version rollout: team-order calling an old team-domain without `CommitReservation`] → deploy
  order is proto → team-domain → team-order; team-order treats `UNIMPLEMENTED` as "domain not upgraded" and
  logs, keeping today's behaviour until domain is upgraded.
- [Existing negative/odd stock rows break the CHECK] → `NOT VALID` first, validate after a count query.
- [Orders placed before the fix still have `active` domain reservations that the sweeper would restore] →
  a SQL backfill across databases is impossible (Rule 3), so `team-order` runs a one-off re-sync (task 4.4)
  that calls `CommitReservation` for every local `COMMITTED` row; run it right after deploying
  `team-domain` and before the next sweeper interval.
- [Release of unknown id returning success can mask bugs] → log at warn with the id and count a metric.
- [Extra synchronous RPC on the checkout path] → one small indexed UPDATE per reservation, issued after the
  order is persisted; failure handling per D1/D7.

## Migration Plan

1. platform-core proto PR (additive) merged; re-vendor + regenerate in team-domain, team-order.
2. team-domain: migration (constraints `NOT VALID`, status), code, deploy. Then run `VALIDATE CONSTRAINT`.
3. team-order: commit call + cancel rewrite + commit re-sync job, deploy.
4. Rollback: team-order can be rolled back independently (it only stops committing; TTL behaviour returns).
   team-domain migration is additive; constraints can be dropped.

## Open Questions

- Whether `team-order` should surface "reservation expired" as a dedicated gRPC code to the gateway
  (currently maps to a generic failed-precondition); does not change the specs or task breakdown.
