## Purpose

Defines who may call `team-promotion`'s voucher redemption RPCs and who may create vouchers and flash sales, so that quota, holds and platform-wide promotions cannot be manipulated by anonymous or unprivileged callers.

## ADDED Requirements

### Requirement: Saga RPCs Commit and Release require service authority

`team-promotion` SHALL require a principal of type `service` holding the scope `promotion.reserve` on `CommitReservation`
and `ReleaseReservation`. A caller without a principal, or with the anonymous principal, SHALL receive `UNAUTHENTICATED`;
any other caller (buyer, seller, admin, or a service principal without the scope) SHALL receive `PERMISSION_DENIED`; in both cases no
reservation and no voucher quota SHALL change. The scope SHALL be carried only by the order service's own service principal
and SHALL NOT be grantable to a user role.

#### Scenario: Anonymous caller cannot commit a reservation

- **WHEN** a caller with no principal, then the anonymous principal, calls `CommitReservation` for an existing hold
- **THEN** both receive `UNAUTHENTICATED` and the voucher's `used` count is unchanged

#### Scenario: A signed-in user cannot release or commit

- **WHEN** a buyer, a seller and an admin each call `ReleaseReservation` and `CommitReservation` for an existing hold
- **THEN** every call is `PERMISSION_DENIED` and the hold's status is unchanged

#### Scenario: A service principal without the scope is rejected

- **WHEN** a service principal holding only `order.read` calls `CommitReservation`
- **THEN** the call is `PERMISSION_DENIED`

#### Scenario: A user principal carrying the scope string is rejected

- **WHEN** a principal of type `user` whose scopes include `promotion.reserve` calls `CommitReservation`
- **THEN** the call is `PERMISSION_DENIED` (the principal type is checked, not only the scope)

#### Scenario: The order service can commit and release

- **WHEN** `service-team-order` (type `service`, scope `promotion.reserve`) calls `CommitReservation` for a reserved hold and `ReleaseReservation` for another reserved hold
- **THEN** the first returns `committed=true` and increments the voucher's `used` by one, the second returns `released=true`

### Requirement: ValidateAndReserve binds the caller's identity

`team-promotion` SHALL reject `ValidateAndReserve` from a caller with no principal (`UNAUTHENTICATED`) or the anonymous
principal (`UNAUTHENTICATED`). When the caller is a service principal of type `service` holding `promotion.reserve`, the
request fields (`buyer_id`, `seller_id`, `cart_subtotal`, `reservation_id`) SHALL be honoured. When the caller is any other
authenticated principal, `team-promotion` SHALL use the principal's id as the buyer (ignoring any `buyer_id` in the
request) and SHALL accept only a `reservation_id` that begins with `preview:<principal id>:`; any other `reservation_id`
SHALL be `PERMISSION_DENIED` and no hold SHALL be created. A service principal without `promotion.reserve` SHALL receive
`PERMISSION_DENIED`.

#### Scenario: Anonymous caller cannot reserve

- **WHEN** a caller with the anonymous principal calls `ValidateAndReserve` with a valid code
- **THEN** the call is `UNAUTHENTICATED` and no hold is created

#### Scenario: A buyer cannot reserve as another buyer

- **WHEN** buyer A calls `ValidateAndReserve` with `buyer_id` = buyer B and `reservation_id` = `preview:<A id>:SAVE10`
- **THEN** the hold that is created (or returned) is attributed to buyer A, not B

#### Scenario: A buyer cannot use an id outside their preview namespace

- **WHEN** buyer A calls `ValidateAndReserve` with `reservation_id` = `preview:<B id>:SAVE10`, or with an order-style id
- **THEN** the call is `PERMISSION_DENIED` and no hold is created or returned

#### Scenario: The checkout preview keeps working

- **WHEN** a logged-in buyer applies a valid voucher at checkout (the frontend calls `ValidateAndReserve` with `reservation_id` = `preview:<own id>:<CODE>` and the buyer's own id)
- **THEN** the response is `valid=true` with the discount, exactly as before this change

#### Scenario: A user-path call cannot consume quota

- **WHEN** a buyer calls `ValidateAndReserve` successfully and then tries `CommitReservation` on the same id
- **THEN** the commit is `PERMISSION_DENIED` and the voucher's `used` count is unchanged

#### Scenario: The order service reserves for a buyer

- **WHEN** `service-team-order` calls `ValidateAndReserve` with `buyer_id` = buyer A, `reservation_id` = an order id and a cart subtotal
- **THEN** the hold is created for buyer A with the computed discount

### Requirement: Voucher creation is limited to sellers and admins, platform scope to admins

`team-promotion` SHALL require the scope `listing.write` on `CreateVoucher` and SHALL require the scope `admin` in addition
when the voucher scope is `PLATFORM`. A caller without a principal or with the anonymous principal SHALL receive
`UNAUTHENTICATED`; a signed-in caller lacking the required scope (a buyer, or a seller requesting `PLATFORM`) SHALL receive
`PERMISSION_DENIED` and no voucher SHALL be created. A `SHOP` voucher SHALL keep being bound to the creating principal's id.

#### Scenario: Buyer cannot create a voucher

- **WHEN** a buyer calls `CreateVoucher` (any scope)
- **THEN** the call is `PERMISSION_DENIED` and no voucher exists for the code

#### Scenario: Seller cannot create a platform voucher

- **WHEN** a seller calls `CreateVoucher` with scope `PLATFORM`
- **THEN** the call is `PERMISSION_DENIED` and no voucher exists for the code

#### Scenario: Seller creates a shop voucher bound to themselves

- **WHEN** a seller calls `CreateVoucher` with scope `SHOP`
- **THEN** the voucher is created with `seller_id` = the seller's id

#### Scenario: Admin creates a platform voucher

- **WHEN** an admin calls `CreateVoucher` with scope `PLATFORM`
- **THEN** the voucher is created with an empty `seller_id`

### Requirement: Flash sale campaigns are created only for listings the caller owns, or by an admin

`team-promotion` SHALL require `listing.write` on `CreateCampaign`. For a non-admin caller it SHALL additionally verify, by
reading the listing from `team-domain`, that the listing's seller is the caller; when ownership cannot be established
(listing not found, not owned, or `team-domain` unreachable or not configured) the call SHALL be rejected
(`PERMISSION_DENIED` for a listing owned by someone else or not found, `UNAVAILABLE` when `team-domain` cannot be reached)
and no campaign SHALL be created. An admin SHALL be allowed for any listing. A caller without a principal or with the
anonymous principal SHALL receive `UNAUTHENTICATED`.

#### Scenario: Seller creates a campaign on their own listing

- **WHEN** a seller calls `CreateCampaign` for a listing they created
- **THEN** the campaign is created and returned with an id

#### Scenario: Seller cannot create a campaign on another seller's listing

- **WHEN** seller A calls `CreateCampaign` for a listing created by seller B
- **THEN** the call is `PERMISSION_DENIED` and no campaign exists for that listing

#### Scenario: Buyer cannot create a campaign

- **WHEN** a buyer calls `CreateCampaign`
- **THEN** the call is `PERMISSION_DENIED`

#### Scenario: Ownership cannot be verified

- **WHEN** a seller calls `CreateCampaign` while `team-domain` is unreachable
- **THEN** the call is `UNAVAILABLE` and no campaign is created

### Requirement: The order service calls promotion as itself

`team-order` SHALL send its own service principal (`service-team-order`, type `service`, scope `promotion.reserve` only)
on `ValidateAndReserve`, `CommitReservation` and `ReleaseReservation`, and SHALL NOT forward the end user's principal on
those calls, whether the call is made inside a buyer's checkout request, by the `PaymentSettled` consumer, or by
compensation and cancellation.

#### Scenario: Checkout with a voucher places the order

- **WHEN** a buyer checks out a cart with a valid voucher code while `team-promotion` enforces the gate
- **THEN** the order is placed with the discount and the hold was created by `service-team-order`

#### Scenario: Paying the order commits the voucher

- **WHEN** the `PaymentSettled` event for that order is consumed
- **THEN** `CommitReservation` succeeds as the service principal and the voucher's `used` increments by one

#### Scenario: Cancelling the order releases the hold

- **WHEN** the buyer cancels the unpaid order
- **THEN** `ReleaseReservation` succeeds as the service principal and the hold is `released`

### Requirement: Voucher browsing stays public

`ListVouchers` and `GetVoucher` SHALL remain callable without a principal so the public Vouchers Hub keeps working.

#### Scenario: Visitor browses vouchers

- **WHEN** an anonymous caller calls `ListVouchers` with an empty `seller_id`
- **THEN** the platform vouchers are returned
