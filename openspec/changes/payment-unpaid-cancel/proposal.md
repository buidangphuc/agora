## Why

When team-order cancels an unpaid order (buyer cancel, or the new 24-hour expiry with reason `PAYMENT_EXPIRED` from `order-unpaid-expiry`), team-payment today does nothing: `handleCancelled` in `team-payment/internal/consumer/settlement.go` acts only when `previous_status` is PAID. The payment transaction stays PENDING or FAILED and `ProcessMockPayment` will still settle it, and a settle that lands after the cancel is "acknowledged without change" by team-order, so the buyer pays for an order that no longer exists and nobody refunds them. The q10-order-lifecycle initiative (locked 2026-10-16, user story 1 edge case "a payment that settles after the expiry cancel leaves the order Cancelled, credits no seller, and the buyer's payment is refunded in full once") requires that a cancelled unpaid order cannot be paid and a late payment is refunded.

initiative: q10-order-lifecycle

## What Changes

- **team-payment**
  - Pre-deploy data (decision B7): no backfill in team-payment. Pre-deploy unpaid orders get no `payment_due_at` in team-order, so they are never expired by the sweep; a manual buyer cancel of one voids its transaction like any PENDING cancel.
  - `internal/repository/payment.go`: new `PaymentStatusCancelled PaymentStatus = 6` mirroring `PAYMENT_STATUS_CANCELLED = 6` of the vendored proto (added by `q10-order-lifecycle-contract`; the proto copy under `team-payment/proto` and its generated Go are refreshed from it, never hand-edited). No migration: `payment_transactions.status` is an unconstrained INT and the refund CHECKs of `0007` only constrain statuses 4 and 5.
  - New repository operation `CancelUnpaid(orderID)`: a compare-and-set `PENDING|FAILED -> CANCELLED` on the order's transaction, returning whether it voided, was already CANCELLED, was absent, or was found in another status. No `PaymentSettled` outbox row is written.
  - `internal/consumer/settlement.go` `handleCancelled`, by `OrderCancelled.previous_status`:
    - PAID: unchanged (`RefundCancelledOrder`).
    - PENDING (new): `PaymentService.VoidCancelledOrder(orderID)`: void a PENDING or FAILED transaction; if the transaction is already PAID (a settle that beat the cancel, or lost the compare-and-set to it) refund its whole remainder through the existing `refund` path keyed `cancel:<order_id>` (one refund per order, so redelivery and the PAID path never double refund); no transaction, or already CANCELLED, is a no-op.
    - any other status: unchanged (no effect).
  - `ProcessMockPayment` (service and handler): a CANCELLED transaction refuses both success and failure simulation with `FAILED_PRECONDITION` (new `ErrPaymentCancelled`, "payment cancelled"), after the existing buyer-or-admin check. A success settle that loses its compare-and-set to a void reports the same refusal (`lostSettleRace`). Before a success settle, `ProcessMockPayment` re-reads the order through the existing team-order `GetOrder` client (decision M1): a CANCELLED order refuses the settle with `FAILED_PRECONDITION` and voids the PENDING or FAILED transaction to CANCELLED; an unreachable team-order returns `UNAVAILABLE` and settles nothing (fail closed). This closes the `CreatePayment`-vs-cancel race.
  - `GetPayment` / `CreatePayment` return the CANCELLED status unchanged on the wire; `CreatePayment` keeps refusing a non-PENDING order (`ErrInvalidOrderState`), so a cancelled order cannot open a new transaction.
  - No seller wallet change: a late payment on a cancelled order never produced an `OrderPaidEvent`, so no `ORDER_SETTLEMENT` exists; the refund therefore writes no `REFUND_DEDUCTION`.
- **platform-e2e** (feature and steps owned by this slice): `features/payment/unpaid_cancel.feature` plus step bindings; uses the short payment deadline from `compose/order-inventory.override.yaml` owned by `order-unpaid-expiry`.
- Not changed: `ReturnRefunded` refunds, the payout and hold logic, `RefundPayment` (seller/admin), the payment proto beyond the vendored enum, team-order, the gateway, the frontend (`frontend-order-lifecycle` renders the expired state).

## NFR

- Consumer latency: the void is one compare-and-set UPDATE plus at most one refund transaction, p99 < 200 ms per record, no new upstream call on the consumer path.
- Settle latency: a success `ProcessMockPayment` adds one team-order `GetOrder` call under the existing upstream client deadline; p99 of the RPC < 500 ms on the local stack.
- Availability: the consumer keeps its at-least-once model (bounded retry, DLQ `order.events.payment-settlement.dlq`, commit after apply or DLQ); a DB outage retries, it never skips a cancel.
- Idempotency: voiding twice, void after refund, and refund after void each leave one end state; a late payment is refunded exactly once (key `cancel:<order_id>`).
- Limits: no new limits; money is int64 VND, the refund equals the transaction amount.
- Ordering tolerance: correct whether the cancel event or the settle is applied first (see threat model).

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| transaction `status` CANCELLED (new value) | business state, not personal | stop a cancelled order being paid | contract performance | with the existing transaction row (unchanged) |
| refund row `cancel:<order_id>` (existing shape) | financial, not personal | return a late payment | contract performance | unchanged |

No new personal data, no new event, no log field beyond order and payment ids.

## Threat model

- **Assets:** buyer money (a payment taken for a dead order), seller wallet integrity, the transaction status machine.
- **Abuse cases:**
  - Buyer opens the pay page before expiry, keeps the tab, pays after the cancel: after the cancel event the transaction is CANCELLED and `ProcessMockPayment` refuses; if the settle wins the race, the cancel consumer refunds it in full.
  - Replaying `ProcessMockPayment` after the cancel to flip CANCELLED back to PAID: refused, the CAS only reopens PENDING or FAILED.
  - Seller credited for an order that was cancelled: impossible, no `OrderPaidEvent` exists; refund writes no deduction.
  - Redelivered or duplicated `OrderCancelled`: key `cancel:<order_id>` and the status CAS make it a no-op.
  - A user voiding someone else's payment: no RPC voids; only the `order.events` consumer does, from a team-order outbox event.
  - Poison event: missing `order_id` is `ErrPermanent` and goes to the DLQ.
- **Authz rule:** unchanged. `ProcessMockPayment` stays buyer-or-admin on the transaction's buyer; a CANCELLED transaction does not widen or narrow it (non-owner still `PERMISSION_DENIED`, checked before the status refusal). The void/refund path has no caller principal: it runs from the consumer only.
- **Race closed (M1):** a `CreatePayment` that read the order as PENDING just before the cancel can insert a transaction after the cancel event was consumed. That transaction cannot be paid: the settle re-reads the order, sees CANCELLED, voids the transaction and refuses. If the re-read sees PENDING, the cancel happens after it, so its event is applied after the settle and the late-payment refund runs. See design D6.
