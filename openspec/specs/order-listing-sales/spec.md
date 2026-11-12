# order-listing-sales Specification

## Purpose
TBD - created by archiving change order-listing-sales. Update Purpose after archive.

## Requirements

### Requirement: Sold count is paid minus cancelled-from-PAID or refunded

The system SHALL keep, per listing, a `sold_count` equal to the total quantity of that listing in paid orders, minus the quantity of orders cancelled from PAID and of orders whose return reached REFUNDED. The count SHALL never go below 0, and each order SHALL be added at most once and subtracted at most once per listing.

#### Scenario: Payment adds the order quantity

- **GIVEN** listing L with `sold_count` 0 and a PENDING order O with 2 units of L
- **WHEN** O moves to PAID
- **THEN** `listing_sales.sold_count` for L is 2

#### Scenario: Cancel from PAID subtracts the order quantity

- **GIVEN** a PAID order O with 2 units of L, `sold_count` 5
- **WHEN** O is cancelled from PAID
- **THEN** `sold_count` for L is 3

#### Scenario: Refunded return subtracts the order quantity

- **GIVEN** a PAID order O with 2 units of L, `sold_count` 5, and an APPROVED return of O
- **WHEN** the return moves to REFUNDED
- **THEN** `sold_count` for L is 3

#### Scenario: Cancel before PAID changes nothing

- **GIVEN** a PENDING order O with 2 units of L, `sold_count` 5
- **WHEN** O is cancelled from PENDING
- **THEN** `sold_count` for L stays 5 and no `ListingSalesChanged` row is written

#### Scenario: A repeated or redelivered transition changes nothing

- **GIVEN** order O already counted and then cancelled from PAID, `sold_count` 3
- **WHEN** the cancel or the `OrderCancelled` handling for O is applied a second time
- **THEN** `sold_count` stays 3 and no new `ListingSalesChanged` row is written

#### Scenario: A cancel and a refund of the same order subtract once

- **GIVEN** a PAID order O with 2 units of L, `sold_count` 5
- **WHEN** O is cancelled from PAID and a refund for O is then applied
- **THEN** `sold_count` for L is 3

#### Scenario: The count never goes below 0
- **VERIFIED BY**: team-order `internal/store` `TestListingSales_FloorsAtZeroWithSeededCount`

- **GIVEN** `listing_sales.sold_count` for L is seeded directly to 1, bypassing the ledger, and a counted order O with 2 units of L
- **WHEN** O is cancelled from PAID
- **THEN** `sold_count` for L is 0 and the stored value is never negative

#### Scenario: A refund or cancel with no counted ledger rows changes nothing
- **VERIFIED BY**: team-order `internal/store` `TestListingSales_NoCountedRowsChangesNothing`

- **GIVEN** a PAID order O whose ledger has no `counted` rows, and `sold_count` 5 for L
- **WHEN** O is cancelled from PAID or its return is refunded
- **THEN** `sold_count` for L stays 5 and no `ListingSalesChanged` row is written

#### Scenario: A client cannot set the sold count

- **WHEN** any caller, including a seller or an admin, sends a request to any team-order RPC with a `sold_count` value
- **THEN** no RPC accepts that field and `listing_sales` stays unchanged

#### Scenario: An order with several lines of the same listing is summed
- **VERIFIED BY**: team-order `internal/store` `TestListingSales_SeveralLinesOfOneListingAreSummed`

- **GIVEN** order O with lines of 1 and 3 units of L in different variants
- **WHEN** O moves to PAID
- **THEN** `sold_count` for L increases by 4

### Requirement: Sales changes are published as absolute values

The system SHALL write one `ListingSalesChanged{listing_id, sold_count, changed_at}` row to `order_outbox_events` in the same transaction as each projection change, produce it to `order.events` keyed by `listing_id`, and carry the absolute post-change `sold_count`, with no party or order identifier.

#### Scenario: A payment emits the new absolute value

- **GIVEN** `sold_count` for L is 3
- **WHEN** an order of 2 units of L is paid
- **THEN** exactly one `ListingSalesChanged{listing_id=L, sold_count=5}` is produced to `order.events` within 5 s

#### Scenario: The event commits or rolls back with the state
- **VERIFIED BY**: team-order `internal/store` `TestListingSales_PaidRollbackLeavesNoRows`

- **WHEN** the transaction that moves an order to PAID is rolled back
- **THEN** `listing_sales`, the ledger and the outbox contain no row for that order

#### Scenario: A multi-listing order emits one event per listing

- **GIVEN** an order with lines for listings L1 and L2
- **WHEN** it moves to PAID
- **THEN** one event per listing is produced, each carrying that listing's own `sold_count`

#### Scenario: The payload carries no personal identifier

- **WHEN** a `ListingSalesChanged` message is decoded
- **THEN** it holds only `listing_id`, `sold_count` and `changed_at`

#### Scenario: Backfill events use a stable id and the commit time
- **VERIFIED BY**: team-order `migrations` `TestMigration0023_BackfillEventIDIsStableAndUsesCommitTime`

- **WHEN** the migration enqueues the event for listing L with `sold_count` 3
- **THEN** its event id equals `hash(L, 3, 'backfill')` and `changed_at` equals the migration transaction's commit time

#### Scenario: A redelivered relay does not change the meaning
- **VERIFIED BY**: team-order `internal/events` `TestListingSalesChanged_RelayedTwiceKeepsValueAndID`

- **WHEN** the relayer produces the same `ListingSalesChanged` row twice
- **THEN** both messages carry the same `sold_count` and the same event id

### Requirement: The migration backfills existing orders

The migrations SHALL seed `listing_sales` and the per-order ledger from existing orders (paid and not cancelled, minus refunded returns) and SHALL enqueue exactly one `ListingSalesChanged` outbox row per listing that has an order line, without duplicating rows when run again. A further migration (0024) SHALL add COD orders already COMPLETED (not yet in the ledger) to the ledger and to `sold_count`, and enqueue one `ListingSalesChanged` row per listing whose `sold_count` changed, with an event id that includes the migration id so it cannot collide with 0023 backfill ids.

#### Scenario: Existing paid orders are counted
- **VERIFIED BY**: team-order `migrations` `TestMigration0023_ExistingPaidOrdersAreCounted`

- **GIVEN** before the migration: listing L has a PAID order of 2 units, a COMPLETED order of 1 unit, a PENDING order of 4 units
- **WHEN** the migration runs
- **THEN** `listing_sales.sold_count` for L is 3

#### Scenario: Orders paid before paid_at was recorded are counted
- **VERIFIED BY**: team-order `migrations` `TestMigration0023_OrdersPaidBeforePaidAtAreCounted`

- **GIVEN** before the migration: a non-COD order of 2 units of L in SHIPPED with `paid_at` NULL
- **WHEN** the migration runs
- **THEN** `sold_count` for L is 2

#### Scenario: Cancelled and refunded orders are excluded
- **VERIFIED BY**: team-order `migrations` `TestMigration0023_CancelledAndRefundedOrdersAreExcluded`

- **GIVEN** before the migration: L has a paid-then-cancelled order of 2 units and a paid order of 1 unit with a REFUNDED return
- **WHEN** the migration runs
- **THEN** `sold_count` for L is 0 and one event with `sold_count=0` is enqueued

#### Scenario: One event per listing

- **GIVEN** 3 listings with order lines, one of them with 5 orders
- **WHEN** the migration runs
- **THEN** exactly 3 `ListingSalesChanged` outbox rows exist, one per listing

#### Scenario: Running the migration body twice does not duplicate
- **VERIFIED BY**: team-order `migrations` `TestMigration0023_BackfillTwiceDoesNotDuplicate`

- **WHEN** the backfill statements run a second time
- **THEN** `listing_sales` totals and the outbox row count are unchanged

#### Scenario: An empty database migrates cleanly
- **VERIFIED BY**: team-order `migrations` `TestMigration0023_EmptyDatabase`

- **WHEN** the migration runs with no orders
- **THEN** it succeeds, both tables are empty and no outbox row is written

#### Scenario: A backfilled order cancelled later is subtracted once
- **VERIFIED BY**: team-order `migrations` `TestMigration0023_BackfilledOrderCancelledLaterIsSubtractedOnce`

- **GIVEN** a backfilled order of 2 units counted in `sold_count` 2
- **WHEN** it is cancelled from PAID
- **THEN** `sold_count` is 0

#### Scenario: Historical COD completed orders are counted once
- **VERIFIED BY**: team-order `migrations` `TestMigration0024_HistoricalCodCompletedOrdersAreCountedOnce`

- **GIVEN** before the COD migration: listing L has `sold_count` 3, a COD COMPLETED order of 2 units not in the ledger, a COD SHIPPED order of 4 units, and a COD PENDING order of 1 unit
- **WHEN** the COD migration runs
- **THEN** `sold_count` for L is 5, the ledger holds one `counted` row for the COD COMPLETED order only, and exactly one `ListingSalesChanged{listing_id=L, sold_count=5}` outbox row is enqueued with event id `hash(L, 5, '0024-backfill-cod')`

#### Scenario: The COD backfill ignores orders already in the ledger and non-COD orders
- **VERIFIED BY**: team-order `migrations` `TestMigration0024_IgnoresLedgerAndNonCodOrders`

- **GIVEN** before the COD migration: a non-COD COMPLETED order of 1 unit of L already counted at PAID, and a COD COMPLETED order already in the ledger
- **WHEN** the COD migration runs
- **THEN** `sold_count` for L is unchanged and no outbox row is enqueued for L

#### Scenario: Running the COD backfill twice does not duplicate
- **VERIFIED BY**: team-order `migrations` `TestMigration0024_BackfillTwiceDoesNotDuplicate`

- **WHEN** the COD backfill statements run a second time
- **THEN** `listing_sales` totals, ledger rows and the outbox row count are unchanged

#### Scenario: The COD backfill on a database with no COD orders succeeds
- **VERIFIED BY**: team-order `migrations` `TestMigration0024_NoCodOrders`

- **WHEN** the COD migration runs with no COD COMPLETED orders
- **THEN** it succeeds, `listing_sales` is unchanged and no outbox row is written
