# Design

## Context

team-payment consumes `order.events` in `internal/consumer/settlement.go` (dedupe by idempotent ledger keys, DLQ, `ErrPermanent`). team-order cancels unpaid orders and emits `OrderCancelled{previous_status, reason_code}` (contract slice, emitted by `order-unpaid-expiry`). Payment settle is `ProcessMockPayment` -> `settlePaid` (CAS from PENDING|FAILED, writes `PaymentSettled` to the outbox). team-order ignores a `PaymentSettled` for a cancelled order.

## Decisions

- D1. Void on `previous_status == PENDING`, regardless of `reason_code`: the buyer cancel from Pending has the same problem as the expiry, and the initiative decision names `previous_status PENDING` as the trigger.
- D2. Void is a compare-and-set `UPDATE ... SET status=6 WHERE order_id=$1 AND status IN (1,3)`; when it updates 0 rows, re-read the row: PAID/PARTIALLY_REFUNDED/REFUNDED means a settle won, go to the refund path; CANCELLED or none means no-op. This makes either ordering of cancel event and settle converge.
- D3. Late settle refund reuses `RefundCancelledOrder` semantics (remainder mode, key `cancel:<order_id>`, source order-cancel, reason `order_cancelled`), so the Q-existing exactly-once guarantee and the REFUNDED end state apply. Because the order never emitted `OrderPaidEvent`, `CreditSettlement` never ran, so no deduction is written; if `ErrNotSettled` is returned the consumer treats it as permanent only on the PAID path as today; on the PENDING path "no settled transaction" is the normal no-op.
- D4. `PAYMENT_STATUS_CANCELLED` is terminal. `ProcessMockPayment` refuses it with `FAILED_PRECONDITION` (existing code family used for `ErrPaymentRefunded`), checked after buyer-or-admin so non-owners learn nothing.
- D5. No new RPC, event or topic. No `PaymentCancelled` event: team-order already knows the order is cancelled.
- D6. `CreatePayment`-vs-cancel race (decision M1): `ProcessMockPayment` re-reads the order with the existing `orderClient.GetOrder` before a success settle. Order CANCELLED: `CancelUnpaid` voids the transaction (PENDING|FAILED -> CANCELLED) and the call returns `ErrPaymentCancelled`; team-order unreachable: `ErrOrderServiceUnavailable` (`UNAVAILABLE`), nothing settled; any other order status: settle as today. The failure simulation does not re-read (it moves no money). Ordering argument: if the re-read sees PENDING, the cancel commits after it, so its `OrderCancelled` is consumed after the settle CAS and the late-payment refund (D3) runs; if the transaction was inserted after the cancel was consumed, the re-read sees CANCELLED. Either way no paid transaction stays on a cancelled order.
- D7. `previous_status` PENDING with a PARTIALLY_REFUNDED transaction (a seller or admin refunded part of a late payment before the cancel was applied) goes to the remainder refund under `cancel:<order_id>` and ends REFUNDED; a fully REFUNDED transaction is a no-op.
- D8. Pre-deploy data (decision B7): no team-payment migration or backfill. Unpaid pre-deploy orders have no `payment_due_at`, so only a buyer cancel can void their transaction.

## Testing note

A late settle after the cancel cannot be produced end to end (the status is CANCELLED once the event is applied); it is covered by consumer-plus-Postgres tests (`internal/pgtest`) that apply the settle first, then the event.

## Clarifications

### Round 1 — 2026-10-16

- M1 [money] Settle on an order already cancelled upstream is refused and voided: can a transaction opened after the cancel was consumed still be paid? → **No. ProcessMockPayment re-reads the order (team-order GetOrder) before settling; a CANCELLED order refuses settle, closing the CreatePayment-vs-cancel race** (human)
- B7 [data] Pending transaction is voided on unpaid cancel: what happens to orders placed before the deploy? → **Unpaid pre-deploy orders get no payment_due_at and are never auto-expired; no team-payment backfill** (human)
- R1 [money] Partially refunded late payment is refunded the remainder once: does a PARTIALLY_REFUNDED transaction get the remainder? → **Yes, once under cancel:<order_id>, ending REFUNDED; a full REFUNDED transaction is a no-op** (human, reviewer recommendation accepted)
- R2 [nfr] Shipped-order cancel changes no transaction: one scenario covered SHIPPED and PAID and latency had no number → **Split into two scenarios; consumer p99 < 200 ms per record, settle RPC p99 < 500 ms** (human, reviewer recommendation accepted)

