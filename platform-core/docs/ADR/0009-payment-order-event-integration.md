# ADR-0009 — Payment→Order integration is event-carried

**Status:** Accepted · **Date:** 2026-09-03 · **Relates to:** ADR-0002, ADR-0005, ADR-0007

## Context

When a (mock) payment settles, the order must move to PAID. As built,
`team-payment.ProcessMockPayment` set `payment = PAID` in its own DB and then made
a **synchronous** `order.UpdateOrderStatus(PAID)` RPC, logging-and-ignoring any
failure. That is a **dual-write**: if the RPC fails or the order service is briefly
down, payment is PAID but the order is stuck PENDING, with no reconciliation — a
money-adjacent inconsistency decided ad hoc in code, with no ADR.

## Decision

- **Payment emits an event; order consumes it.** On settle, `team-payment` writes
  `payment = SETTLED` **and** a `platform.payment.v1.PaymentSettled` outbox row in
  **one transaction**, then a relayer publishes it at-least-once to Kafka
  `payment.events` (key = `order_id`, wrapped in `EventEnvelope`, `event_id` =
  outbox row id) — the same transactional-outbox pattern team-domain uses (ADR-0005).
- **team-order consumes idempotently.** It dedupes on `event_id`/`payment_id` and
  transitions the order to PAID exactly once. Re-delivery is safe.
- **No synchronous cross-service state-coupling call** remains on the settle path.

## Alternatives rejected

- **Keep the synchronous RPC** — the dual-write; the bug.
- **Synchronous RPC + a reconciler job** — still dual-writes on the happy path and
  adds a second moving part; the outbox makes the write atomic at the source.
- **RabbitMQ job** — this is a **state-change event**, not a background job, so per
  ADR-0002 it belongs on Kafka, not RabbitMQ.

## Consequences

- Payment and order converge reliably; no PAID-payment / PENDING-order split. Adds a
  payment outbox table + relayer in team-payment and a `payment.events` consumer in
  team-order. Order transition is now **eventually** consistent (consumer lag) rather
  than synchronous — acceptable, and consistent with the CQRS posture; lag is an SLO
  to quantify. New topic `payment.events` must be provisioned (integration wave).
