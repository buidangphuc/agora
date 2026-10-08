## Purpose

Defines how one payment is refunded in several parts. Each refund is identified by its own id and applied at most
once. The cumulative refunded amount never exceeds the payment amount. The payment shows the buyer, the seller and
admins what was refunded and by which refund.

## ADDED Requirements

### Requirement: A payment can be refunded several times up to its amount

`RefundPayment` SHALL accept a refund of a payment whose status is `PAID` or `PARTIALLY_REFUNDED` when the requested
amount is positive and at most the refundable remainder (the payment amount minus its cumulative refunded amount).
After an accepted refund, the payment's refunded amount SHALL grow by the refund's amount. Its status SHALL be
`PARTIALLY_REFUNDED` while the refunded amount is below the payment amount and `REFUNDED` once they are equal.

A refund of a `REFUNDED` payment, or one above the remainder, SHALL fail with `FAILED_PRECONDITION` and write nothing.
The message for an amount above the remainder SHALL be `refund amount exceeds the refundable remainder`. The
refunded amount SHALL never exceed the payment amount, including under concurrent refunds by any path (seller or
admin calls, returns, cancellation).

#### Scenario: Two partial refunds leave the payment partially refunded and then refunded

- **WHEN** the seller of a paid order of 500000 refunds 200000 and then 300000 through the gateway, each with its own
  refund id
- **THEN** after the first refund the payment reads `PARTIALLY_REFUNDED` with a refunded amount of 200000
- **AND** after the second it reads `REFUNDED` with a refunded amount of 500000

#### Scenario: A refund above the remainder is refused and writes nothing

- **WHEN** 200000 of a paid order of 500000 has been refunded and the seller refunds 300001 with a fresh refund id
- **THEN** the call fails with `FAILED_PRECONDITION` and message `refund amount exceeds the refundable remainder`
- **AND** the payment still reads `PARTIALLY_REFUNDED` with a refunded amount of 200000 and one refund

#### Scenario: Two partial refunds racing near the cap cannot over-refund

- **WHEN** 300000 of a credited payment of 500000 has been refunded and the seller sends two concurrent refunds of
  150000 with different refund ids
- **THEN** exactly one succeeds and the other fails with `FAILED_PRECONDITION`
- **AND** the payment reads `PARTIALLY_REFUNDED` with a refunded amount of 450000
- **AND** the seller has exactly two `REFUND_DEDUCTION` entries for the payment, -300000 and -150000

#### Scenario: Concurrent refunds with their own ids refund at most the payment amount

- **WHEN** the seller of a credited payment of 500000 sends eight concurrent refunds of 125000, each with its own
  refund id
- **THEN** exactly four succeed and the other four fail with `FAILED_PRECONDITION`
- **AND** the payment reads `REFUNDED` with a refunded amount of 500000 and four refunds
- **AND** the seller has exactly four `REFUND_DEDUCTION` entries of -125000, each referencing one of the successful
  refunds

### Requirement: A refunded payment can never be paid again

A payment that is `PARTIALLY_REFUNDED` or `REFUNDED` SHALL NOT be settled again by `ProcessMockPayment`, whether
the simulated outcome is success or failure. The call SHALL fail with `FAILED_PRECONDITION` and the message
`payment has been refunded; it cannot be paid again`, and the payment's status, refunded amount and refunds SHALL be
unchanged. (Re-settling would reopen it to further refunds of money already returned.)

#### Scenario: A refunded payment cannot be paid again

- **WHEN** a credited 500000 payment has been partly refunded by 200000 and its buyer calls the mock payment again
  for the same order through the gateway
- **THEN** the call fails with `FAILED_PRECONDITION` and the message `payment has been refunded; it cannot be paid
  again`
- **AND** the payment still reads `PARTIALLY_REFUNDED` with a refunded amount of 200000

### Requirement: Each refund is identified by its refund id and applied once

`RefundPayment` SHALL require a `refund_id` of 1 to 64 characters drawn from letters, digits, `.`, `_`, `:` and `-`. A
missing or invalid id SHALL fail with `INVALID_ARGUMENT` and write nothing.

The refund id SHALL be the refund's idempotency key. A call that repeats a refund id already applied to the same
payment with the same amount SHALL succeed, return the payment's current state and write nothing more. A refund id
already used for a different payment or a different amount SHALL fail with `ALREADY_EXISTS` and write nothing.
Concurrent calls with the same refund id SHALL apply the refund once.

Refunds that `team-payment` applies on its own SHALL use ids derived from their source, so a redelivered fact maps to
the same refund:

- `return:<return_id>` for a return.
- `cancel:<order_id>` for a cancellation.

#### Scenario: Retrying a refund with the same refund id refunds once

- **WHEN** the seller of a credited paid order of 500000 refunds 200000 with refund id R and then sends the same call
  again
- **THEN** both calls succeed
- **AND** the payment reads `PARTIALLY_REFUNDED` with a refunded amount of 200000 and one refund
- **AND** the seller has exactly one `REFUND_DEDUCTION` of -200000

#### Scenario: Reusing a refund id for a different amount is refused

- **WHEN** a refund of 200000 with refund id R was applied and the seller refunds 100000 of the same payment with
  refund id R
- **THEN** the call fails with `ALREADY_EXISTS`
- **AND** the payment's refunded amount stays 200000

#### Scenario: A refund without a refund id is refused

- **WHEN** the seller of a paid order refunds 100000 without a refund id
- **THEN** the call fails with `INVALID_ARGUMENT`
- **AND** the payment reads `PAID` with no refund

### Requirement: The payment reports what was refunded and by which refund

Every `PaymentTransaction` returned by `GetPayment` and `RefundPayment` SHALL carry its cumulative `refunded_amount`
and its refunds, oldest first. Each refund SHALL carry:

- its id;
- its source (`SELLER_OR_ADMIN`, `RETURN`, `ORDER_CANCEL` or `LEGACY`);
- its source id: the refund id given by the caller, the return id, the order id, or the payment id;
- the requested amount and the applied amount;
- its reason;
- its creation time.

`ListLedgerEntries` SHALL carry each entry's reference: the payment id for an `ORDER_SETTLEMENT`, the refund id for a
`REFUND_DEDUCTION`, and empty for a payout.

#### Scenario: A partially refunded payment lists its refunds through the gateway

- **WHEN** the seller of a paid order of 500000 refunds 200000 with refund id R1 and reason `damaged`, and 100000
  with refund id R2
- **THEN** the buyer's `GetPayment` through the gateway reads `PARTIALLY_REFUNDED` with a refunded amount of 300000
- **AND** it lists two refunds in that order, each with source `SELLER_OR_ADMIN`, source ids R1 and R2, and requested
  and applied amounts of 200000 and 100000
- **AND** the first refund has reason `damaged`

#### Scenario: The seller's ledger entries name the refund each deduction belongs to

- **WHEN** a credited payment of 500000 has two refunds, R1 of 200000 and R2 of 100000
- **THEN** the seller's `ListLedgerEntries` shows the `ORDER_SETTLEMENT` referencing the payment id
- **AND** it shows two `REFUND_DEDUCTION` entries whose references are the two refunds' ids

### Requirement: The order's seller can read the order's payment

`GetPayment` SHALL return the payment to three callers:

- the order's buyer;
- the order's seller, as reported by `team-order`;
- a principal with the `admin` scope.

Every other caller SHALL get `PERMISSION_DENIED`.

#### Scenario: The order's seller reads the payment of their order

- **WHEN** the seller of a paid order calls `GetPayment` with the order id through the gateway
- **THEN** the call returns the order's payment with its status, refunded amount and refunds

#### Scenario: Another seller cannot read the payment of an order

- **WHEN** a seller who does not sell the order calls `GetPayment` with that order's id through the gateway
- **THEN** the call fails with `PERMISSION_DENIED`

### Requirement: Payments refunded under the single-refund model keep their outcome after the upgrade

The `team-payment` migration that introduces cumulative refunds SHALL convert each payment that is `REFUNDED` with a
positive refunded amount:

- it gains exactly one refund with source `LEGACY` and id `legacy:<payment id>`, whose requested and applied amounts
  are the refunded amount;
- that payment's `REFUND_DEDUCTION`, if any, now references that refund instead of the payment;
- its status and refunded amount stay as they were. A legacy partial refund therefore keeps the payment `REFUNDED`
  and closed to further refunds and to the cancel remainder.

A `REFUNDED` payment with a refunded amount of 0 (refunded before refunded amounts were recorded) SHALL be left
untouched. The migration SHALL NOT change any seller's ledger sum. Its down migration SHALL restore the payment-id
references.

#### Scenario: A legacy partial refund becomes one legacy refund and stays closed

- **WHEN** a scratch payment database at the previous schema holds a payment of 500000 refunded under the old model
  (status `REFUNDED`, refunded amount 200000, a credit of +500000 and a `REFUND_DEDUCTION` of -200000 referencing the
  payment), and the migration is applied with the `team-payment` migrate image
- **THEN** the database holds one `LEGACY` refund `legacy:<payment id>` of 200000 for that payment
- **AND** the deduction references `legacy:<payment id>`
- **AND** the payment is still `REFUNDED` with a refunded amount of 200000, and the seller's ledger sum is still
  300000

#### Scenario: A legacy partially refunded payment refuses a further refund

- **WHEN** the `team-payment` image runs against that migrated scratch database and an admin refunds 100000 of the
  legacy payment with a fresh refund id
- **THEN** the call fails with `FAILED_PRECONDITION`
- **AND** the payment still has one refund

#### Scenario: A payment refunded before refunded amounts were recorded is left as it was

- **WHEN** the scratch database also holds a `REFUNDED` payment with a refunded amount of 0 and no deduction, and the
  migration is applied
- **THEN** that payment has no refund, keeps status `REFUNDED` and a refunded amount of 0
- **AND** no ledger row changed

#### Scenario: Rolling back the migration restores the payment-id references

- **WHEN** the down migration is applied to the migrated scratch database
- **THEN** the legacy deduction references the payment id again
- **AND** the seller's ledger sum is still 300000
