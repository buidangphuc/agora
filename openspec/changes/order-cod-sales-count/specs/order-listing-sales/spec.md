## MODIFIED Requirements

### Requirement: Sold count is paid minus cancelled-from-PAID or refunded

The system SHALL keep, per listing, a `sold_count` equal to the total quantity of that listing in counted orders, minus the quantity of orders cancelled from PAID and of orders whose return reached REFUNDED. An order is counted when it moves to PAID, or, for a COD order (which never reaches PAID), when it moves from SHIPPED to COMPLETED. The count SHALL never go below 0, and each order SHALL be added at most once and subtracted at most once per listing, whichever way it was counted.

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
- **VERIFIED BY**: team-order proto field check `TestNoRPCAcceptsSoldCount`

- **GIVEN** the team-order request messages in the proto
- **WHEN** any caller, including a seller or an admin, sends a request to any team-order RPC with a `sold_count` value
- **THEN** no RPC accepts that field and `listing_sales` stays unchanged

#### Scenario: An order with several lines of the same listing is summed
- **VERIFIED BY**: team-order `internal/store` `TestListingSales_SeveralLinesOfOneListingAreSummed`

- **GIVEN** order O with lines of 1 and 3 units of L in different variants
- **WHEN** O moves to PAID
- **THEN** `sold_count` for L increases by 4

#### Scenario: A COD order completed by the buyer is counted

- **GIVEN** listing L with `sold_count` 5 and a COD order O with 2 units of L in SHIPPED
- **WHEN** the buyer moves O to COMPLETED
- **THEN** `sold_count` for L is 7 and exactly one `ListingSalesChanged{listing_id=L, sold_count=7}` is produced to `order.events` within 5 s

#### Scenario: A COD order completed by the seller is counted

- **GIVEN** listing L with `sold_count` 5 and a COD order O with 2 units of L in SHIPPED
- **WHEN** the seller moves O to COMPLETED
- **THEN** `sold_count` for L is 7 and exactly one `ListingSalesChanged{listing_id=L, sold_count=7}` is produced within 5 s

#### Scenario: A COD order completed by the auto-complete sweep is counted

- **GIVEN** listing L with `sold_count` 5 and a COD order O with 2 units of L in SHIPPED whose `auto_complete_at` has passed
- **WHEN** the auto-complete sweep runs
- **THEN** O is COMPLETED, `sold_count` for L is 7 and exactly one `ListingSalesChanged{listing_id=L, sold_count=7}` is produced within 5 s

#### Scenario: A COD order cannot jump from PENDING to COMPLETED

- **GIVEN** listing L with `sold_count` 5 and a COD order O with 2 units of L in PENDING
- **WHEN** any caller tries to move O to COMPLETED
- **THEN** the call fails with the existing status-conflict error, O stays PENDING, `sold_count` for L stays 5 and no `ListingSalesChanged` row is written

#### Scenario: Completing a COD order stays within the latency budget

- **GIVEN** a local run with COD orders of up to 10 lines in SHIPPED, and a baseline p95 of SHIPPED to COMPLETED measured before this change on the same setup
- **WHEN** at least 200 such orders are completed, with baseline and change measured on the same machine in the same run, taking the median p95 of 3 runs
- **THEN** the median p95 latency is at most 5 ms above the median baseline p95

#### Scenario: A COD order is not counted before it completes

- **GIVEN** listing L with `sold_count` 5 and a COD order O with 2 units of L in PENDING
- **WHEN** O moves to SHIPPED, or is cancelled from PENDING
- **THEN** `sold_count` for L stays 5 and no `ListingSalesChanged` row is written

#### Scenario: Completing an online-paid order does not count it again

- **GIVEN** a non-COD order O with 2 units of L, counted at PAID, `sold_count` 5, now in SHIPPED
- **WHEN** O moves to COMPLETED
- **THEN** `sold_count` for L stays 5 and no new `ListingSalesChanged` row is written

#### Scenario: A repeated or concurrent COD completion counts once

- **GIVEN** a COD order O with 2 units of L in SHIPPED, `sold_count` 5
- **WHEN** two callers complete O at the same time, or the completion is retried after it succeeded
- **THEN** exactly one call succeeds, the other fails with the existing status-conflict error, and `sold_count` for L is 7 with one `ListingSalesChanged` row

#### Scenario: A COD completion that fails midway leaves no sales rows

- **GIVEN** a COD order O with 2 units of L in SHIPPED and a forced failure when the `ListingSalesChanged` outbox row is written
- **WHEN** O is moved to COMPLETED
- **THEN** the call fails, O stays SHIPPED, and `listing_sales`, the ledger and the outbox contain no row for O

#### Scenario: A COD order with several listings emits one event per listing

- **GIVEN** a COD order with lines for listings L1 (2 units) and L2 (1 unit)
- **WHEN** it moves to COMPLETED
- **THEN** one `ListingSalesChanged` per listing is produced, each carrying that listing's own `sold_count`

#### Scenario: A COD return request does not change the sold count

- **GIVEN** a COD order O with 2 units of L counted at COMPLETED, `sold_count` 7, and a return request of O
- **WHEN** the return is requested and approval is refused because the order was not paid online
- **THEN** `sold_count` for L stays 7

## MODIFIED Requirements

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

- **GIVEN** before the COD migration: listing L has `sold_count` 3, a COD COMPLETED order of 2 units not in the ledger, a COD SHIPPED order of 4 units, and a COD PENDING order of 1 unit
- **WHEN** the COD migration runs
- **THEN** `sold_count` for L is 5, the ledger holds one `counted` row for the COD COMPLETED order only, and exactly one `ListingSalesChanged{listing_id=L, sold_count=5}` outbox row is enqueued with event id `hash(L, 5, '0024-backfill-cod')`

#### Scenario: The COD backfill ignores orders already in the ledger and non-COD orders

- **GIVEN** before the COD migration: a non-COD COMPLETED order of 1 unit of L already counted at PAID, and a COD COMPLETED order already in the ledger
- **WHEN** the COD migration runs
- **THEN** `sold_count` for L is unchanged and no outbox row is enqueued for L

#### Scenario: Running the COD backfill twice does not duplicate

- **WHEN** the COD backfill statements run a second time
- **THEN** `listing_sales` totals, ledger rows and the outbox row count are unchanged

#### Scenario: The COD backfill on a database with no COD orders succeeds

- **WHEN** the COD migration runs with no COD COMPLETED orders
- **THEN** it succeeds, `listing_sales` is unchanged and no outbox row is written
