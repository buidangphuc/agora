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

## Addendum (2026-10): seller ledger driven by `order.events`

Status: accepted. Change: `openspec/changes/port-payment-ledger-integrity`.

- **The seller settlement credit is driven by `OrderPaidEvent`.** `team-payment` now
  also consumes `order.events` (consumer group `team-payment.settlement`, dead-letter
  topic `order.events.payment-settlement.dlq`). Each `platform.order.v1.OrderPaidEvent`
  appends one `ORDER_SETTLEMENT` credit for the order's seller, of the payment
  transaction's amount, referencing that transaction. `team-order` writes the event once,
  in the same transaction as the only `Pending → Paid` compare-and-set, so a payment
  that loses to a cancel (late payment) never credits the seller.
- **The inline credit is gone.** `ProcessMockPayment` writes no ledger entry; the
  credit appears asynchronously after the payment (two outbox hops).
- **Cancelling a paid order refunds automatically.** `team-order` writes
  `platform.order.v1.OrderCancelled` (with `previous_status`) to `order.events` through
  its outbox, in the cancel claim's transaction. The same `team-payment` consumer
  refunds the payment in full when `previous_status` is `PAID`, and leaves an already
  refunded payment alone.
- **Refund deduction and payout hold-back live in `team-payment`'s ledger.** A refund
  moves `PAID → REFUNDED` by compare-and-set and writes exactly one `REFUND_DEDUCTION`
  for a credited payment, whichever of the refund and the credit is applied first
  (both serialise on the payment row; the ledger's unique `(type, reference_id)` index
  makes each write once). Payouts draw only on proceeds outside the refund hold window
  (`PAYOUT_HOLD_DAYS`, default 7).
- Delivery stays at-least-once: the consumer commits only after the ledger write or the
  DLQ produce, and redelivery is a no-op through the unique index (no dedupe table).
- **`payment.events` stays single-type** (`PaymentSettled`, emitted only as `PAID`). The
  payment outbox claim can let a later row of one order overtake an earlier one; that
  is not observable today, but per-order ordering must be ported before a second event
  type (e.g. `PaymentRefunded`) joins `payment.events`.
