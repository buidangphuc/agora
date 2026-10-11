# ADR-0007 — Durable purchase saga + compensation semantics

**Status:** Accepted · **Date:** 2026-09-03 · **Relates to:** ADR-0002 (brokers), ADR-0009

## Context

The checkout flow in `team-order` orchestrates a multi-step purchase (reserve
stock → create order(s) → settle payment) with rollback on failure. As built it
was an **in-memory, single-request best-effort** try/rollback: no persisted saga
state, no recovery after a crash, and compensations ran on the **same request
context** that had just failed — so a timeout (the common failure) also cancelled
the `ReleaseStock` compensations, whose errors were then discarded (`_, _ =`). A
crash after `ReserveStock` but before persisting the order leaks stock forever.
This is a correctness bug, not a limitation: the producer side is engineered for
at-least-once, but the saga silently downgraded the purchase to lossy.

## Decision

- **Persist saga state before external effects.** `team-order` writes a
  `saga_state` (and reservation) row in Postgres before/around each external call,
  so an interrupted saga is recoverable (resume or compensate) rather than lost.
- **Compensations run on a fresh context.** Every compensating action uses
  `context.Background()` with its own deadline — never the failed request ctx — so
  a client timeout/cancel cannot also abort the cleanup. Failed compensations are
  retried and **parked** (never `_, _ =` discarded), mirroring team-domain's
  outbox park-after-max-attempts pattern.
- **Scope compensation to un-committed work.** In multi-seller checkout, a later
  failure must not release stock already committed to persisted orders; either all
  seller-orders persist in one tx, or compensation targets only un-committed orders.
- **Reservations carry a TTL and are swept** (ADR-0008) so leaked reservations are
  reclaimed even if the saga process dies entirely.

## Alternatives rejected

- **Keep in-memory best-effort** — the status quo; loses stock on crash/timeout.
- **Full workflow engine (Temporal/Cadence)** — real durability but a heavy new
  dependency and operational surface unjustified at this stage (ADR-0002 keeps the
  stack fixed). The Postgres-backed saga log gives durability without new infra.
- **2PC across services** — violates DB-per-service (ADR rule 3) and couples
  availability; the saga + compensation model is the deliberate CQRS-era choice.

## Consequences

- Recoverable purchases; no stock leak on crash/timeout; compensation errors are
  observable and retried. Adds a `saga_state`/`reservations` table + a sweeper.
- A small latency cost (persist before effect) on the write path, acceptable for a
  money-adjacent flow. Recovery/resume logic must itself be idempotent.

## Addendum — 2026-10 (change `port-order-inventory-correctness`)

Refines the saga for checkout correctness, order lifecycle guards and retries.

- **Three-phase, all-or-nothing placement.** `CreateOrdersFromCart` (sellers in
  sorted order) runs A) reserve every item, B) `CommitReservation` every item,
  C) place all orders, bind reservations `RESERVED → COMMITTED` with their
  `order_id`, and mark the saga `COMPLETED` in **one Postgres transaction**.
  Any failure before or in C compensates every reservation of the saga
  (*One seller out of stock fails the whole two-seller checkout*; *A two-seller
  checkout places one order per seller*).
- **Attempt-scoped reservation ids**: derived from the saga id, never the
  client key, so a retry after failure never reuses a released reservation
  (*An unkeyed retry after a failed checkout succeeds*).
- **`Idempotency-Key`** (1–255 printable ASCII; gateway forwards as
  `idempotency-key` metadata) is stored on the saga, unique per buyer. A
  completed saga replays its orders; a pending one returns `ABORTED`; a
  compensated/failed saga clears the key, so the key is free again; a stale
  `PENDING` saga is compensated by the sweep (*The same idempotency key returns
  the same orders*; *Concurrent checkouts with one key create one set of
  orders*; *A failed checkout frees its idempotency key*; *Idempotency keys are
  scoped to the buyer*; *A replayed checkout submission creates no second
  order*; *A new checkout after a completed one creates a new order*).
- **One transition table with actor classes, written compare-and-set**
  (`UPDATE … WHERE status = ANY(allowed_from)`), so no status write bypasses the
  table or loses a race silently (*A completed order cannot be reopened*; *A
  seller cannot mark an order paid*; *Skipping from paid straight to completed
  is refused*; *The seller ships a paid order*; *A stranger cannot change an
  order's status*; *Concurrent cancels restore stock once*; *A cancel racing a
  shipment applies exactly one of them*).
- **Cancel claims first**: win the CAS to `Cancelled` (from `Pending|Paid`),
  then release reservations by their original ids, then the voucher hold; a
  failed release is parked and retried by the sweep (*Cancelling a paid order
  restores its stock once*; *Cancelling a shipped order is refused and keeps its
  stock*; *A cancel whose stock release fails is retried until the stock
  returns*). Shipment likewise claims `Shipped` before inserting (*Shipping a
  cancelled order is refused*).
- **Settlement marks only a pending order paid**, and the **voucher hold is
  committed only after `Paid`** is recorded (this or an earlier delivery), so a
  cancelled order's hold is never committed (*A late payment after cancel is
  ignored*; *Payment racing cancel ends cancelled with stock restored once*; *A
  late payment of a cancelled voucher order leaves the voucher unused*).
