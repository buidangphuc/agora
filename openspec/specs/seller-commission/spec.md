# seller-commission Specification

## Purpose
Defines how team-payment books a commission against the seller when an order settles, returns it proportionally on refunds, keeps commission ledger rows unique and sign-consistent, and lets a seller read gross, commission and net of a settled order.

## Requirements

### Requirement: Settlement books a commission against the seller

When `team-payment` settles an order from an `OrderPaidEvent` that carries the price breakdown, it SHALL append, in the
same database transaction as the `ORDER_SETTLEMENT` credit, exactly one `COMMISSION` ledger entry with status
`COMPLETED` for the credited seller, referencing the payment transaction, of the amount
`-floor((base x rate_bps + 5000) / 10000)` in integer VND, where `base = items_subtotal - shop_discount_amount` and
`rate_bps` is the configured rate. Shipping and `platform_discount_amount` SHALL NOT be part of the base. The commission
SHALL be computed once per order on the full base. No `COMMISSION` entry SHALL be written when the amount is 0 or when
the event carries no price breakdown; the latter SHALL increment the metric
`payment_commission_skipped_total{reason="no_breakdown"}`. A redelivered or replayed event SHALL NOT write a second
commission. An event whose breakdown is inconsistent (a negative value, or `shop_discount_amount` above
`items_subtotal`) SHALL be parked on the dead-letter topic with nothing written. The commission entry SHALL record the
rate applied.

#### Scenario: A paid order books 5% on subtotal minus shop voucher

- **WHEN** an order with items_subtotal 300000, shipping_fee 30000, shop_discount_amount 20000 and
  platform_discount_amount 0 (payment 310000) is settled at the default rate of 500 bps
- **THEN** the seller's ledger has one `ORDER_SETTLEMENT` of +310000 and one `COMMISSION` of -14000 referencing the
  same payment, written in one transaction
- **AND** the seller's balance grew by 296000
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A paid order books 5% on subtotal minus shop voucher`

#### Scenario: The platform voucher and shipping are outside the base

- **WHEN** an order with items_subtotal 300000, shipping_fee 30000, shop_discount_amount 0 and
  platform_discount_amount 10000 is settled at 500 bps
- **THEN** the `COMMISSION` entry is -15000
- **AND** the `ORDER_SETTLEMENT` entry is +330000
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::The platform voucher and shipping are outside the base`

#### Scenario: Commission rounds half up to whole VND

- **WHEN** an order with items_subtotal 100010 and shop_discount_amount 0 is settled at 500 bps, and another with
  items_subtotal 100001 at 500 bps
- **THEN** the first books a `COMMISSION` of -5001 and the second one of -5000
- **VERIFIED BY**: `team-payment/internal/store/commission_math_test.go` `TestCommissionAmount_RoundsHalfUpToWholeVND`

#### Scenario: A redelivered paid event books no second commission

- **WHEN** the `OrderPaidEvent` of an already settled order is produced to `order.events` again, byte for byte
- **THEN** after a sentinel order of the same seller is settled, the seller has exactly one `COMMISSION` entry for the
  replayed order
- **AND** the balance equals the sum of the two orders' credits minus their commissions
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A redelivered paid event books no second commission`

#### Scenario: Concurrent settlements of one order book one commission

- **WHEN** the same `OrderPaidEvent` is handled by eight concurrent consumers
- **THEN** the ledger has exactly one `ORDER_SETTLEMENT` and one `COMMISSION` for that payment
- **VERIFIED BY**: `team-payment/internal/store/commission_settlement_test.go` `TestCommissionSettlement_ConcurrentSettlementsBookOneCommission`

#### Scenario: An event without the breakdown books no commission

- **WHEN** an `OrderPaidEvent` with fields 9-12 absent is consumed
- **THEN** the seller is credited the paid amount and the ledger has no `COMMISSION` entry for that payment
- **AND** `payment_commission_skipped_total{reason="no_breakdown"}` increased by 1
- **VERIFIED BY**: `team-payment/internal/settlement/breakdown_test.go` `TestConsumer_EventWithoutTheBreakdownBooksNoCommission`

#### Scenario: A zero commission writes no row

- **WHEN** an order with items_subtotal 5 and shop_discount_amount 0 is settled at 500 bps, or any order at 0 bps
- **THEN** the ledger has no `COMMISSION` entry for it and the settlement credit is written
- **VERIFIED BY**: `team-payment/internal/store/commission_settlement_test.go` `TestCommissionSettlement_ZeroCommissionWritesNoRow`

#### Scenario: An inconsistent breakdown is parked and writes nothing

- **WHEN** an `OrderPaidEvent` with shop_discount_amount 400000 above items_subtotal 300000 is produced to
  `order.events`
- **THEN** the record appears on `order.events.payment-settlement.dlq`
- **AND** the ledger has no entry for that order, and a later paid order of the same seller is settled normally
- **VERIFIED BY**: `team-payment/internal/settlement/breakdown_test.go` `TestConsumer_InconsistentBreakdownIsParkedAndWritesNothing`

#### Scenario: A failed commission insert rolls back the credit

- **WHEN** the `COMMISSION` insert fails inside the settlement transaction
- **THEN** no `ORDER_SETTLEMENT` entry exists for that payment either
- **AND** the record is retried and, once the failure clears, both entries are written once
- **VERIFIED BY**: `team-payment/internal/store/commission_settlement_test.go` `TestCommissionSettlement_FailedCommissionInsertRollsBackTheCredit`

#### Scenario: A client cannot set or forge a commission

- **WHEN** a seller or an admin calls any `PaymentService` method through the gateway
- **THEN** no request message carries a commission amount or rate, and a `COMMISSION` entry exists only for payments
  settled from `order.events`
- **AND** a direct call to `ChargeSellerFee` with a seller token is denied by `team-gateway` as not routed
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A client cannot set or forge a commission`

#### Scenario: Commission adds at most 5 ms p95 to settlement

- **WHEN** 200 settlements with a breakdown and 200 without are run on the same local database in one run, median of
  3 runs
- **THEN** the p95 of `CreditSettlement` with commission exceeds the p95 without it by at most 5 ms
- **VERIFIED BY**: `team-payment/internal/store/commission_perf_test.go` `TestCommissionOverheadAtMostFiveMsP95`

### Requirement: Refunds return commission proportionally

When a refund of a payment that was credited and charged a commission is applied, `team-payment` SHALL append, in the
same database transaction as the refund's `REFUND_DEDUCTION`, one `COMMISSION_REVERSAL` ledger entry (positive, status
`COMPLETED`, for the credited seller) referencing the refund key, of the amount `target - already_reversed`, where
`target = round_half_up(commission x refunded / paid)`, `refunded` is the payment's cumulative refunded amount after
this refund, `paid` is the payment amount, and `already_reversed` is the sum of the payment's earlier reversals. The
computation SHALL use integer arithmetic only. The sum of reversals SHALL never exceed the commission and a full
refund SHALL return exactly the commission. No entry SHALL be written when the amount is 0, when the payment has no
`COMMISSION` entry, or on a replay of the same refund key. Refunds recorded before the credit SHALL get their
reversals from the credit path in refund order, with the same cumulative rule. Cancellation of a paid order (a
refund of the remainder keyed `cancel:<order_id>`) and a return refund SHALL both follow this rule.

#### Scenario: A 40% partial refund returns 40% of the commission

- **WHEN** the order of 310000 with a 14000 commission is refunded 124000 by a return
- **THEN** the ledger has a `REFUND_DEDUCTION` of -124000 and a `COMMISSION_REVERSAL` of +5600 referencing the same
  refund key
- **AND** `GetOrderSettlement` reports `net` 177600
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A 40% partial refund returns 40% of the commission`

#### Scenario: A full refund returns exactly the commission

- **WHEN** the same order is then refunded the remaining 186000
- **THEN** a second `COMMISSION_REVERSAL` of +8400 is booked and the reversals total 14000
- **AND** the seller's balance for that order is 0
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A full refund returns exactly the commission`

#### Scenario: Cumulative rounding never exceeds the commission

- **WHEN** a payment of 310000 with a 14000 commission is refunded in 310 refunds of 1000
- **THEN** the sum of `COMMISSION_REVERSAL` entries is exactly 14000 after the last refund
- **AND** no prefix of the sequence has booked more than `round_half_up(14000 x refunded / 310000)`
- **VERIFIED BY**: `team-payment/internal/store/commission_math_test.go` `TestReversalTarget_CumulativeNeverExceedsCommissionAndEndsExact`

#### Scenario: A tiny refund whose share rounds to zero books no reversal

- **WHEN** the same payment is refunded 1 VND once
- **THEN** a `REFUND_DEDUCTION` of -1 is booked and no `COMMISSION_REVERSAL` entry is written
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A tiny refund whose share rounds to zero books no reversal`

#### Scenario: Cancelling a paid order returns the whole commission

- **WHEN** a credited order with a 14000 commission is cancelled from `Paid` and the `OrderCancelled` event is
  applied twice
- **THEN** exactly one `COMMISSION_REVERSAL` of +14000 exists and the order's net is 0
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::Cancelling a paid order returns the whole commission`

#### Scenario: A refund that landed before the credit still gets its reversal

- **WHEN** a refund of 124000 is recorded for a payment before its `OrderPaidEvent` is consumed, and the event is then
  consumed
- **THEN** the ledger has the credit, the commission, the deduction of -124000 and a reversal of +5600, once each
- **VERIFIED BY**: `team-payment/internal/store/commission_settlement_test.go` `TestReversal_RefundBeforeCreditStillGetsItsReversal`

#### Scenario: A refund by a seller who is not the order's seller is denied and reverses nothing

- **WHEN** another seller calls `RefundPayment` for the payment of a commissioned order
- **THEN** the call fails with `PERMISSION_DENIED` and the ledger has no new `REFUND_DEDUCTION` or
  `COMMISSION_REVERSAL`
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A refund by a seller who is not the order's seller is denied and reverses nothing`

#### Scenario: A replayed refund writes no second reversal

- **WHEN** the `ReturnRefunded` event of an already applied refund is redelivered
- **THEN** the ledger entries and the balance are unchanged
- **VERIFIED BY**: `team-payment/internal/store/commission_settlement_test.go` `TestReversal_ReplayedRefundWritesNoSecondReversal`

#### Scenario: A payment without commission is refunded without a reversal

- **WHEN** a payment credited from an event without the price breakdown is refunded in part
- **THEN** a `REFUND_DEDUCTION` is booked and no `COMMISSION_REVERSAL` entry exists for it
- **VERIFIED BY**: `team-payment/internal/store/commission_settlement_test.go` `TestReversal_PaymentWithoutCommissionIsRefundedWithoutReversal`

#### Scenario: Concurrent refunds never reverse more than the commission

- **WHEN** eight concurrent refunds of 1/8 of a credited payment with a 14000 commission, each with its own refund
  key, are applied
- **THEN** the reversals sum to exactly 14000 and the payment reads `REFUNDED`
- **VERIFIED BY**: `team-payment/internal/store/commission_settlement_test.go` `TestReversal_ConcurrentRefundsNeverReverseMoreThanTheCommission`

#### Scenario: The ledger reconciles after any order of events

- **WHEN** credit, commission, deduction and reversal events for a set of orders are applied in a shuffled order with
  duplicates
- **THEN** each seller's balance equals the sum of their ledger entries and equals, per order, the
  `net` reported by `GetOrderSettlement`
- **VERIFIED BY**: `team-payment/internal/store/commission_reconcile_test.go` `TestReconcile_LedgerReconcilesAfterAnyOrderOfEvents`

### Requirement: The ledger store keeps commission rows unique and sign-consistent

The `team-payment` ledger store SHALL refuse a second `COMMISSION` entry for one payment reference, a second
`COMMISSION_REVERSAL` entry for one refund reference, a `COMMISSION` entry that is not negative, a
`COMMISSION_REVERSAL` entry that is not positive, and any `COMMISSION` or `COMMISSION_REVERSAL` entry without a
reference. Entries of the existing types SHALL keep their rules and every existing row SHALL stay readable.

#### Scenario: The store refuses a second commission for one payment

- **WHEN** a second `COMMISSION` row with the reference of an already charged payment is inserted directly into
  `team-payment`'s database
- **THEN** the insert fails with a unique violation and the seller's balance is unchanged
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::The store refuses a second commission for one payment`

#### Scenario: The store refuses a commission that is not negative

- **WHEN** a `COMMISSION` row of +1 with a fresh reference is inserted directly
- **THEN** the insert fails with a check violation
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::The store refuses a commission that is not negative`

#### Scenario: The store refuses a reversal that is not positive

- **WHEN** a `COMMISSION_REVERSAL` row of -1 with a fresh reference is inserted directly
- **THEN** the insert fails with a check violation
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::The store refuses a reversal that is not positive`

#### Scenario: The store refuses a commission without a reference

- **WHEN** a `COMMISSION` row of -1 without a reference is inserted directly
- **THEN** the insert fails with a check violation
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::The store refuses a commission without a reference`

#### Scenario: The migration keeps existing ledger rows valid

- **WHEN** migration `0018` is applied to a database holding `ORDER_SETTLEMENT`, `REFUND_DEDUCTION` and `PAYOUT`
  rows, then rolled back and applied again
- **THEN** it succeeds each time and the sum of entries per seller is unchanged
- **VERIFIED BY**: `team-payment/internal/store/migration0019_pg_test.go` `TestMigration0019_ExistingLedgerRowsSurviveUpDownUp`

### Requirement: The commission rate is configured and validated at startup

`team-payment` SHALL read the commission rate from `COMMISSION_RATE_BPS` (integer basis points, default 500, valid
0 to 3000, 0 disables commission). Any other value, including a non-integer, SHALL make `team-payment` refuse to start
with an error naming the key. The key SHALL be documented in the README and `.env.example`. A rate change SHALL affect
only orders settled afterwards.

#### Scenario: An invalid rate refuses to start

- **WHEN** the `team-payment` image is started with `COMMISSION_RATE_BPS=-1`, `3001`, `abc` or `5.5`
- **THEN** each container exits non-zero and its log names `COMMISSION_RATE_BPS`
- **VERIFIED BY**: `team-payment/internal/config/config_test.go` `TestCommissionRateInvalidRefusesStart`

#### Scenario: The boundary rates are accepted

- **WHEN** `team-payment` is started with `COMMISSION_RATE_BPS=0` and with `3000`
- **THEN** it starts, and with 3000 an order of items_subtotal 100000 books a `COMMISSION` of -30000
- **VERIFIED BY**: `team-payment/internal/config/config_test.go` `TestCommissionRateBoundariesAccepted`

#### Scenario: A rate change does not alter earlier orders

- **WHEN** an order is settled at 500 bps, `team-payment` is restarted with 1000 bps, and the earlier order is
  refunded in full
- **THEN** the reversal equals the original commission and `GetOrderSettlement` for it reports
  `commission_rate_bps` 500
- **VERIFIED BY**: `team-payment/internal/rpc/get_order_settlement_test.go` `TestGetOrderSettlement_RateChangeDoesNotAlterEarlierOrders`

### Requirement: A seller reads gross, commission and net of a settled order

`PaymentService.GetOrderSettlement` SHALL return, for an order whose `ORDER_SETTLEMENT` credit exists: `credited`,
`commission` (positive VND, 0 when none), `commission_reversed`, `refund_deducted` (positive VND), `net = credited -
commission + commission_reversed - refund_deducted`, and `commission_rate_bps` (the rate recorded on the commission
entry, 0 when none). It SHALL be callable by the seller the credit belongs to and by an admin. It SHALL return
`NOT_FOUND` for any other caller, for an unknown order, and for an order that is not settled (unpaid, cancelled before
payment, or paid but not yet credited). An empty `order_id` SHALL return `INVALID_ARGUMENT`. `team-gateway` SHALL
forward it with the caller's credentials and reject an unauthenticated call with `UNAUTHENTICATED`. The call SHALL
write nothing.

#### Scenario: The seller reads the breakdown of a settled order

- **WHEN** the seller of the order of 310000 with a 14000 commission and no refunds calls `GetOrderSettlement` through
  the gateway
- **THEN** the response is credited 310000, commission 14000, commission_reversed 0, refund_deducted 0, net 296000,
  commission_rate_bps 500
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::The seller reads the breakdown of a settled order`

#### Scenario: The breakdown reflects a partial refund

- **WHEN** the same order has been refunded 124000 and the seller calls it again
- **THEN** commission_reversed is 5600, refund_deducted is 124000 and net is 177600
- **VERIFIED BY**: `team-payment/internal/rpc/get_order_settlement_test.go` `TestGetOrderSettlement_BreakdownReflectsAPartialRefund`

#### Scenario: An order settled without commission reports zero commission

- **WHEN** the seller calls it for an order credited from an event without the breakdown, paid 320000
- **THEN** credited is 320000, commission 0, commission_rate_bps 0 and net 320000
- **VERIFIED BY**: `team-payment/internal/rpc/get_order_settlement_test.go` `TestGetOrderSettlement_OrderSettledWithoutCommissionReportsZeroCommission`

#### Scenario: Another seller gets NOT_FOUND

- **WHEN** a different seller calls it for that order
- **THEN** the call fails with `NOT_FOUND`, the same as for an unknown order id
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::Another seller gets NOT_FOUND`

#### Scenario: A buyer gets NOT_FOUND

- **WHEN** the buyer of the order calls it
- **THEN** the call fails with `NOT_FOUND`
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A buyer gets NOT_FOUND`

#### Scenario: An admin can read any settlement

- **WHEN** an admin calls it for that order
- **THEN** the response equals the seller's
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::An admin can read any settlement`

#### Scenario: An unsettled order is not found

- **WHEN** the seller calls it for an order that is `Pending`, and for one whose payment was voided by cancellation
- **THEN** each call fails with `NOT_FOUND`
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::An unsettled order is not found`

#### Scenario: A missing order id is rejected

- **WHEN** the seller calls it with an empty `order_id`
- **THEN** the call fails with `INVALID_ARGUMENT`
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::A missing order id is rejected`

#### Scenario: An unauthenticated call is refused

- **WHEN** the gateway receives the call without a token
- **THEN** it responds `UNAUTHENTICATED` and `team-payment` receives no request
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::An unauthenticated call is refused`

#### Scenario: Reading the settlement writes nothing

- **WHEN** the seller calls it ten times in a row
- **THEN** the ledger entry count and the balance are unchanged
- **VERIFIED BY**: platform-e2e `payment/psc_commission.feature::Reading the settlement writes nothing`
