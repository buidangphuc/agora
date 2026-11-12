# my-vouchers Specification

## Purpose
TBD - created by archiving change promotion-new-user-vouchers. Update Purpose after archive.

## Requirements

### Requirement: A buyer lists their own vouchers

`VoucherService.ListMyVouchers` SHALL return the vouchers whose `owner_user_id` equals the caller principal id, limited to those that are active, started (`starts_at` is null or not after now), not expired (`ends_at` is null or after now) and not exhausted (`used < quota` or `quota = 0`), ordered by `ends_at` ascending then code, at most 50. It SHALL take no user id. It SHALL require an authenticated principal and no scope. team-gateway SHALL route it for any signed-in principal (not in the admin-procedure map) and answer `UNAUTHENTICATED` for an anonymous caller. A voucher with a live hold stays listed.

#### Scenario: A signed-in buyer sees only their own vouchers
- **WHEN** buyer A holds a VOUCHER_KIND_NEW_USER voucher and buyer B holds another, and A calls `ListMyVouchers` through the gateway
- **THEN** the response has exactly A's voucher and no voucher whose `owner_user_id` is B
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/my_vouchers.feature` `A signed-in buyer sees only their own vouchers`; unit `team-promotion/internal/rpc/voucher_my_test.go` `TestListMyVouchersOwnerOnly`

#### Scenario: An anonymous caller is refused
- **WHEN** `ListMyVouchers` is called through the gateway without a token
- **THEN** the response is `UNAUTHENTICATED` and team-promotion receives zero calls
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/my_vouchers.feature` `An anonymous caller cannot list vouchers`; unit `team-gateway/internal/edge/q7_promotion_test.go` `TestListMyVouchersAnonymous`

#### Scenario: Any signed-in principal needs no scope
- **WHEN** a buyer token without `listing.write` or admin scopes calls `ListMyVouchers`
- **THEN** the call returns OK and the policy map of the gateway still has exactly its previous entries
- **VERIFIED BY**: unit `team-gateway/internal/edge/policy_test.go` `TestAdminProceduresSetUnchanged`; `team-promotion/internal/rpc/voucher_my_test.go` `TestListMyVouchersNoScope`

#### Scenario: Used, expired, inactive and not-yet-started vouchers are hidden
- **WHEN** the buyer owns four vouchers: one with `used = quota`, one with `ends_at` = now, one deactivated, one with `starts_at` one hour ahead
- **THEN** `ListMyVouchers` returns none of them; a voucher with `ends_at` one second ahead is returned
- **VERIFIED BY**: unit `team-promotion/internal/rpc/voucher_my_test.go` `TestListMyVouchersFilters`

#### Scenario: A buyer without vouchers gets an empty list
- **WHEN** a buyer who owns no voucher calls `ListMyVouchers`
- **THEN** the response has zero vouchers and status OK
- **VERIFIED BY**: unit `team-promotion/internal/rpc/voucher_my_test.go` `TestListMyVouchersEmpty`

#### Scenario: More than 50 vouchers are capped
- **WHEN** a buyer owns 60 active user-bound vouchers
- **THEN** the response has exactly 50 vouchers, soonest expiry first
- **VERIFIED BY**: unit `team-promotion/internal/store/voucher_my_test.go` `TestListByOwnerLimit`

### Requirement: User-bound vouchers stay out of public reads

`ListVouchers` SHALL exclude every voucher with a non-empty `owner_user_id`. `GetVoucher` SHALL answer `NOT_FOUND` for a user-bound voucher unless the caller principal is its owner. `owner_user_id` SHALL be set in a response only for the owner. `OrderService.HasCompletedOrder` SHALL NOT be reachable through team-gateway.

#### Scenario: Public list excludes user-bound vouchers
- **WHEN** an anonymous visitor calls `ListVouchers` while 3 VOUCHER_KIND_NEW_USER vouchers and 2 public vouchers exist
- **THEN** the response has the 2 public vouchers only, and no `owner_user_id` is non-empty
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/my_vouchers.feature` `Public list excludes user-bound vouchers`; unit `team-promotion/internal/store/voucher_my_test.go` `TestListExcludesOwned`

#### Scenario: A non-owner cannot read a user-bound voucher by code
- **WHEN** buyer B calls `GetVoucher` with the code of A's VOUCHER_KIND_NEW_USER voucher, and A calls it with the same code
- **THEN** B gets `NOT_FOUND` and A gets the voucher
- **VERIFIED BY**: unit `team-promotion/internal/rpc/voucher_my_test.go` `TestGetVoucherOwnerOnly`

#### Scenario: HasCompletedOrder is not routed at the gateway
- **WHEN** a client posts `/platform.order.v1.OrderService/HasCompletedOrder` to the gateway with a valid buyer token
- **THEN** the response is `UNIMPLEMENTED` (HTTP 404 or 501) and team-order receives zero calls
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/my_vouchers.feature` `HasCompletedOrder is not routed at the gateway`; unit `team-gateway/internal/edge/order_test.go` `TestHasCompletedOrderNotRouted`
