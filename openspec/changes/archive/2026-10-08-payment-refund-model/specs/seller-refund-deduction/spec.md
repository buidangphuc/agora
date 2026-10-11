## MODIFIED Requirements

### Requirement: A refund deducts the refunded amount from the credited seller exactly once

Each refund of a payment SHALL be identified by its refund id (see `payment-cumulative-refunds`). For a payment whose
seller was credited, each applied refund with a positive amount SHALL result in exactly one `REFUND_DEDUCTION` ledger
entry for that seller, of minus that refund's applied amount, referencing that refund's id rather than the payment.
Retrying a refund with the same refund id SHALL write no further deduction. A refund the payment refuses (the
payment is `REFUNDED`, or the amount exceeds the refundable remainder) SHALL write no deduction. Concurrent refunds
of one payment SHALL each write exactly one deduction when they succeed. Their deductions together SHALL never
exceed the payment amount.

#### Scenario: Refunding a credited payment deducts the refunded amount from the seller

- **WHEN** a seller was credited 500000 for a paid order and the seller refunds 200000 of that payment through the
  gateway with a fresh refund id
- **THEN** the payment reads `PARTIALLY_REFUNDED` with a refunded amount of 200000
- **AND** the seller's ledger has exactly one `REFUND_DEDUCTION` of -200000, whose reference is that refund's id
- **AND** `GetWalletBalance` is 300000

#### Scenario: A second refund of the same payment deducts its own amount

- **WHEN** the seller then refunds a further 300000 of the same payment with another fresh refund id
- **THEN** the payment reads `REFUNDED` with a refunded amount of 500000
- **AND** the seller's ledger has two `REFUND_DEDUCTION` entries, -200000 and -300000, each referencing its own
  refund
- **AND** `GetWalletBalance` is 0

#### Scenario: A second refund of the same payment is refused and deducts nothing more

- **WHEN** a credited payment of 500000 has been refunded in full by one refund, and the seller refunds it again with a
  fresh refund id
- **THEN** the call fails with `FAILED_PRECONDITION`
- **AND** the seller still has exactly one `REFUND_DEDUCTION` for the payment, of -500000, and the same balance

#### Scenario: Concurrent refunds of one payment deduct once

- **WHEN** the seller of a credited payment of 500000 sends eight concurrent refunds of 100000, all with the same
  refund id
- **THEN** every call succeeds
- **AND** the payment reads `PARTIALLY_REFUNDED` with a refunded amount of 100000
- **AND** the seller has exactly one `REFUND_DEDUCTION` of -100000

### Requirement: A refund deducts only money that was credited, in either arrival order

The deduction SHALL follow the credit. A refund of a payment whose order never became `Paid` SHALL NOT deduct the
seller. Refunds processed before the payment's credit is written SHALL still end with exactly one credit, and with one
deduction per refund that had a positive applied amount, once the credit is written.

#### Scenario: Refunding a payment whose order was cancelled deducts nothing

- **WHEN** a payment succeeded late for an order that stayed `Cancelled`, and the order's seller refunds it in full
- **THEN** the refund succeeds and the payment reads `REFUNDED`
- **AND** after a sentinel order of the same seller is credited, the seller's ledger has no entry for the cancelled
  order and the balance equals the sentinel's amount

#### Scenario: A refund issued right after payment ends with one credit and one deduction

- **WHEN** a buyer pays an order of 500000 and its seller refunds 500000 immediately after the payment call returns,
  before the credit is observed
- **THEN** once the order's credit is written, the seller has exactly one `ORDER_SETTLEMENT` entry of +500000 and one
  `REFUND_DEDUCTION` entry of -500000 for it
- **AND** the balance contribution of that order is 0

#### Scenario: Two refunds issued before the credit end with one deduction each

- **WHEN** a buyer pays an order of 500000 and its seller refunds 100000 and then 150000 of it, with two refund ids,
  immediately after the payment call returns and before the credit is observed
- **THEN** once the order's credit is written, the payment reads `PARTIALLY_REFUNDED` with a refunded amount of
  250000
- **AND** the seller has exactly one `ORDER_SETTLEMENT` of +500000 and two `REFUND_DEDUCTION` entries, -100000 and
  -150000, each referencing its own refund

### Requirement: Cancelling a paid order refunds the buyer and deducts the seller exactly once

When an order is cancelled from `Paid`, `team-payment` SHALL refund whatever is still refundable on the order's
payment (the payment amount minus the cumulative refunded amount), without any further call by the buyer, seller or
an admin. This is one refund with the refund id `cancel:<order_id>`, after which the payment reads `REFUNDED`. For a
credited payment with a positive remainder, it SHALL write exactly one `REFUND_DEDUCTION` of minus that remainder for
the seller, referencing the cancel refund.

The refund SHALL be driven by a cancellation fact that `team-order` records in the same transaction as the cancel, so
it is applied even if `team-payment` is unavailable at cancel time. The cancel and the payment's credit SHALL give the
same result in either arrival order. A redelivered cancellation SHALL refund and deduct nothing more. A payment that
is already `REFUNDED` before the cancel SHALL be left as it is, with no further refund or deduction. An order
cancelled from `Pending` SHALL trigger no refund.

#### Scenario: Cancelling a credited paid order refunds the buyer and deducts the seller

- **WHEN** a buyer pays an order of 500000, its seller's credit of +500000 is observed, and the buyer cancels the order
  through the gateway
- **THEN** within the settle window the buyer's `GetPayment` reads `REFUNDED` with a refunded amount of 500000
- **AND** the seller's ledger has exactly one `REFUND_DEDUCTION` of -500000 for it, referencing `cancel:<order_id>`
- **AND** the seller's balance is back to its value before the payment

#### Scenario: Cancelling a paid order before its credit is observed ends with one credit and one deduction

- **WHEN** a buyer pays an order of 500000 and cancels it as soon as the order reads `Paid`, before the seller's credit
  is observed
- **THEN** once both are applied the payment reads `REFUNDED`
- **AND** the seller has exactly one `ORDER_SETTLEMENT` of +500000 and one `REFUND_DEDUCTION` of -500000 for the order

#### Scenario: A cancel while team-payment is stopped is refunded when it restarts

- **WHEN** a credited paid order is cancelled by its buyer while `team-payment` is stopped, and `team-payment` is then
  started again
- **THEN** the buyer's payment reads `REFUNDED`
- **AND** the seller has exactly one `REFUND_DEDUCTION` of minus the payment amount

#### Scenario: A redelivered cancellation does not refund or deduct again

- **WHEN** a cancelled paid order has been refunded and deducted, and its cancellation record is produced to
  `order.events` again, byte for byte
- **THEN** after a sentinel order of the same seller is credited, the seller still has exactly one `REFUND_DEDUCTION`
  for the cancelled order
- **AND** the payment lists exactly one refund with source `ORDER_CANCEL`

#### Scenario: Cancelling a partially refunded order refunds the remainder

- **WHEN** the seller of a credited paid order of 500000 refunds 200000, and the buyer then cancels the order
- **THEN** within the settle window the payment reads `REFUNDED` with a refunded amount of 500000
- **AND** the payment lists two refunds, 200000 from the seller and 300000 with source `ORDER_CANCEL`
- **AND** the seller has exactly two `REFUND_DEDUCTION` entries for the order, -200000 and -300000

#### Scenario: Cancelling an order its seller already refunded deducts nothing more

- **WHEN** the seller of a credited paid order of 500000 refunds 500000, and the buyer then cancels the order
- **THEN** the payment stays `REFUNDED` with one refund
- **AND** after a sentinel order of the same seller is credited, the seller has exactly one `REFUND_DEDUCTION` of
  -500000 for the order
