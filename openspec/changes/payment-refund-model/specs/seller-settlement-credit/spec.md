## MODIFIED Requirements

### Requirement: Settlement credits survive redelivery, restarts and poison records

The credit consumer SHALL be at-least-once and idempotent. It SHALL commit a record's offset only after the credit is
written or the record is parked on the dead-letter topic `order.events.payment-settlement.dlq`, so a record not yet
committed is processed after a restart. A redelivered or replayed `OrderPaidEvent` SHALL leave the ledger unchanged.
A record that can never be applied SHALL be parked on the dead-letter topic without blocking later records. The
consumer SHALL ignore every event type on `order.events` other than `OrderPaidEvent`, `OrderCancelled` (see
`seller-refund-deduction`) and `ReturnRefunded` (see `return-refund-settlement`).

#### Scenario: Replaying a paid-order event does not credit again

- **WHEN** a seller has been credited for a paid order and that order's `OrderPaidEvent` record is produced to
  `order.events` again, byte for byte
- **THEN** after a sentinel order of the same seller is credited, the seller has exactly one `ORDER_SETTLEMENT` entry
  for the replayed order
- **AND** the balance equals the two orders' amounts

#### Scenario: Re-consuming order events after a restart does not credit again

- **WHEN** a seller has been credited for two paid orders, `team-payment` is stopped, its consumer group is moved
  back to before those orders' events, and `team-payment` is started again
- **THEN** once the group has caught up, the seller's ledger entries and balance are unchanged
- **AND** an order the seller is paid for after the restart is credited once

#### Scenario: A malformed order event is dead-lettered and later credits still apply

- **WHEN** a record whose value is not a valid event envelope is produced to `order.events`, followed by a paid order
- **THEN** the malformed record appears on `order.events.payment-settlement.dlq`
- **AND** the paid order's seller is credited once

#### Scenario: A shipped-order event does not touch the ledger

- **WHEN** a credited order is shipped by its seller, so an `OrderShipped` event follows on `order.events`
- **THEN** after a sentinel order of the same seller is credited, the seller's ledger holds only the two settlement
  credits
