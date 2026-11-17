# Proposal: payment-settlement-commission

initiative: monetization

## Why
The platform earns nothing on an online-paid order: `team-payment` credits the seller the full payment amount at
settlement (`CreditSettlement`, `Amount: t.Amount`). The monetization initiative books a flat 5% commission against
the seller's wallet at settlement, returns it in proportion on refunds, and lets the seller see gross, fee and net.
Side finding: the paid amount is net of `platform_discount_amount`, so today the seller funds the platform voucher;
the locked decision is that the platform funds it.

## What changes
- `team-payment` reads the new price breakdown on `OrderPaidEvent` (`items_subtotal`, `shipping_fee`,
  `shop_discount_amount`, `platform_discount_amount`; provided by `order-paid-price-breakdown`) and, in the same DB
  transaction as the `ORDER_SETTLEMENT` credit, appends one `COMMISSION` ledger entry of
  `-floor((base x rate_bps + 5000) / 10000)` where `base = items_subtotal - shop_discount_amount`.
- The `ORDER_SETTLEMENT` credit becomes the amount paid plus `platform_discount_amount` (MODIFIES
  `seller-settlement-credit`). An event without the breakdown keeps the old credit and books no commission.
- A refund of a settled payment appends one `COMMISSION_REVERSAL` entry (positive) in the same transaction as the
  `REFUND_DEDUCTION`, so the cumulative reversal is `round_half_up(commission x refunded / paid)` and a full refund
  returns exactly the commission. A refund recorded before the credit gets its reversals from the credit path.
- The payout holdback holds a credit net of its commission and reversals (MODIFIES `seller-payout-holdback`).
- New migration `0018` extends the ledger type/sign CHECK with `COMMISSION` (< 0) and `COMMISSION_REVERSAL` (> 0)
  (`SUBSCRIPTION_FEE` is added by `payment-seller-fee-charge`; both slices edit the same CHECK and are serialised by
  initiative-build) and adds a nullable `commission_rate_bps` column on the ledger.
- New config `COMMISSION_RATE_BPS` (integer basis points, default 500, valid 0..3000).
- New `PaymentService.GetOrderSettlement` (read-only) in `team-payment`, forwarded by `team-gateway`.
- Consumer copies of `payment.proto` and `order.proto` in `team-payment` and `team-gateway` are synced to the
  contract from `monetization-contract`.

Not changed: COD orders (never settle into the wallet, no commission, accepted gap); no backfill of orders paid
before deploy; `order_facts` is not touched; no per-category rate; `AD_SPEND` is not written.

## NFR
- Commission adds at most 5 ms p95 to `CreditSettlement` (same transaction, one extra insert), measured locally.
- Ledger reconciles: balance = sum of entries, zero drift, after any order of paid, refund and cancel events and any
  redelivery.
- `GetOrderSettlement` p95 at most 50 ms locally (three indexed reads).
- Money is int64 integer VND, rates integer basis points, no floating point in any amount computation.
- Availability: unchanged; the consumer stays at-least-once and idempotent, poison records go to the existing DLQ.

## Data classification
| Field | Purpose | Consent basis | Retention |
|---|---|---|---|
| `wallet_ledger.seller_id` on `COMMISSION` / `COMMISSION_REVERSAL` rows | bookkeeping of the platform fee against the seller (existing wallet purpose) | contract performance (seller terms), same as existing ledger rows | follows existing ledger retention, unchanged |
| `wallet_ledger.commission_rate_bps` | audit of the rate applied at settlement | none (no personal data) | same as the ledger row |
| `GetOrderSettlement` response (amounts, order id) | show the seller gross / fee / net | contract performance | not stored, computed per call |
| Breakdown fields on `OrderPaidEvent` | compute the commission | none (order amounts, no party identifier beyond the existing seller id) | event retention, unchanged |

Raw seller or buyer ids are never logged (sloglint forbidden keys); logs carry order id and counts only.

## Threat model
- Assets: seller wallet integrity (money), the commission the platform is owed, seller privacy of per-order
  financials.
- Abuse cases: a redelivered or replayed `OrderPaidEvent` double-charging commission; concurrent refunds and credit
  racing so reversals exceed the commission; a client-forged breakdown (the event is internal on `order.events`,
  never client input); a seller reading another seller's settlement by guessing an order id; a misconfigured rate
  (negative or above 30%) silently skewing money; rounding drift across many partial refunds.
- Authz rule: `GetOrderSettlement` is allowed only to the seller the order's `ORDER_SETTLEMENT` credit belongs to,
  or an admin; anyone else, and an unsettled order, gets `NOT_FOUND` (no existence leak). Commission and reversal rows
  are written only by the settlement consumer and the refund path, never by an RPC argument.
