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
