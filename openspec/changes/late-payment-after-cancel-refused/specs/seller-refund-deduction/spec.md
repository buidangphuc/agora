## MODIFIED Requirements

### Requirement: A refund deducts only money that was credited, in either arrival order

The deduction SHALL follow the credit. A refund of a payment whose order never became `Paid` SHALL NOT deduct the
seller. Refunds processed before the payment's credit is written SHALL still end with exactly one credit, and with one
deduction per refund that had a positive applied amount, once the credit is written.

#### Scenario: Refunding a payment whose order was cancelled deducts nothing

- **WHEN** a buyer's payment for an order was refused because the order was `Cancelled`, and the order's seller refunds
  it in full
- **THEN** the refund fails with `failed_precondition` and the payment reads `CANCELLED`
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
