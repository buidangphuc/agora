## Purpose

Defines how a refund comes out of the seller's wallet: exactly once per refunded payment, only for money the seller
was actually credited, never blocked by the seller's balance or hold, and applied automatically when a paid order
is cancelled.

## ADDED Requirements

### Requirement: A refund deducts the refunded amount from the credited seller exactly once

A successful `RefundPayment` SHALL move the payment from `PAID` to `REFUNDED` by compare-and-set and, for a payment
whose seller was credited, SHALL result in exactly one `REFUND_DEDUCTION` ledger entry of minus the refunded amount
for that seller, referencing the payment. A refund of a payment that is no longer `PAID` SHALL fail with
`FAILED_PRECONDITION` and write nothing; of concurrent refunds of one payment exactly one SHALL succeed.

#### Scenario: Refunding a credited payment deducts the refunded amount from the seller

- **WHEN** a seller was credited 500000 for a paid order and the seller refunds 200000 of that payment through the
  gateway
- **THEN** the payment reads `REFUNDED`, the seller's ledger has exactly one `REFUND_DEDUCTION` entry of -200000, and
  `GetWalletBalance` is 300000

#### Scenario: A second refund of the same payment is refused and deducts nothing more

- **WHEN** the seller refunds the same, already refunded payment again
- **THEN** the call fails with `FAILED_PRECONDITION` and the seller still has exactly one `REFUND_DEDUCTION` entry and
  the same balance

#### Scenario: Concurrent refunds of one payment deduct once

- **WHEN** the seller sends eight concurrent refunds of 100000 for one credited payment
- **THEN** exactly one succeeds, the others fail with `FAILED_PRECONDITION`, and the seller has exactly one
  `REFUND_DEDUCTION` entry of -100000

### Requirement: A refund deducts only money that was credited, in either arrival order

The deduction SHALL follow the credit: a refund of a payment whose order never became `Paid` SHALL NOT deduct the
seller, and a refund that is processed before the payment's credit is written SHALL still end with exactly one credit
and one deduction once the credit is written.

#### Scenario: Refunding a payment whose order was cancelled deducts nothing

- **WHEN** a payment succeeded late for an order that stayed `Cancelled`, and the order's seller refunds it in full
- **THEN** the refund succeeds and the payment reads `REFUNDED`, and after a sentinel order of the same seller is
  credited the seller's ledger has no entry for the cancelled order and the balance equals the sentinel's amount

#### Scenario: A refund issued right after payment ends with one credit and one deduction

- **WHEN** a buyer pays an order of 500000 and its seller refunds 500000 immediately after the payment call returns,
  before the credit is observed
- **THEN** once the order's credit is written the seller has exactly one `ORDER_SETTLEMENT` entry of +500000 and one
  `REFUND_DEDUCTION` entry of -500000 for it, and the balance contribution of that order is 0

### Requirement: Refunds are never blocked by the seller's balance or hold

A refund SHALL be applied even when the deduction takes the seller's balance below zero (the proceeds were already
paid out) or when the proceeds are inside the hold window.

#### Scenario: A refund after the proceeds were paid out takes the balance negative

- **WHEN** a seller's 500000 credit has passed the hold window and been paid out in full, and the seller then refunds
  200000 of that payment
- **THEN** the refund succeeds, the seller's ledger has one `REFUND_DEDUCTION` of -200000, and `GetWalletBalance` is
  -200000

### Requirement: Cancelling a paid order refunds the buyer and deducts the seller exactly once

When an order is cancelled from `Paid`, `team-payment` SHALL refund the order's payment in full without any further
call by the buyer, seller or an admin: the payment SHALL move from `PAID` to `REFUNDED` and, for a credited payment,
exactly one `REFUND_DEDUCTION` of minus the payment amount SHALL be written for the seller. The refund SHALL be
driven by a cancellation fact that `team-order` records in the same transaction as the cancel, so it is applied even
if `team-payment` is unavailable at cancel time. The cancel and the payment's credit SHALL give the same result in
either arrival order; a redelivered cancellation SHALL refund and deduct nothing more; a payment already refunded
before the cancel SHALL be left as it is, with no second deduction; an order cancelled from `Pending` SHALL trigger no
refund.

#### Scenario: Cancelling a credited paid order refunds the buyer and deducts the seller

- **WHEN** a buyer pays an order of 500000, its seller's credit of +500000 is observed, and the buyer cancels the order
  through the gateway
- **THEN** within the settle window the buyer's `GetPayment` reads `REFUNDED`, the seller's ledger has exactly one
  `REFUND_DEDUCTION` of -500000 for it, and the seller's balance is back to its value before the payment

#### Scenario: Cancelling a paid order before its credit is observed ends with one credit and one deduction

- **WHEN** a buyer pays an order of 500000 and cancels it as soon as the order reads `Paid`, before the seller's credit
  is observed
- **THEN** once both are applied the payment reads `REFUNDED` and the seller has exactly one `ORDER_SETTLEMENT` of
  +500000 and one `REFUND_DEDUCTION` of -500000 for the order

#### Scenario: A cancel while team-payment is stopped is refunded when it restarts

- **WHEN** a credited paid order is cancelled by its buyer while `team-payment` is stopped, and `team-payment` is then
  started again
- **THEN** the buyer's payment reads `REFUNDED` and the seller has exactly one `REFUND_DEDUCTION` of minus the
  payment amount

#### Scenario: A redelivered cancellation does not refund or deduct again

- **WHEN** a cancelled paid order has been refunded and deducted, and its cancellation record is produced to
  `order.events` again, byte for byte
- **THEN** after a sentinel order of the same seller is credited the seller still has exactly one `REFUND_DEDUCTION`
  for the cancelled order

#### Scenario: Cancelling an order its seller already refunded deducts nothing more

- **WHEN** the seller of a credited paid order of 500000 refunds 200000, and the buyer then cancels the order
- **THEN** the payment stays `REFUNDED` and, after a sentinel order of the same seller is credited, the seller has
  exactly one `REFUND_DEDUCTION` of -200000 for the order
