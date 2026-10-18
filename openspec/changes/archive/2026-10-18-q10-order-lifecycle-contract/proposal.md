## Why
The q10-order-lifecycle initiative stops orders stalling after checkout: unpaid orders are reminded and expire, shipped orders are confirmed or auto-completed, returns get a window and a seller deadline, sellers get work notifications, and the cancel button sends a reason code. Every backend and frontend slice needs the same wire contract first. This slice adds the `ConfirmReceipt` RPC, two enums, new `Order` / `OrderReturn` / event fields, four new `order.events` messages and `PAYMENT_STATUS_CANCELLED`, plus the proto copies consumers carry. Behaviour lives in `order-unpaid-expiry`, `payment-unpaid-cancel`, `order-receipt-completion`, `notification-order-work` and `frontend-order-lifecycle`. This slice merges FIRST across q10/q11 and owns `Order` field numbers 18-24 (q11-checkout-coherence uses 25+; this slice's test does not assert anything about 25+).

initiative: q10-order-lifecycle

## What changes
`platform-core/packages/proto/platform/order/v1/order.proto` (additive only):
- `OrderService` gains `rpc ConfirmReceipt(ConfirmReceiptRequest) returns (ConfirmReceiptResponse)`; `ConfirmReceiptRequest { string id = 1; }`, `ConfirmReceiptResponse { Order order = 1; }`. Comment: buyer of the order only; SHIPPED to COMPLETED; allowed while the buyer's return is PENDING or APPROVED; `PERMISSION_DENIED` for any other caller, `FAILED_PRECONDITION` for a non-SHIPPED order.
- `enum CancelReason`: `CANCEL_REASON_UNSPECIFIED=0`, `CHANGED_MIND=1`, `WRONG_ITEM=2`, `DAMAGED=3`, `DELIVERY_DELAY=4`, `OTHER=5`, `PAYMENT_EXPIRED=6` (system only), `ADMIN_FORCE_FAIL=7` (ForceFailSaga only). Comments map 1-4 to the FE strings `changed_mind`, `wrong_item`, `damaged`, `delivery_delay`.
- `enum CompletedBy`: `UNSPECIFIED=0`, `BUYER=1`, `SELLER=2`, `SYSTEM=3`.
- `CancelOrderRequest.reason_code = 3` (`CancelReason`); comment: a buyer may not send `PAYMENT_EXPIRED` or `ADMIN_FORCE_FAIL` (`PERMISSION_DENIED`); the free-text `reason = 2` stays.
- `Order` fields 18-24: `payment_due_at=18`, `ship_by=19`, `shipped_at=20`, `completed_at=21`, `return_window_ends_at=22` (all `google.protobuf.Timestamp`), `CancelReason cancel_reason=23`, `Timestamp auto_complete_at=24`. Comments carry the derivation (`payment_due_at` created_at + 24h, unset for COD and for orders placed before the deploy (never auto-expired; backfill of the other deadlines is owned by order-unpaid-expiry / order-receipt-completion); `ship_by` paid_at or COD created_at + 2d; `return_window_ends_at` completed_at + 15d; `auto_complete_at` shipped_at + 7d, unset otherwise). Durations are env config read when the field is snapshotted.
- `OrderReturn.can_escalate = 10` (bool): server-computed at read, true when the return is REJECTED and the order is SHIPPED or inside its return window; re-escalation after a closed dispute is allowed while it is true.
- `OrderCancelled.reason_code = 8` (`CancelReason`).
- `OrderPaidEvent.seller_id = 7` (string), `OrderPaidEvent.ship_by = 8` (Timestamp).
- New `order.events` messages (outbox, key = order id, `EventEnvelope.type` = full name): `OrderPlaced`, `OrderPaymentReminder`, `OrderCompleted`, `ReturnRequested`, with the field numbers from `plans/q10-order-lifecycle/00-spec.md` Contracts (verbatim). Comments: `OrderPlaced` is emitted for every placed order, `payment_due_at` unset for COD, `ship_by` set for COD only; `OrderPaymentReminder` once per order at `payment_due_at - 2h`; `OrderCompleted` once per order; `ReturnRequested` once per created return.

`platform-core/packages/proto/platform/payment/v1/payment.proto`: `PaymentStatus` gains `PAYMENT_STATUS_CANCELLED = 6` (order cancelled before payment; `ProcessMockPayment` refuses it). No existing value moves.

Copies: `order.proto` byte-identical to `proto/platform/order/v1/order.proto` of team-frontend, team-order, team-engagement, team-gateway, team-domain, team-notification, team-chat, team-payment, team-search, team-analytics; `payment.proto` byte-identical to `proto/platform/payment/v1/payment.proto` of team-chat, team-gateway, team-frontend, team-payment, team-search, team-analytics, team-notification, team-order, team-domain, team-engagement (all identical to the source today). Consumers regenerate in their own slices and never edit the copy.

A committed Go test `platform-core/packages/proto/platform/order/v1/lifecycle_compat_test.go` (same approach as `analytics/v1/loopevents_compat_test.go`) pins RPCs, enums, messages, field numbers, comment text and wire round-trips.

Out of scope: any runtime behaviour (timers, sweeps, notifications, UI), gateway routes for `ConfirmReceipt` (added in order-receipt-completion), dispute messages (q6-trust-enforcement), renumbering any existing field.

## NFR
- Additive only: `buf breaking` against the base commit passes before the slice commit (the mutation check runs after it, against HEAD); consumers unaware of the additions keep working (unknown fields skipped, unknown enum values parse as numbers).
- No new service, port or dependency. Contract-only: no runtime latency or availability impact.
- Compat: old bytes of existing messages are unchanged; a new `Order` parses in an old schema (fields 18-24 skipped).
- Limits fixed by the contract: all money int64 VND; timestamps are UTC `google.protobuf.Timestamp`.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `order_id`, `buyer_id`, `seller_id`, `return_id` in the new events | pseudonymous identifiers (existing data, no new class) | route notification, payment and reward consumers | contract performance, already collected | as the existing `order.events` topic; order-unpaid-expiry and order-receipt-completion must state it |
| `total_amount`, `refund_amount`, `currency` | business data | amount shown in notification, refund keying | contract | same |
| deadline and lifecycle timestamps, `cancel_reason`, `reason_code`, `completed_by`, `can_escalate` | non-personal | drive timers, UI and escalation | contract | lives with the order |

No new personal data: events carry ids, amounts and timestamps only, no address, name or payment instrument. The free-text `CancelOrderRequest.reason` already exists and is unchanged.

## Threat model
- Assets: order state integrity (no cancel of a paid order by a forged system reason); buyer money (a payment for a cancelled order); seller payout (completion triggers downstream reward and dispute rules).
- Abuse cases: a buyer sending `PAYMENT_EXPIRED` or `ADMIN_FORCE_FAIL` to disguise a cancel as a system one (the contract documents `PERMISSION_DENIED`; enforcement and tests live in order-unpaid-expiry); a non-buyer calling `ConfirmReceipt` (documented `PERMISSION_DENIED`, enforced in order-receipt-completion); a consumer copy drifting from the source (mitigated by the `cmp` check); a renumbered field silently reinterpreting old data (mitigated by `buf breaking` and the pinned descriptor test).
- Authz rule: `ConfirmReceipt` is callable only by the buyer of the order; system-only `CancelReason` values are never accepted from a buyer. This slice documents the rules; enforcement belongs to the owning slices.
