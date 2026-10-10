# ADR-0008 — Inventory reservation model (idempotent + TTL)

**Status:** Accepted · **Date:** 2026-09-03 · **Relates to:** ADR-0007

## Context

`team-domain` owns stock and exposes `ReserveStock` / `ReleaseStock` to the
purchase saga (ADR-0007). Two correctness gaps existed: (1) `ReserveStockRequest`
already carries a `reservation_id` field, but the caller never set it, so a retried
checkout (network blip, saga resume) **double-decremented** stock; (2) a reservation
had no expiry, so any reservation the saga failed to release (crash, discarded
compensation) leaked stock permanently. Reservation is a core marketplace
correctness concern and had no ADR.

## Decision

- **Idempotent reserve keyed by `reservation_id`.** The caller sets a stable
  `reservation_id` per `(cart_item, attempt)`. `team-domain` records applied
  reservations `(reservation_id PK, listing_id, qty, expires_at)` and treats a
  repeat with the same id as a **no-op** returning the prior result. Re-delivery and
  saga resume are therefore safe.
- **Reservations expire and are swept.** Each reservation has `expires_at` (default
  TTL, e.g. 15m). A periodic sweeper in `team-domain` releases expired reservations
  and restores stock. The sweep is idempotent and re-runnable.
- **Release is idempotent too** — releasing an unknown/already-released id is a
  no-op, so compensation retries (ADR-0007) never over-release.

## Alternatives rejected

- **No reservation ledger, decrement directly** — cannot dedupe retries; the
  double-decrement bug.
- **Distributed lock per listing** — serialization + a new dependency; the
  idempotency-key ledger achieves correctness without contention.
- **Rely on the saga to always release** — proven false: process death leaks stock.
  TTL sweep is the backstop.

## Consequences

- Retried/ resumed checkouts are safe; leaked reservations self-heal within one TTL.
- Adds a `reservations` table + a sweeper goroutine/job in team-domain. Callers MUST
  pass `reservation_id` — enforced by the team-order task; an unset id is a bug.
- TTL is a tunable: too short risks releasing a valid in-flight reservation; start at
  15m and revisit against real checkout duration (an NFR to quantify).

## Addendum — 2026-10 (change `port-order-inventory-correctness`)

Supersedes the points below; the original decisions otherwise stand.

- **Lifecycle.** A reservation is `active → committed | released`, and
  `committed → released`. Only `active` reservations expire; the sweep restores
  stock for `active` rows past `expires_at` only (scenario: *A placed order keeps
  its stock after the reservation TTL*).
- **`CommitReservation(reservation_id)`** (`ListingService`, additive RPC) moves
  `active → committed` so a placed order's stock is never swept. Committed
  stays releasable so cancel-after-payment returns stock. Unknown or released id
  is `FAILED_PRECONDITION` (*A placed order keeps its stock…*).
- **Release is keyed by `reservation_id`** and restores the *stored* quantity;
  the request's `listing_id`/`variant_id`/`quantity` are ignored (kept for wire
  compatibility). One conditional `UPDATE … WHERE status IN ('active','committed')`
  plus the stock restore in one transaction, using the same predicate as the sweep,
  so release and sweep never both restore. Unknown/already-released is a
  successful no-op (*A failed checkout returns its stock once even after the TTL
  sweep*; *A cancelled order's stock is returned once even after the TTL sweep*).
- **A released id refuses reserve** (`FAILED_PRECONDITION`); callers use a new id.
  An empty `reservation_id` is `INVALID_ARGUMENT` on reserve and release; the
  ledger-less decrement is removed (*A checkout holds the ordered quantity
  exactly once*).
- **Stock events.** Each stock-changing transaction (reserve, release, sweep)
  enqueues `ListingStockChanged` (absolute stock after the change) to the outbox
  in the same transaction (*A checkout announces the listing's new stock*; *A
  cancel announces the restored stock*).
- **Configurable timing.** `RESERVATION_TTL` (default 15m) and
  `RESERVATION_SWEEP_INTERVAL` (default 1m) in `team-domain` and `team-order`;
  invalid or non-positive values fall back to the default with a WARN, never
  failing boot (*An invalid reservation TTL falls back to the default*; *The e2e
  stack runs the sweeper on the configured short cadence*).
