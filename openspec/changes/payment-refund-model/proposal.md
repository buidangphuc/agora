## Why

`team-payment` can refund a payment only once: `RefundPayment` moves `PAID → REFUNDED` by compare-and-set and any
refund, even a partial one, closes the payment (`seller-refund-deduction`). The ledger keys the one deduction on the
payment id (`(REFUND_DEDUCTION, payment id)` unique, migration `0006`), so a second partial refund, a return after a
partial refund, or a cancel after a partial refund cannot be expressed. RMA refunds are not real at all: the seller
approves a return in `team-order`, and the storefront's `refundPayment` stub (`team-frontend/src/lib/gateway/payment.ts`)
returns success without calling the gateway, so the buyer is never refunded and the seller is never deducted. The
refund button is also on the buyer's page, where `UpdateReturnStatus` refuses the buyer.

The human decided: cumulative refunds (model B), and RMA refunds driven by a `team-order` fact.

## What Changes

- **platform-core proto (lands first, alone, additive; `buf breaking` clean).**
  - `payment/v1`: the `PAYMENT_STATUS_PARTIALLY_REFUNDED = 5` enum value.
  - `payment/v1`: on `PaymentTransaction`, `refunded_amount = 11` and `repeated PaymentRefund refunds = 12`.
  - `payment/v1`: a new `PaymentRefund` message and a `PaymentRefundSource` enum.
  - `payment/v1`: `RefundPaymentRequest.refund_id = 4` and `WalletEntry.reference_id = 7`.
  - `order/v1`: a new `ReturnRefunded` fact.
  - `order/v1`: `Order.paid_at = 17`, so the storefront can tell an order paid online from a cash-on-delivery one.
  - `order/v1`: a new `OrderService.ListOrderReturns` RPC and its request and response messages.
  - Every vendored copy is re-vendored.
- **team-payment, cumulative refunds.** A payment can be refunded several times: `PAID → PARTIALLY_REFUNDED →
  REFUNDED`, with `REFUNDED` only when the cumulative refunded amount equals the payment amount. Each refund has a
  refund id (its idempotency key), is stored as its own row, and writes exactly one `REFUND_DEDUCTION` for the
  credited seller referencing **that refund**. Refunds of one payment are serialised, so concurrent refunds never
  push the refunded amount past the payment amount.
  - **BREAKING (wire behaviour, decided by the human):** `RefundPayment` now requires `refund_id`. A missing id fails
    with `INVALID_ARGUMENT`.
  - **BREAKING (wire behaviour):** a second refund with a new id succeeds while money remains, where it used to fail
    with `FAILED_PRECONDITION`.
  - **BREAKING (wire behaviour):** an amount above the remainder fails with `FAILED_PRECONDITION`, where it used to
    fail with `INTERNAL`.
- **team-payment, cancel refunds the remainder.** `OrderCancelled` from `Paid` refunds whatever is still refundable,
  once, under the refund id `cancel:<order_id>`.
- **team-payment, refund before credit, generalised.** A credit written after one or more refunds books one deduction
  per refund.
- **team-payment, RMA refunds.** The settlement consumer applies the new `ReturnRefunded` fact as a refund with the id
  `return:<return_id>`. When the return asks for more than the payment still has (because another refund path took
  part of it), team-payment **refunds the remainder and records both the requested and the applied amount**. When
  nothing remains, it records an applied amount of 0. The record is never parked (design D6).
- **team-payment, reads.** `GetPayment` is now readable by the order's seller as well. Hold-back nets each held credit
  against the deductions of that payment's refunds.
- **team-payment, migration `0007`.**
  - Adds a `payment_refunds` table.
  - Backfills one `LEGACY` refund per payment refunded under the old model and re-points that payment's deduction to
    the legacy refund.
  - Legacy partial refunds stay `REFUNDED`, which closes them.
  - Adds status/amount CHECKs (design D9).
- **team-order.**
  - The `APPROVED → REFUNDED` transition of a return becomes a compare-and-set that writes `ReturnRefunded`
    (with the stored `refund_amount`) to its outbox in the same transaction.
  - A return on an order that was never paid online (no `paid_at`, e.g. cash on delivery) cannot be moved to
    `REFUNDED`. The move fails with `FAILED_PRECONDITION`, says cash-on-delivery refunds are handled outside the
    system, and emits nothing (decided by the human).
  - `CreateReturnRequest` caps a return's amount at what the order's other non-rejected returns leave. The check is
    serialised per order.
  - New `ListOrderReturns` RPC.
- **team-gateway.** Routes `ListOrderReturns`. This is a forwarder only, with no logic.
- **team-frontend.**
  - The `refundPayment` stub and `mockRefundAction` are removed. The buyer's returns tab shows the return status
    with no refund control.
  - The seller's order page gets a returns tab where the seller can approve, reject and refund. Refund only moves the
    return to `REFUNDED`.
  - The page also shows the payment's refunded amount and, for each refunded return, the amount the payment actually
    applied, read from the gateway.
- **platform-e2e.** New features for the new capabilities, the modified scenarios updated, the old stub-based
  `order/rma_return.feature` replaced.

## Capabilities

### New Capabilities

- `payment-cumulative-refunds`: covers the partial/full refund status model and the refund id as idempotency key. It
  caps the refunded amount under concurrency, defines the refund fields on the wire, lets the seller read the
  payment, and migrates legacy single-refund rows.
- `return-refund-settlement`: covers the `ReturnRefunded` fact from `team-order` and the per-order return cap. It also
  covers how `team-payment` applies the fact exactly once, clamps it to the remainder, parks facts that cannot be
  applied, and lists an order's returns.
- `seller-return-refund-ui`: covers the storefront RMA flow after the stub is gone. The seller handles returns and
  sees the refund the payment applied. The buyer sees status only.

### Modified Capabilities

- `seller-refund-deduction`: one deduction per refund instead of per payment, refund-before-credit for several
  refunds, and cancel refunds the remainder instead of skipping a partially refunded payment.
- `seller-settlement-credit`: the consumer also handles `ReturnRefunded` (the ignore list changes).
- `seller-payout-holdback`: a held credit is netted against the deductions of its payment's refunds (deductions no
  longer carry the payment id).

## Non-goals

- Real money movement, a real PSP, refunds to a different instrument (AGENTS.md §7: mock only).
- Restocking returned items, return shipping, return evidence or photos, disputes.
- A buyer-initiated refund, or refunding a return without the seller's approval step (`PENDING → REFUNDED` stays
  invalid).
- Per-line-item refunds. A return carries one amount.
- Refunding cash-on-delivery orders in the system. `team-order` refuses the move to `REFUNDED` for them. The DLQ path
  for a `ReturnRefunded` without a paid payment remains only as a defensive fallback (design D7).
- A `PaymentRefunded` event on `payment.events`. Also left out: notifications to the buyer about a refund and
  analytics facts for refunds.
- Blocking a cancel while a return is open.
- Live UI updates. The seller sees the payment's applied refund on reload.

## Impact

- Repos:
  - `platform-core`: proto only.
  - `team-payment`: code, migration `0007`, README, `FEATURES.yaml`.
  - `team-order`: code, README, `FEATURES.yaml` (no schema change).
  - `team-gateway`: forwarder.
  - `team-frontend`: UI, `FEATURES.yaml`.
  - `platform-e2e`.
  - Re-vendor only in `team-analytics`, `team-chat`, `team-domain`, `team-engagement`, `team-notification` and
    `team-search`.
  - Root compose and `platform-gitops`: no change. No new topic, env key or consumer group: `ReturnRefunded` rides
    `order.events` and the existing `team-payment.settlement` group.
- Architecture rules:
  - Rule 1: the storefront calls only the gateway.
  - Rule 2: the gateway only forwards.
  - Rule 3: `team-order` never reads payment data and does not call `team-payment` to approve a refund.
    `team-payment` reads its own DB and calls `team-order` `GetOrder` only to authorise a seller read.
  - Rule 4: the contract change comes first, additively.
  - Rule 5: the new state-change fact goes on Kafka `order.events`, keyed by order id.
- Deploy order: `team-payment` (understands `ReturnRefunded`) before `team-order` emits it. Otherwise the record is
  ignored by the old consumer and lost to it (design Migration Plan).
