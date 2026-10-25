## MODIFIED Requirements

### Requirement: Only an order that became paid credits its seller

A payment for an order `team-order` did not move to `Paid` SHALL NOT credit the seller. A payment attempted after the
order was cancelled is refused by team-payment (see `unpaid-order-payment`) and reads `CANCELLED`.

#### Scenario: A late payment for a cancelled order does not credit the seller

- **WHEN** a buyer opens a payment for a `Pending` order, cancels the order, and then pays it with the mock payment
- **THEN** the payment call fails with `failed_precondition`, the order stays `Cancelled`, the payment reads
  `CANCELLED`, and after a sentinel order of the same seller is credited the seller's ledger has no entry for the
  cancelled order's amount and the balance equals the sentinel's amount
