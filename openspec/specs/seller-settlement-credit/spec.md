# seller-settlement-credit Specification

## Purpose
Defines how a seller is credited for a paid order: once, durably, only for an order that actually became paid, and
with ledger rows the store itself keeps unique and sign-consistent.

## Requirements

### Requirement: A paid order credits its seller exactly once

When an order becomes `Paid`, `team-payment` SHALL append exactly one `ORDER_SETTLEMENT` ledger entry with status
`COMPLETED` for the order's seller, of the amount of the order's payment transaction, referencing that transaction.
The credit SHALL be driven by `team-order`'s `OrderPaidEvent` on `order.events`, not by the payment RPC, so it
appears asynchronously after the payment succeeds. Paying an already paid order again, or settling one order
concurrently more than once, SHALL NOT add a second credit. `ProcessMockPayment` SHALL NOT write any ledger entry
itself.

#### Scenario: Paying an order credits its seller with the paid amount

- **WHEN** a buyer pays a `Pending` order of 500000 with the mock payment and the order becomes `Paid`
- **THEN** within the settle window the seller's `ListLedgerEntries` through the gateway shows exactly one new
  `ORDER_SETTLEMENT` entry of +500000 with status `COMPLETED`, and `GetWalletBalance` grew by 500000

#### Scenario: Paying an already paid order again does not credit twice

- **WHEN** the buyer calls the mock payment again for the same, already credited order
- **THEN** the call reports the order already paid, and after a later sentinel order of the same seller is credited
  the seller still has exactly one `ORDER_SETTLEMENT` entry for the first order's amount

#### Scenario: Concurrent settlements of one order credit the seller once

- **WHEN** the buyer sends eight concurrent successful mock payments for one `Pending` order
- **THEN** the order is `Paid` and, after a sentinel order of the same seller is credited, the seller has exactly one
  `ORDER_SETTLEMENT` entry for that order

### Requirement: Only an order that became paid credits its seller

A payment that succeeds for an order `team-order` did not move to `Paid` (for example a late payment after the order
was cancelled) SHALL NOT credit the seller, even though the payment transaction is `PAID`.

#### Scenario: A late payment for a cancelled order does not credit the seller

- **WHEN** a buyer opens a payment for a `Pending` order, cancels the order, and the mock payment then succeeds
- **THEN** the order stays `Cancelled`, the payment reads `PAID`, and after a sentinel order of the same seller is
  credited the seller's ledger has no entry for the cancelled order's amount and the balance equals the sentinel's
  amount

### Requirement: Settlement credits survive redelivery, restarts and poison records

The credit consumer SHALL be at-least-once and idempotent: it SHALL commit a record's offset only after the credit
is written or the record is parked on the dead-letter topic `order.events.payment-settlement.dlq`, so a record not yet
committed is processed after a restart, and a redelivered or replayed `OrderPaidEvent` SHALL leave the ledger
unchanged. A record that can never be applied SHALL be parked on the dead-letter topic without blocking later
records. Event types on `order.events` other than `OrderPaidEvent` and `OrderCancelled` (see `seller-refund-deduction`)
SHALL be ignored.

#### Scenario: Replaying a paid-order event does not credit again

- **WHEN** a seller has been credited for a paid order and that order's `OrderPaidEvent` record is produced to
  `order.events` again, byte for byte
- **THEN** after a sentinel order of the same seller is credited, the seller has exactly one `ORDER_SETTLEMENT` entry
  for the replayed order and the balance equals the two orders' amounts

#### Scenario: Re-consuming order events after a restart does not credit again

- **WHEN** a seller has been credited for two paid orders, `team-payment` is stopped, its consumer group is moved
  back to before those orders' events, and `team-payment` is started again
- **THEN** once the group has caught up the seller's ledger entries and balance are unchanged, and an order the seller
  is paid for after the restart is credited once

#### Scenario: A malformed order event is dead-lettered and later credits still apply

- **WHEN** a record whose value is not a valid event envelope is produced to `order.events`, followed by a paid order
- **THEN** the malformed record appears on `order.events.payment-settlement.dlq` and the paid order's seller is
  credited once

#### Scenario: A shipped-order event does not touch the ledger

- **WHEN** a credited order is shipped by its seller, so an `OrderShipped` event follows on `order.events`
- **THEN** after a sentinel order of the same seller is credited the seller's ledger holds only the two settlement
  credits

### Requirement: The ledger store keeps settlement and refund rows unique and sign-consistent

The `team-payment` ledger store SHALL refuse a second entry of the same type with the same reference, an
`ORDER_SETTLEMENT` entry that is not positive, a `REFUND_DEDUCTION` entry that is not negative, a `PAYOUT` entry that
is not negative unless it is a `REJECTED` reversal, and any new `ORDER_SETTLEMENT` or `REFUND_DEDUCTION` entry without
a reference. Entries written before this rule (without a reference) SHALL remain readable and count in the balance.

#### Scenario: The ledger store refuses a second settlement credit for one payment

- **WHEN** a second `ORDER_SETTLEMENT` row with the reference of an already credited payment is inserted directly
  into `team-payment`'s database
- **THEN** the insert fails with a unique violation and the seller's balance through the gateway is unchanged

#### Scenario: The ledger store refuses a settlement credit that is not positive

- **WHEN** an `ORDER_SETTLEMENT` row of -1 with a fresh reference is inserted directly into `team-payment`'s database
- **THEN** the insert fails with a check violation

#### Scenario: The ledger store refuses a refund deduction without a reference

- **WHEN** a `REFUND_DEDUCTION` row of -1 without a reference is inserted directly into `team-payment`'s database
- **THEN** the insert fails with a check violation
