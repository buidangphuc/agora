# voucher-scope Specification

## Purpose
Voucher scope (shop or platform) and owning shop reported by voucher validation and reservation.

## Requirements

### Requirement: ValidateAndReserve reports the voucher scope and shop

`VoucherService.ValidateAndReserve` SHALL return, in `ValidateAndReserveResponse.scope` and `seller_id`, the stored `scope` and `seller_id` of the voucher identified by the request `code`, whenever that voucher is known: for a new valid hold, for any rejection (expired, not yet active, below `min_spend`, quota exhausted, not valid for this seller) and for an idempotent replay (the voucher of the stored hold, including a replay of a released hold). The values SHALL come from the stored voucher and never from the request. A PLATFORM voucher SHALL have an empty `seller_id`. When no voucher is known (empty `reservation_id`, empty code, unknown code, or a replayed hold whose voucher no longer exists) the response SHALL carry `VOUCHER_SCOPE_UNSPECIFIED` and an empty `seller_id`. `valid`, `reason`, `discount_amount` and `voucher_id` SHALL be exactly what they were before this change.

#### Scenario: Shop voucher reports SHOP scope and its seller
- **WHEN** the order service reserves a valid shop voucher owned by seller S1 with `seller_id` S1 and a `cart_subtotal` at or above its `min_spend`
- **THEN** the response has `valid=true`, `scope=VOUCHER_SCOPE_SHOP` and `seller_id=S1`
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveReportsShopScope`; platform-e2e `features/promo/voucher_scope.feature` "Shop voucher reports SHOP scope and its seller"

#### Scenario: Platform voucher reports PLATFORM scope and no seller
- **WHEN** the order service reserves a valid platform voucher with an empty `seller_id`
- **THEN** the response has `valid=true`, `scope=VOUCHER_SCOPE_PLATFORM` and an empty `seller_id`
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReservePlatformScopeNoSeller`; platform-e2e `features/promo/voucher_scope.feature` "Platform voucher reports PLATFORM scope and no seller"

#### Scenario: Platform voucher ignores the seller id of the request
- **WHEN** the order service reserves a valid platform voucher with `seller_id` S9 in the request
- **THEN** the response has `valid=true`, `scope=VOUCHER_SCOPE_PLATFORM` and an empty `seller_id`
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReservePlatformIgnoresRequestSeller`

#### Scenario: Platform voucher min spend is checked on the subtotal sent
- **WHEN** the order service reserves a platform voucher with `min_spend` 500000 and `cart_subtotal` 500000, then another platform voucher reservation id for the same code with `cart_subtotal` 499999
- **THEN** the first response is `valid=true` and the second is `valid=false` with reason "order subtotal below minimum spend", and both report `scope=VOUCHER_SCOPE_PLATFORM`
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReservePlatformMinSpendBoundary`; platform-e2e `features/promo/voucher_scope.feature` "Platform voucher min spend is checked on the subtotal sent"

#### Scenario: A shop voucher for another seller is refused and still reports its shop
- **WHEN** the order service reserves a shop voucher owned by seller S1 with `seller_id` S2
- **THEN** the response has `valid=false`, reason "voucher not valid for this seller", `scope=VOUCHER_SCOPE_SHOP`, `seller_id=S1`, and no hold exists
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveWrongSellerReportsOwnerScope`; platform-e2e `features/promo/voucher_scope.feature` "A shop voucher for another seller is refused and still reports its shop"

#### Scenario: A shop voucher without a seller id is refused
- **WHEN** the order service reserves a shop voucher owned by seller S1 with an empty `seller_id`
- **THEN** the response has `valid=false`, reason "voucher not valid for this seller", `scope=VOUCHER_SCOPE_SHOP` and no hold exists
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveShopVoucherRequiresSeller`

#### Scenario: A rejected voucher still reports its scope
- **WHEN** the order service reserves an expired platform voucher, then a shop voucher whose quota is exhausted
- **THEN** both responses have `valid=false` with the existing reasons ("voucher expired", "voucher quota exhausted"), the first reports `scope=VOUCHER_SCOPE_PLATFORM` and the second `scope=VOUCHER_SCOPE_SHOP` with its seller
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveRejectionsReportScope`

#### Scenario: An unknown code reports no scope
- **WHEN** the order service reserves a code that does not exist, then an empty code
- **THEN** both responses have `valid=false` ("voucher code not found", "voucher code is required"), `scope=VOUCHER_SCOPE_UNSPECIFIED` and an empty `seller_id`
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveUnknownCodeNoScope`; platform-e2e `features/promo/voucher_scope.feature` "An unknown code reports no scope"

#### Scenario: Replay reports the stored voucher's scope
- **WHEN** a hold exists for reservation id "r1" on shop voucher V of seller S1, and the order service reserves "r1" again with a different code, `seller_id` and `cart_subtotal`
- **THEN** the response is `valid=true` with the stored `discount_amount` and `voucher_id`, `scope=VOUCHER_SCOPE_SHOP` and `seller_id=S1`
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveReplayReportsStoredScope`

#### Scenario: Replay of a released hold reports the scope
- **WHEN** the hold for "r1" on platform voucher P was released and the order service reserves "r1" again
- **THEN** the response is `valid=false` with reason "reservation already released", `scope=VOUCHER_SCOPE_PLATFORM` and an empty `seller_id`
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveReleasedReplayReportsScope`

#### Scenario: Replay whose voucher is gone keeps its stored outcome
- **WHEN** a hold exists for "r1" whose voucher can no longer be loaded and the order service reserves "r1" again
- **THEN** the response keeps the stored `valid`, `discount_amount` and `voucher_id` with `scope=VOUCHER_SCOPE_UNSPECIFIED` and an empty `seller_id`
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveReplayMissingVoucherNoScope`

#### Scenario: A repository failure on replay is an internal error
- **WHEN** the voucher lookup of a replayed hold fails with a store error other than not found
- **THEN** the service returns the error and the handler answers `INTERNAL` with a generic message
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveReplayStoreErrorPropagates`

#### Scenario: Discount and reasons are unchanged
- **WHEN** the existing voucher cases run (percent with `max_discount`, fixed clamped to the subtotal, expired, quota exhausted, idempotent hold)
- **THEN** every `valid`, `reason`, `discount_amount` and `voucher_id` equals the value asserted before this change
- **VERIFIED BY**: team-promotion `internal/service` `TestValidateAndReserveIdempotent`, `TestValidateAndReserveRejections`, `TestQuotaExhaustedRejected`, `TestComputeDiscount`

### Requirement: The scope is reported on every authorised caller path

The `ValidateAndReserve` handler SHALL copy the service result's scope and shop into `ValidateAndReserveResponse.scope` and `seller_id` for a SERVICE principal holding `promotion.reserve` and for any other authenticated principal calling inside its own `preview:<principal id>:` namespace. The handler SHALL keep refusing a service principal without `promotion.reserve` and a caller outside its preview namespace with `PERMISSION_DENIED`, an anonymous caller with `UNAUTHENTICATED`, and an empty `reservation_id` with `INVALID_ARGUMENT`, and SHALL create no hold in those cases.

#### Scenario: The order service receives the scope
- **WHEN** the order service principal (scope `promotion.reserve`) reserves a valid shop voucher through the gRPC handler
- **THEN** the response carries `scope=VOUCHER_SCOPE_SHOP` and the voucher's `seller_id`
- **VERIFIED BY**: team-promotion `internal/handler` `TestValidateAndReserveHandlerEchoesScope`; platform-e2e `features/promo/voucher_scope.feature` "Shop voucher reports SHOP scope and its seller"

#### Scenario: A buyer preview receives the scope
- **WHEN** a buyer reserves a valid platform voucher with reservation id `preview:<buyer id>:abc`
- **THEN** the response has `valid=true`, `scope=VOUCHER_SCOPE_PLATFORM` and the hold is bound to the buyer id of the principal, not the request's `buyer_id`
- **VERIFIED BY**: team-promotion `internal/handler` `TestValidateAndReservePreviewEchoesScope`; platform-e2e `features/promo/voucher_scope.feature` "A buyer preview receives the scope"

#### Scenario: A buyer outside the preview namespace is denied
- **WHEN** a buyer reserves with reservation id `preview:<another user id>:abc`, or with an id that has no `preview:` prefix
- **THEN** the call fails `PERMISSION_DENIED` and no hold exists
- **VERIFIED BY**: team-promotion `internal/handler` `TestValidateAndReservePreviewNamespaceDenied`; platform-e2e `features/promo/voucher_scope.feature` "A buyer outside the preview namespace is denied"

#### Scenario: A service without the reserve scope is denied
- **WHEN** a service principal without scope `promotion.reserve` calls `ValidateAndReserve`
- **THEN** the call fails `PERMISSION_DENIED` and no hold exists
- **VERIFIED BY**: team-promotion `internal/handler` `TestValidateAndReserveServiceWithoutScopeDenied`

#### Scenario: An anonymous caller is unauthenticated
- **WHEN** a caller with no principal calls `ValidateAndReserve`
- **THEN** the call fails `UNAUTHENTICATED` and no hold exists
- **VERIFIED BY**: team-promotion `internal/handler` `TestValidateAndReserveAnonymousUnauthenticated`

#### Scenario: An empty reservation id is rejected
- **WHEN** the order service calls `ValidateAndReserve` with an empty `reservation_id`
- **THEN** the call fails `INVALID_ARGUMENT` and no hold exists
- **VERIFIED BY**: team-promotion `internal/handler` `TestValidateAndReserveEmptyReservationIDInvalid`

#### Scenario: A store failure is a generic internal error
- **WHEN** the voucher store fails during `ValidateAndReserve`
- **THEN** the call fails `INTERNAL` with a generic message that names no store detail
- **VERIFIED BY**: team-promotion `internal/handler` `TestValidateAndReserveStoreFailureInternal`
