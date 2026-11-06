# voucher-per-user-limits Specification

## Purpose
TBD - created by archiving change promotion-new-user-vouchers. Update Purpose after archive.

## Requirements

### Requirement: ValidateAndReserve enforces owner, per-user limit and new-customer-only

After the existing checks, `ValidateAndReserve` SHALL apply, in this order: (1) a voucher with a non-empty `owner_user_id` is refused for any other buyer with `valid=false` and reason `VOUCHER_NOT_ELIGIBLE`; (2) a voucher with `per_user_limit` N > 0 is refused with `valid=false` and reason `VOUCHER_PER_USER_LIMIT` when the buyer already has N or more reservations of that voucher that are `committed`, or `reserved` with `expires_at` after now (a `released` or expired reservation does not count); (3) a voucher with `new_user_only = true` calls `OrderService.HasCompletedOrder(buyer_id)` as a SERVICE principal with scope `order.read` and is refused with `valid=false` and reason `VOUCHER_NOT_ELIGIBLE` when it answers true. Refusals use the same response shape as today's (no gRPC error), so the checkout saga is unchanged. New holds SHALL carry `expires_at` = now + `VOUCHER_RESERVATION_TTL` (default 15m, equal to the saga `RESERVATION_TTL` of ADR-0008). The count in (2) and the hold insert SHALL run under one per-(voucher, buyer) advisory lock in one transaction. When `HasCompletedOrder` fails, times out (`UPSTREAM_ORDER_TIMEOUT`, default 500ms) or `UPSTREAM_ORDER_ADDR` is unset, the call SHALL fail closed: gRPC `UNAVAILABLE` and no reservation; this includes `PERMISSION_DENIED` or `UNAUTHENTICATED` from team-order for a missing `order.read` scope. `HasCompletedOrder` SHALL NOT be called when `new_user_only` is false. A voucher with `per_user_limit = 0` and `new_user_only = false` and empty owner SHALL behave as before. An existing reservation for the same `reservation_id` SHALL still return its original result before these checks run (idempotency).

#### Scenario: Another buyer cannot use a user-bound voucher
- **WHEN** buyer B calls `ValidateAndReserve` with the code of a VOUCHER_KIND_NEW_USER voucher owned by A, subtotal 200000
- **THEN** the call returns `valid=false` with reason `VOUCHER_NOT_ELIGIBLE` and no gRPC error, and no `voucher_reservations` row exists for that reservation id
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/voucher_per_user.feature` `Another buyer cannot use a user-bound voucher`; unit `team-promotion/internal/voucher/voucher_test.go` `TestOwnerMismatchRefused`

#### Scenario: The owner uses the voucher once
- **WHEN** owner A reserves the voucher with subtotal 200000, the hold is committed, and A reserves it again with a new reservation id
- **THEN** the first call returns valid with discount 20000 (30000 for the invited voucher); the second returns `valid=false` with reason `VOUCHER_PER_USER_LIMIT`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/voucher_per_user.feature` `The owner uses the voucher once`; unit `team-promotion/internal/voucher/voucher_test.go` `TestSingleUseOwnerVoucher`

#### Scenario: Subtotal below the minimum spend is refused for a VOUCHER_KIND_NEW_USER voucher
- **WHEN** the owner reserves with subtotal 99999 and then with subtotal 100000
- **THEN** the first returns `valid=false` with reason "order subtotal below minimum spend" and creates no hold; the second returns valid with the full discount
- **VERIFIED BY**: unit `team-promotion/internal/voucher/voucher_test.go` `TestNewUserMinSpendBoundary`

#### Scenario: Per-user limit counts live and committed holds, not released or expired ones
- **WHEN** a platform voucher has `per_user_limit = 2`, buyer B reserves twice (reservation ids r1, r2), releases r1, then reserves with r3, then reserves with r4
- **THEN** r3 is valid (count was 1) and r4 returns `valid=false` with reason `VOUCHER_PER_USER_LIMIT` (count 2)
- **VERIFIED BY**: unit `team-promotion/internal/voucher/voucher_test.go` `TestPerUserLimitCountsLiveHolds`

#### Scenario: An expired hold stops counting at its expiry
- **WHEN** buyer B holds r1 on a `per_user_limit = 1` voucher and the clock is 1 s before `expires_at`, then 0 s after
- **THEN** a reserve with r2 returns `valid=false` reason `VOUCHER_PER_USER_LIMIT` before the expiry and is valid at or after it; a committed r1 keeps counting after `expires_at`
- **VERIFIED BY**: unit `team-promotion/internal/voucher/voucher_test.go` `TestExpiredHoldNotCounted`, `TestCommittedHoldCountsPastExpiry`; Postgres `team-promotion/internal/store/migration_test.go` `TestReservationExpiresAtColumn`

#### Scenario: per_user_limit of zero keeps the global quota behaviour
- **WHEN** a voucher with `per_user_limit = 0` and quota 5 is reserved 3 times by the same buyer with distinct reservation ids
- **THEN** all 3 calls are valid and no per-user count query runs
- **VERIFIED BY**: unit `team-promotion/internal/voucher/voucher_test.go` `TestZeroPerUserLimitUnchanged`

#### Scenario: Parallel reservations cannot exceed the per-user limit
- **WHEN** 10 concurrent `ValidateAndReserve` calls with distinct reservation ids run for the same buyer on a voucher with `per_user_limit = 1`
- **THEN** exactly 1 call is valid and 9 return `valid=false` with reason `VOUCHER_PER_USER_LIMIT`
- **VERIFIED BY**: Postgres `team-promotion/internal/voucher/voucher_concurrency_test.go` `TestPerUserLimitRace`

#### Scenario: new_user_only refuses a buyer with a completed order
- **WHEN** a voucher has `new_user_only = true` and `HasCompletedOrder` answers true for buyer B
- **THEN** `ValidateAndReserve` returns `valid=false` with reason `VOUCHER_NOT_ELIGIBLE` and creates no hold
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/voucher_per_user.feature` `new_user_only refuses a buyer with a completed order`; unit `team-promotion/internal/voucher/voucher_test.go` `TestNewUserOnlyRefusesExistingCustomer`

#### Scenario: new_user_only accepts a buyer with no completed order
- **WHEN** `HasCompletedOrder` answers false for buyer B on a `new_user_only` voucher, subtotal above min spend
- **THEN** the call is valid with the voucher discount and `HasCompletedOrder` was called exactly once
- **VERIFIED BY**: unit `team-promotion/internal/voucher/voucher_test.go` `TestNewUserOnlyAcceptsNewBuyer`

#### Scenario: Order service failure fails closed
- **WHEN** a `new_user_only` voucher is reserved and `HasCompletedOrder` returns an error, returns `PERMISSION_DENIED` (token without `order.read`), exceeds 500 ms, or `UPSTREAM_ORDER_ADDR` is unset
- **THEN** the call fails with gRPC `UNAVAILABLE` and no `voucher_reservations` row exists
- **VERIFIED BY**: unit `team-promotion/internal/voucher/voucher_test.go` `TestNewUserOnlyFailsClosed`

#### Scenario: The order service is not asked for vouchers without new_user_only
- **WHEN** a voucher with `new_user_only = false` is reserved while the order client is unavailable
- **THEN** the call succeeds and the order client records zero calls
- **VERIFIED BY**: unit `team-promotion/internal/voucher/voucher_test.go` `TestOrderNotCalledWithoutFlag`

#### Scenario: Idempotent replay returns the original result before the new checks
- **WHEN** a reservation id that already holds a voucher is replayed after the buyer reached the per-user limit
- **THEN** the replay returns the original discount and the same voucher id with no error
- **VERIFIED BY**: unit `team-promotion/internal/voucher/voucher_test.go` `TestReplayBeforePerUserChecks`

### Requirement: Create and update accept per_user_limit and new_user_only

`CreateVoucher` (seller for SHOP scope, admin for PLATFORM scope, gates unchanged) SHALL accept `per_user_limit` (0 to 1000) and `new_user_only` and store them; the voucher is always kind VOUCHER_KIND_STANDARD with empty `owner_user_id`. `UpdateVoucher` (admin-only, gate unchanged) SHALL accept optional `per_user_limit` and `new_user_only`; it SHALL NOT change `owner_user_id` or `kind`, and SHALL refuse changes to a VOUCHER_KIND_NEW_USER voucher with `INVALID_ARGUMENT`. A negative or above-1000 `per_user_limit` SHALL be refused with `INVALID_ARGUMENT`. The admin audit event of create and update SHALL include the two fields.

#### Scenario: Seller creates a shop voucher with a per-user limit
- **WHEN** a seller with `listing.write` calls `CreateVoucher` with SHOP scope, `per_user_limit = 1`, `new_user_only = true`
- **THEN** the response voucher has `per_user_limit` 1, `new_user_only` true, kind VOUCHER_KIND_STANDARD, empty `owner_user_id`
- **VERIFIED BY**: unit `team-promotion/internal/rpc/voucher_per_user_test.go` `TestCreateAcceptsPerUserFields`

#### Scenario: Out-of-range per_user_limit is refused
- **WHEN** `CreateVoucher` is called with `per_user_limit = -1`, and again with 1001
- **THEN** both fail `INVALID_ARGUMENT` and no voucher row is created; with 0 and with 1000 the voucher is created
- **VERIFIED BY**: unit `team-promotion/internal/rpc/voucher_per_user_test.go` `TestPerUserLimitBounds`

#### Scenario: A buyer cannot create vouchers
- **WHEN** a principal with neither `listing.write` nor an admin scope calls `CreateVoucher` with `per_user_limit = 1`
- **THEN** the call fails `PERMISSION_DENIED` and no voucher row is created
- **VERIFIED BY**: unit `team-promotion/internal/rpc/voucher_per_user_test.go` `TestPerUserFieldsKeepAuthz`

#### Scenario: A seller cannot update vouchers
- **WHEN** a principal with `listing.write` and no admin scope calls `UpdateVoucher` with `per_user_limit = 3`
- **THEN** the call fails `PERMISSION_DENIED` and the stored row is unchanged
- **VERIFIED BY**: unit `team-promotion/internal/rpc/voucher_per_user_test.go` `TestPerUserFieldsKeepAuthz`

#### Scenario: Update changes the new fields but not owner or kind
- **WHEN** an admin calls `UpdateVoucher` with `per_user_limit = 3` on a STANDARD voucher, and again on a VOUCHER_KIND_NEW_USER voucher
- **THEN** the first returns `per_user_limit` 3 with an unchanged kind; the second fails `INVALID_ARGUMENT` and the stored row is unchanged
- **VERIFIED BY**: unit `team-promotion/internal/rpc/voucher_per_user_test.go` `TestUpdatePerUserFields`
