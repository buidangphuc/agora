# late-payment-after-cancel-refused

initiative: adhoc

## Why
`payment-unpaid-cancel` (archived into `unpaid-order-payment`) made `ProcessMockPayment` refuse a payment for an order
that is already `Cancelled`: the call returns `FAILED_PRECONDITION` "payment cancelled" and the transaction is voided
to `CANCELLED`. Three older specs still describe the opposite: the late payment succeeds (`PAID`) and is then ignored
by team-order, not credited, or refunded with no deduction. The code and the e2e scenarios already follow
`unpaid-order-payment` (PR #44); only the three requirement texts contradict it. The newest approved requirement wins,
so the older specs are aligned to it.

## What changes
- `order-lifecycle-guards`: the late-payment scenarios say the payment is refused as cancelled and the order stays
  `Cancelled`; the voucher of a cancelled order stays unused. The consumer rule (only `Pending` → `Paid`) is unchanged
  and still covers a payment that wins a race with the cancel.
- `seller-settlement-credit`: a refused late payment reads `CANCELLED` and credits nothing.
- `seller-refund-deduction`: a refund of a cancelled order's payment is refused with `FAILED_PRECONDITION`, the payment
  stays `CANCELLED` and the seller's ledger has no entry for it.
- No code change; the e2e scenarios of the same names already assert this.

## NFR
- No runtime change.

## Data classification
No personal data is added, stored, logged or sent.

## Threat model
- Asset: buyer money and seller ledger.
- Abuse case: a buyer pays an order after cancelling it to get stock or a voucher without a valid order. The payment is
  refused before settlement, so no money moves, no credit is written and the voucher stays unused.
