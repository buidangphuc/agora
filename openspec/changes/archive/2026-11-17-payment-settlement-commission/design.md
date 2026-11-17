# Design: payment-settlement-commission

## D1. Where it runs
`Handler.handlePaid` passes the breakdown to `Service.CreditSettlement`, which passes a `Commissionable` value
(`platformDiscount`, `base`, `ok`) to `SettlementLedger.CreditSettlement`. The store inserts, in the existing
transaction after the payment-row lock and the seller lock: the `ORDER_SETTLEMENT` credit, then the `COMMISSION`
entry, then the per-refund deductions and reversals. Lock order is unchanged (payment row, then seller lock).

## D2. Breakdown present
The breakdown is present when all four optional fields have presence on the event. Absent: credit = paid amount
(old behaviour), no commission, metric `payment_commission_skipped_total{reason="no_breakdown"}` incremented. Present
but inconsistent (any value negative, or `shop_discount_amount > items_subtotal`): permanent error, record parked on
the existing DLQ, nothing written (the order slice guarantees consistency, so this is a defect signal).

## D3. Amounts
- `commission = (base x rate_bps + 5000) / 10000` in integer division (floor of half-up), `base` clamped at 0.
  A commission of 0 (rate 0, or base too small) writes no row, because the CHECK requires a negative amount.
- `credit = paid + platform_discount_amount` where `paid` is the settled payment transaction amount (the
  transaction wins over `total_amount`, as today).
- Reversal target after a refund: `round_half_up(commission x refunded / paid)` using the payment's cumulative
  `refunded_amount` after this refund, computed as `(2 x commission x refunded + paid) / (2 x paid)` in 128-bit
  safe arithmetic (`math/big` or a mulDiv helper), never floating point. Reversal = target minus the sum of
  reversals already booked for the payment (joined through `payment_refunds`). Reversal 0 writes no row.
- A full refund gives target = commission exactly, so the sum of reversals equals the commission and never exceeds it.

## D4. Idempotency
`COMMISSION` is keyed `(COMMISSION, payment id)`, `COMMISSION_REVERSAL` is keyed `(COMMISSION_REVERSAL, refund key)`,
both through the existing unique index `uq_wallet_ledger_type_reference` and `insertLedgerOnce`. A refund replay
returns before the reversal step (existing `ReplayOf` path). A refund of a payment with no `ORDER_SETTLEMENT` credit
writes no reversal; the credit path computes reversals from the refund list in order with a running cumulative sum.
A payment credited without commission (old event) has no `COMMISSION` row, so no reversal is ever written.

## D5. Holdback
`heldCreditsSQL` nets, per in-window credit, the `COMMISSION` entry (reference = payment id) and the
`COMMISSION_REVERSAL` entries (reference = refund id joined to the payment) in addition to the deductions, never
below 0.

## D6. GetOrderSettlement
Resolve the order's settled transaction (`SettledTransaction`), then the `ORDER_SETTLEMENT` credit; none -> NOT_FOUND.
Authorise on the credit's seller (principal id equals it, or admin); mismatch -> NOT_FOUND. Return `credited`,
`commission` (abs of the COMMISSION row or 0), `commission_reversed` (sum of reversals), `refund_deducted` (abs of
the sum of deductions of the payment's refunds), `net = credited - commission + commission_reversed - refund_deducted`,
and `commission_rate_bps` read from the `COMMISSION` row (0 when none). The gateway forwards it with a read call.

## D7. Rate config
`Commission{RateBps int `env:"COMMISSION_RATE_BPS" default:"500"`}` validated 0..3000 at startup like `Payout`;
invalid refuses to start naming the key. Documented in README and `.env.example`. The rate is stored on the
`COMMISSION` row, so a later rate change never alters past orders or their reversals.

## D8. Platform voucher and refunds
Refund deductions stay equal to the refunded buyer amount. The platform-funded voucher part of the credit is not
clawed back on refund (assumption; see open issues of the initiative review).
