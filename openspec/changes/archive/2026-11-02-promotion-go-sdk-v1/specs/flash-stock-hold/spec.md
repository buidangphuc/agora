## MODIFIED Requirements

### Requirement: Holds are persisted in a migration-managed table

Migration `0004_flash_sale_holds` SHALL create `flash_sale_holds` with `UNIQUE(reservation_id)`, an `expires_at` column set to creation time plus 15 minutes, a foreign key to `flash_sale_campaigns(id)`, and a down migration that drops it; the migration SHALL be idempotent (`IF NOT EXISTS`) and SHALL NOT alter existing rows. The migration files SHALL be embedded in the service binary under `internal/store/migrations` and applied only by the subcommand `server migrate up` (the service never migrates at boot). The service SHALL return a generic `INTERNAL` error when the store fails and SHALL NOT reserve in that case. Database tests SHALL run against a real Postgres and SHALL fail, not skip, when none is available under `make check`; a database test that reports SKIP SHALL fail the gate.

#### Scenario: Migration applies and rolls back
- **WHEN** `0004` up, down and up are applied in order on a database that has migrations 0001-0003
- **THEN** each step succeeds, the table exists after the first and third step, and does not exist after the second
- **VERIFIED BY**: team-promotion `internal/store` `TestPostgresFlashHoldMigrationRoundTrip`

#### Scenario: A store failure is a generic internal error
- **WHEN** the hold repository returns an error during reserve
- **THEN** the call returns `INTERNAL` with the message `internal error`, no hold exists, and the repository error text is absent from the response
- **VERIFIED BY**: team-promotion `internal/rpc` `TestFlashHoldStoreFailureIsInternal`

#### Scenario: A missing database fails the database tests under make check
- **WHEN** `GO_SDK_REQUIRE_DOCKER=1` is set and no Docker daemon or `PROMOTION_TEST_DATABASE_URL` is available
- **THEN** `TestPostgresFlashHoldMigrationRoundTrip` fails and is not reported as skipped
- **VERIFIED BY**: team-promotion `internal/store` `TestDatabaseTestsFailWithoutDockerWhenRequired`

### Requirement: Concurrent reserves never exceed the stock cap

The system SHALL guarantee that for any campaign, at all times, `stock_sold` plus the sum of active unexpired holds does not exceed `stock_cap` when units are taken through `ReserveFlashSaleStock`. The Postgres implementation SHALL lock the campaign row for the check-and-insert and for commit. The in-memory implementation (`internal/store/storefake`, a test fake only) SHALL hold one mutex around the same steps. The Postgres test SHALL run against a real Postgres and SHALL fail, not skip, when none is available under `make check`.

#### Scenario: Concurrent reserves are capped
- **WHEN** 20 concurrent reserves of 1 unit each, with distinct reservation ids, hit a campaign with `stock_cap` 5
- **THEN** exactly 5 responses are `reserved=true`, 15 are `reserved=false` with `reason=insufficient_stock`, and `remaining` is 0
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestReserveFlashSaleStockConcurrentCap` (fake store) and `internal/store` `TestPostgresFlashHoldConcurrentCap` (real Postgres); platform-e2e `features/promo/flash_sale_stock_hold.feature` "Concurrent reserves are capped"

#### Scenario: Concurrent reserves of one reservation id hold once
- **WHEN** 10 concurrent reserves with the same `reservation_id` and quantity 2 hit a campaign with `stock_cap` 5
- **THEN** all 10 responses are `reserved=true` and exactly one hold of 2 units exists
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestReserveFlashSaleStockConcurrentSameReservation`

#### Scenario: Concurrent commits and reserves keep the invariant
- **WHEN** a campaign with `stock_cap` 5 has 3 active holds, and in parallel the 3 holds are committed while 10 other single-unit reserves run
- **THEN** exactly 2 of the 10 reserves are `reserved=true`, `stock_sold` is 3, and `stock_sold` plus held never exceeds 5 at the end
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestFlashHoldConcurrentCommitAndReserve`

