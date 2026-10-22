## ADDED Requirements

### Requirement: ReserveFlashSaleStock holds flash stock idempotently

`FlashSaleService.ReserveFlashSaleStock` SHALL create one hold of `quantity` units for `reservation_id` on the campaign `campaign_id` that expires 15 minutes after creation, and return `reserved=true` with an empty `reason`, when all of the following hold, checked in this order with the first failure deciding the reason: (1) `not_active`: the campaign exists, is inside its `[starts_at, ends_at)` window, the flash-sale kill-switch is not OFF, its `listing_id` equals the request's and its `variant_id` is empty or equal to the request's; (2) `price_changed`: `expected_sale_price` equals the campaign's `sale_price`; (3) `insufficient_stock`: `stock_cap - stock_sold - active holds >= quantity`. A failing check SHALL return `reserved=false` with exactly that `reason` and SHALL create no hold. The only reason values are `not_active`, `price_changed` and `insufficient_stock`. A call whose `reservation_id` already has a hold SHALL return that hold's stored outcome and SHALL ignore the other request fields: `reserved=true` for an active or committed hold, `reserved=false` with `reason=not_active` for a released or expired one; it creates no second hold and re-checks nothing. The RPC SHALL return `INVALID_ARGUMENT` (no reason) for an empty `reservation_id`, `campaign_id` or `listing_id`, a `reservation_id` longer than 200 bytes, or `quantity <= 0`; an empty `variant_id` is valid. An active hold is one with status `reserved` whose expiry is later than now.

#### Scenario: Reserve holds the units of an active campaign
- **WHEN** the order service reserves 2 units with the campaign's exact `sale_price` on an active campaign with `stock_cap` 5, `stock_sold` 0 and no holds
- **THEN** the response is `reserved=true` with an empty `reason` and `GetFlashSaleStock.remaining` is 3
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockHoldsUnits`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Reserve holds the units of an active campaign"

#### Scenario: Reserve is idempotent on the reservation id
- **WHEN** the same `reservation_id` is reserved twice with identical fields on a campaign with `stock_cap` 5
- **THEN** both responses are `reserved=true`, exactly one hold of the quantity exists, and `remaining` dropped once
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockIdempotent`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Reserve is idempotent on the reservation id"

#### Scenario: Reserve replay with different fields returns the stored outcome
- **WHEN** a hold of 2 units exists for `reservation_id` "r1" and the order service reserves "r1" again with quantity 4 and a different `expected_sale_price`
- **THEN** the response is `reserved=true`, the hold still has 2 units, and `remaining` is unchanged
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockReplayIgnoresChangedFields`

#### Scenario: Reserve refuses an ended campaign
- **WHEN** the order service reserves 1 unit on a campaign whose `ends_at` is in the past
- **THEN** the response is `reserved=false` with `reason=not_active` and no hold exists
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockRefusesInactiveWindow`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Reserve refuses an ended campaign"

#### Scenario: Reserve refuses a campaign that has not started
- **WHEN** the order service reserves 1 unit on a campaign whose `starts_at` is in the future
- **THEN** the response is `reserved=false` with `reason=not_active`
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockRefusesInactiveWindow`

#### Scenario: Reserve refuses while the kill-switch is OFF
- **WHEN** the flash-sale kill-switch is OFF and the order service reserves 1 unit on an otherwise active campaign
- **THEN** the response is `reserved=false` with `reason=not_active` and no hold exists
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockRefusesWhenKillSwitchOff`

#### Scenario: Reserve refuses a price that differs from the campaign
- **WHEN** the order service reserves with `expected_sale_price` 1 below the campaign's `sale_price`
- **THEN** the response is `reserved=false` with `reason=price_changed` and no hold exists
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockRefusesPriceMismatch`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Reserve refuses a price that differs from the campaign"

#### Scenario: Reserve refuses more units than remain
- **WHEN** a campaign has `stock_cap` 5 and 4 units are held, and the order service reserves 2
- **THEN** the response is `reserved=false` with `reason=insufficient_stock` and the held total stays 4
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockRefusesInsufficientStock`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Reserve refuses more units than remain"

#### Scenario: Reserve accepts exactly the remaining units
- **WHEN** a campaign has `stock_cap` 5 with `stock_sold` 2 and 1 unit held, and the order service reserves 2
- **THEN** the response is `reserved=true` and `remaining` is 0
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockBoundaryExactRemaining`

#### Scenario: The first failing check decides the reason
- **WHEN** the order service reserves on an ended campaign with a wrong `expected_sale_price` and a quantity above the remaining stock, then on an active campaign with a wrong price and a quantity above the remaining stock
- **THEN** the first response has `reason=not_active` and the second has `reason=price_changed`
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockReasonOrder`

#### Scenario: Reserve covers a variant campaign only for that variant
- **WHEN** a campaign has `variant_id` "v1" and the order service reserves for variant "v2" of the same listing, then for "v1", and a second campaign with empty `variant_id` is reserved for variant "v2"
- **THEN** the first response is `reserved=false` with `reason=not_active`, the second is `reserved=true`, and the third is `reserved=true`
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockVariantScope`

#### Scenario: Reserve refuses a listing that is not the campaign's
- **WHEN** the order service reserves with a `listing_id` that differs from the campaign's
- **THEN** the response is `reserved=false` with `reason=not_active`
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockVariantScope`

#### Scenario: Reserve refuses an unknown campaign
- **WHEN** the order service reserves on a `campaign_id` that does not exist
- **THEN** the response is `reserved=false` with `reason=not_active` and no error
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockUnknownCampaign`

#### Scenario: Reserve replay after release is not active
- **WHEN** a hold is released and the same `reservation_id` is reserved again
- **THEN** the response is `reserved=false` with `reason=not_active` and the held total stays 0
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockReplayAfterRelease`

#### Scenario: Reserve replay after expiry is not active
- **WHEN** a hold is 15 minutes old and the same `reservation_id` is reserved again
- **THEN** the response is `reserved=false` with `reason=not_active` and no new hold exists
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockReplayAfterExpiry`

#### Scenario: A hold expires 15 minutes after reserve
- **WHEN** 2 units are held on a campaign with `stock_cap` 5 and the clock moves to 14 minutes 59 seconds, then to 15 minutes
- **THEN** `remaining` is 3 at 14:59 and 5 at 15:00, and a further reserve of 5 units at 15:00 is `reserved=true`
- **VERIFIED BY**: team-promotion `internal/service` `TestFlashHoldExpiresAfterFifteenMinutes`

#### Scenario: Reserve rejects malformed input
- **WHEN** reserve is called with an empty `reservation_id`, or an empty `campaign_id`, or an empty `listing_id`, or `quantity` 0, or `quantity` -1
- **THEN** each call returns `INVALID_ARGUMENT` with no `reason` and no hold exists
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldRPCsRejectInvalidInput`

#### Scenario: A reservation id of 200 bytes is accepted and 201 is rejected
- **WHEN** reserve is called with a `reservation_id` of 200 bytes, then of 201 bytes
- **THEN** the first returns `reserved=true` with an empty `reason` and the second returns `INVALID_ARGUMENT`
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldReservationIDLengthBoundary`

#### Scenario: An empty variant id is valid
- **WHEN** reserve is called with an empty `variant_id` on a campaign with an empty `variant_id`
- **THEN** the response is `reserved=true`
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldEmptyVariantAccepted`

### Requirement: Concurrent reserves never exceed the stock cap

The system SHALL guarantee that for any campaign, at all times, `stock_sold` plus the sum of active unexpired holds does not exceed `stock_cap` when units are taken through `ReserveFlashSaleStock`. The Postgres implementation SHALL lock the campaign row for the check-and-insert and for commit; the in-memory implementation SHALL hold one mutex around the same steps.

#### Scenario: Concurrent reserves are capped
- **WHEN** 20 concurrent reserves of 1 unit each, with distinct reservation ids, hit a campaign with `stock_cap` 5
- **THEN** exactly 5 responses are `reserved=true`, 15 are `reserved=false` with `reason=insufficient_stock`, and `remaining` is 0
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockConcurrentCap` (in-memory) and `internal/repository` `TestPostgresFlashHoldConcurrentCap` (skipped without a database); platform-e2e `features/promo/flash_sale_stock_hold.feature` "Concurrent reserves are capped"

#### Scenario: Concurrent reserves of one reservation id hold once
- **WHEN** 10 concurrent reserves with the same `reservation_id` and quantity 2 hit a campaign with `stock_cap` 5
- **THEN** all 10 responses are `reserved=true` and exactly one hold of 2 units exists
- **VERIFIED BY**: team-promotion `internal/service` `TestReserveFlashSaleStockConcurrentSameReservation`

#### Scenario: Concurrent commits and reserves keep the invariant
- **WHEN** a campaign with `stock_cap` 5 has 3 active holds, and in parallel the 3 holds are committed while 10 other single-unit reserves run
- **THEN** exactly 2 of the 10 reserves are `reserved=true`, `stock_sold` is 3, and `stock_sold` plus held never exceeds 5 at the end
- **VERIFIED BY**: team-promotion `internal/service` `TestFlashHoldConcurrentCommitAndReserve`

### Requirement: CommitFlashSaleStock sells the held units exactly once

`FlashSaleService.CommitFlashSaleStock`, called by the order service in checkout Phase B, SHALL, for an active hold (status `reserved`, not expired), set it to `committed` and add its `quantity` to the campaign's `stock_sold` in one transaction, and return `committed=true`. A second commit of a committed hold SHALL return `committed=true` and change nothing. A commit of an unknown, released or expired `reservation_id` SHALL return `committed=false` and change nothing. Commit SHALL NOT re-check the sale window or the kill-switch. Empty `reservation_id` or one longer than 200 bytes SHALL return `INVALID_ARGUMENT`.

#### Scenario: Commit adds the quantity to stock_sold
- **WHEN** a hold of 2 units on a campaign with `stock_cap` 5 is committed
- **THEN** the response is `committed=true`, the campaign's `stock_sold` is 2, and `GetFlashSaleStock.remaining` is 3
- **VERIFIED BY**: team-promotion `internal/service` `TestCommitFlashSaleStockAddsToSold`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Commit adds the quantity to stock_sold"

#### Scenario: Commit is idempotent
- **WHEN** the same hold is committed twice
- **THEN** both responses are `committed=true` and `stock_sold` grew by the quantity once
- **VERIFIED BY**: team-promotion `internal/service` `TestCommitFlashSaleStockIdempotent`

#### Scenario: Commit of an unknown reservation does nothing
- **WHEN** commit is called with a `reservation_id` that has no hold
- **THEN** the response is `committed=false` and no campaign's `stock_sold` changes
- **VERIFIED BY**: team-promotion `internal/service` `TestCommitFlashSaleStockUnknown`

#### Scenario: Commit of a released hold is refused
- **WHEN** a hold is released and then committed
- **THEN** the response is `committed=false` and `stock_sold` is unchanged
- **VERIFIED BY**: team-promotion `internal/service` `TestCommitFlashSaleStockAfterRelease`

#### Scenario: Commit of an expired hold is refused
- **WHEN** a hold is 15 minutes old and is committed
- **THEN** the response is `committed=false` and `stock_sold` is unchanged
- **VERIFIED BY**: team-promotion `internal/service` `TestCommitFlashSaleStockAfterExpiry`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Commit of an expired hold is refused"

#### Scenario: Commit still works after the sale window ended
- **WHEN** a hold was taken while the campaign was active, the campaign's `ends_at` has since passed, the hold is under 15 minutes old, and it is committed
- **THEN** the response is `committed=true` and `stock_sold` includes the quantity
- **VERIFIED BY**: team-promotion `internal/service` `TestCommitFlashSaleStockAfterWindowEnded`

#### Scenario: Commit rejects an empty reservation id
- **WHEN** commit is called with an empty `reservation_id`, or one of 201 bytes
- **THEN** each call returns `INVALID_ARGUMENT`
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldRPCsRejectInvalidInput`

### Requirement: ReleaseFlashSaleStock returns a hold idempotently

`FlashSaleService.ReleaseFlashSaleStock` SHALL set an active hold to `released` so its units count as remaining again, and return `released=true`. For a `committed` hold it SHALL set the hold to `released` and subtract its `quantity` from `stock_sold` in one transaction, and return `released=true`. A release of an already released hold, or of an expired uncommitted hold, SHALL return `released=true` and change no counter (a repeat never decrements `stock_sold` twice). A release of an unknown `reservation_id` SHALL return `released=false`, SHALL NOT return an error, and SHALL change nothing. Empty `reservation_id` or one longer than 200 bytes SHALL return `INVALID_ARGUMENT`.

#### Scenario: Release returns the units of an active hold
- **WHEN** a hold of 2 units on a campaign with `stock_cap` 5 is released
- **THEN** the response is `released=true` and `GetFlashSaleStock.remaining` is 5
- **VERIFIED BY**: team-promotion `internal/service` `TestReleaseFlashSaleStockReturnsUnits`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Release returns the units of an active hold"

#### Scenario: Release returns the units of a committed hold
- **WHEN** a hold of 2 units on a campaign with `stock_cap` 5 is committed and then released
- **THEN** the response is `released=true`, `stock_sold` is 0 and `remaining` is 5
- **VERIFIED BY**: team-promotion `internal/service` `TestReleaseFlashSaleStockAfterCommit`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Release returns the units of a committed hold"

#### Scenario: Release is idempotent
- **WHEN** a committed hold of 2 units is released twice
- **THEN** both responses are `released=true` and `stock_sold` is 0 after each (decremented once)
- **VERIFIED BY**: team-promotion `internal/service` `TestReleaseFlashSaleStockIdempotent`

#### Scenario: Release of an unknown reservation is a no-op
- **WHEN** release is called with a `reservation_id` that has no hold
- **THEN** the response is `released=false`, there is no error, and `remaining` is unchanged
- **VERIFIED BY**: team-promotion `internal/service` `TestReleaseFlashSaleStockUnknownNoOp`

#### Scenario: Release of an expired hold changes no counter
- **WHEN** a hold is 15 minutes old and is released
- **THEN** the response is `released=true` and `stock_sold` and `remaining` are unchanged
- **VERIFIED BY**: team-promotion `internal/service` `TestReleaseFlashSaleStockExpiredNoCounterChange`

#### Scenario: Release rejects an empty reservation id
- **WHEN** release is called with an empty `reservation_id`, or one of 201 bytes
- **THEN** each call returns `INVALID_ARGUMENT`
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldRPCsRejectInvalidInput`

### Requirement: The stock meter counts holds

`FlashSaleService.GetFlashSaleStock` SHALL return `remaining = max(0, stock_cap - stock_sold - sum of active unexpired holds)` and `stock_cap` unchanged. Commit SHALL NOT change `remaining`; release SHALL raise it by the released quantity; an expired hold SHALL NOT count.

#### Scenario: Remaining reflects holds and sold units
- **WHEN** a campaign has `stock_cap` 10, `stock_sold` 3 and an active hold of 2, a released hold of 4 and an expired hold of 1
- **THEN** `GetFlashSaleStock` returns `remaining` 5 and `stock_cap` 10
- **VERIFIED BY**: team-promotion `internal/service` `TestGetFlashSaleStockCountsHolds`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "Remaining reflects holds and sold units"

#### Scenario: Remaining is floored at zero
- **WHEN** a campaign has `stock_sold` above `stock_cap` and a reserved hold
- **THEN** `GetFlashSaleStock` returns `remaining` 0
- **VERIFIED BY**: team-promotion `internal/service` `TestGetFlashSaleStockRemaining`

#### Scenario: Stock of an unknown campaign is not found
- **WHEN** `GetFlashSaleStock` is called with a `campaign_id` that does not exist
- **THEN** the call returns `NOT_FOUND`
- **VERIFIED BY**: team-promotion `internal/handler` `TestGetFlashSaleStockUnknownNotFound`

### Requirement: The flash hold RPCs are service-only

The three RPCs `ReserveFlashSaleStock`, `CommitFlashSaleStock` and `ReleaseFlashSaleStock` SHALL be served only to a principal of type SERVICE that holds scope `promotion.reserve`. A user principal, including one that carries the `promotion.reserve` scope string or `admin`, SHALL get `PERMISSION_DENIED`; a service principal without the scope SHALL get `PERMISSION_DENIED`; a missing or anonymous principal SHALL get `UNAUTHENTICATED`. A refused call SHALL change no hold and no `stock_sold`. The RPCs SHALL NOT be reachable through the gateway by a browser.

#### Scenario: A buyer cannot reserve flash stock
- **WHEN** a user principal (buyer, no scopes) calls reserve, commit and release
- **THEN** each call returns `PERMISSION_DENIED` and `GetFlashSaleStock.remaining` is unchanged
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldRPCsAreServiceOnly`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "A buyer cannot reserve flash stock"

#### Scenario: A user carrying the service scope is still refused
- **WHEN** a principal of type user with scopes `promotion.reserve` and `admin` calls reserve
- **THEN** the call returns `PERMISSION_DENIED`
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldRPCsAreServiceOnly`; platform-e2e `features/promo/flash_sale_stock_hold.feature` "A user carrying the service scope is still refused"

#### Scenario: A service without the scope is refused
- **WHEN** a service principal `service-other` without `promotion.reserve` calls commit
- **THEN** the call returns `PERMISSION_DENIED`
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldRPCsAreServiceOnly`

#### Scenario: An anonymous caller is unauthenticated
- **WHEN** an anonymous principal or no principal calls release
- **THEN** the call returns `UNAUTHENTICATED`
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldRPCsAreServiceOnly`

#### Scenario: The order service principal is served
- **WHEN** a service principal `service-team-order` with scope `promotion.reserve` calls reserve, commit and release in turn
- **THEN** each call returns OK
- **VERIFIED BY**: platform-e2e `features/promo/flash_sale_stock_hold.feature` "The order service principal is served"

### Requirement: Holds are persisted in a migration-managed table

Migration `0004_flash_sale_holds` SHALL create `flash_sale_holds` with `UNIQUE(reservation_id)`, an `expires_at` column set to creation time plus 15 minutes, a foreign key to `flash_sale_campaigns(id)`, and a down migration that drops it; the migration SHALL be idempotent (`IF NOT EXISTS`) and SHALL NOT alter existing rows. The service SHALL return a generic `INTERNAL` error when the store fails and SHALL NOT reserve in that case.

#### Scenario: Migration applies and rolls back
- **WHEN** `0004` up, down and up are applied in order on a database that has migrations 0001-0003
- **THEN** each step succeeds, the table exists after the first and third step, and does not exist after the second
- **VERIFIED BY**: team-promotion `internal/repository` `TestPostgresFlashHoldMigrationRoundTrip` (skipped without a database)

#### Scenario: A store failure is a generic internal error
- **WHEN** the hold repository returns an error during reserve
- **THEN** the call returns `INTERNAL` with the message `internal error`, no hold exists, and the repository error text is absent from the response
- **VERIFIED BY**: team-promotion `internal/handler` `TestFlashHoldStoreFailureIsInternal`

### Requirement: Old holds are purged without losing committed ones

A daily purge SHALL delete only `released` holds and expired uncommitted holds more than 30 days after they became terminal. It SHALL NOT delete an active hold or a `committed` hold of any age; committed holds SHALL remain until their campaign is deleted (cascade), so a release of an old committed hold still returns its units.

#### Scenario: Released and expired holds are purged after 30 days
- **WHEN** the purge runs with a released hold 31 days old, an expired uncommitted hold 31 days past expiry, a released hold 29 days old and an active hold
- **THEN** the first two rows are deleted and the other two remain
- **VERIFIED BY**: team-promotion `internal/repository` `TestPurgeReleasedAndExpiredHolds`

#### Scenario: A committed hold older than 30 days is not purged
- **WHEN** the purge runs with a committed hold 90 days old on a campaign with `stock_sold` 2, and that hold is then released
- **THEN** the hold row still existed at purge time, the release returns `released=true`, and `stock_sold` is 0
- **VERIFIED BY**: team-promotion `internal/repository` `TestPurgeKeepsCommittedHolds` and `internal/service` `TestReleaseOldCommittedHoldAfterPurge`
