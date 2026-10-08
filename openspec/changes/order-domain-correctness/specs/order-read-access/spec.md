## Purpose

Defines who may read an order, its shipment and its saga view in `team-order`, and requires the saga view and the force-fail helper to describe what actually happened instead of a simulated story.

## ADDED Requirements

### Requirement: Order reads require a principal and ownership

`GetOrder` and `GetShipmentTracking` SHALL reject a call that carries no authenticated principal with
`UNAUTHENTICATED` (the gateway's anonymous principal is treated as no principal). An authenticated caller SHALL be
allowed to read an order only when they are its buyer, its seller or an admin, using one shared rule across every
order RPC that accepts admins; any other caller SHALL receive `PERMISSION_DENIED`. `GetShipmentTracking` SHALL apply
the same rule whichever identifier (tracking code, order id or shipment id) is used. Every order RPC other than
`CalculateShippingFee` SHALL require a principal.

`GetOrder` SHALL additionally allow an internal service principal (`x-principal-type: service`) that holds the
service-only scope `order.read` to read any order, so that `team-payment` can read the order it is paying. The scope
SHALL be read-only (it widens no other RPC) and SHALL be granted to no identity role; only the principal type together
with the scope qualifies, so a user or anonymous principal carrying the scope string is treated like any other caller.

#### Scenario: GetOrder without a principal is rejected

- **WHEN** `GetOrder` is called with no principal
- **THEN** the call fails with `UNAUTHENTICATED` and no order data is returned

#### Scenario: The buyer and the seller can read the order

- **WHEN** the buyer, then the seller, of an order call `GetOrder`
- **THEN** both receive the order

#### Scenario: An admin can read any order

- **WHEN** an admin calls `GetOrder` for another user's order
- **THEN** the order is returned

#### Scenario: A service principal with order.read can read any order

- **WHEN** a `service` principal holding `order.read` calls `GetOrder` for an order it does not own
- **THEN** the order is returned; a missing order is still `NOT_FOUND`

#### Scenario: A service principal without order.read is denied

- **WHEN** a `service` principal that lacks `order.read`, or a user principal that merely carries the `order.read` scope, calls `GetOrder` for another party's order
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: Another user cannot read the order

- **WHEN** a user who is neither buyer nor seller nor admin calls `GetOrder`
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: Shipment tracking is owner-scoped whichever identifier is used

- **WHEN** a user who is neither buyer nor seller nor admin calls `GetShipmentTracking` with the tracking code, order id or shipment id of another order
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: Every order RPC requires a principal

- **WHEN** each order RPC except `CalculateShippingFee` is called with no principal
- **THEN** each fails with `UNAUTHENTICATED`

### Requirement: The saga view reflects persisted state

`GetSagaState` SHALL build its steps from the order's persisted status and from its saga and reservation records,
and SHALL NOT report a step as succeeded unless it happened. An order still awaiting payment SHALL show the payment
step as pending; a cancelled order SHALL show the compensation step according to whether its reservations are
actually released or still awaiting a retry. A step whose time is not recorded SHALL be returned without a
timestamp rather than with an invented one. The view SHALL be readable only by the order's buyer or an admin.

#### Scenario: A pending order does not show payment as charged

- **WHEN** `GetSagaState` is called for a `Pending` order
- **THEN** the order-created and stock-reserved steps are successful and the payment and confirmation steps are pending, with `is_compensated` false

#### Scenario: A paid order shows payment as settled

- **WHEN** `GetSagaState` is called for a `Paid` order
- **THEN** the payment step is successful and no compensation is reported

#### Scenario: A cancelled order with released stock shows completed compensation

- **WHEN** `GetSagaState` is called for a `Cancelled` order whose reservations are all released
- **THEN** `is_compensated` is true and the compensation step is reported as compensated

#### Scenario: A cancelled order with a parked release shows compensation still pending

- **WHEN** `GetSagaState` is called for a `Cancelled` order with a reservation still awaiting release retry
- **THEN** the compensation step is reported as in progress, not compensated

#### Scenario: Unrecorded times are not invented

- **WHEN** a step has no recorded time
- **THEN** its timestamp is absent

#### Scenario: Another user cannot read the saga view

- **WHEN** a user who is neither the buyer nor an admin calls `GetSagaState`
- **THEN** the call fails with `PERMISSION_DENIED`

### Requirement: ForceFailSaga reports what it did

`ForceFailSaga` SHALL validate `fail_step` (empty, `payment` or `shipping`; any other value is `INVALID_ARGUMENT`
and leaves the order unchanged), cancel the order through the normal cancel path, and report `success=true` only when
the order's reservations were released. When the order was cancelled but a release is still awaiting retry it SHALL
return `success=false` with a message saying so, and the returned saga view SHALL be the truthful one.

#### Scenario: Unknown fail_step is rejected

- **WHEN** `ForceFailSaga` is called with `fail_step` "banana"
- **THEN** the call fails with `INVALID_ARGUMENT` and the order is unchanged

#### Scenario: A clean force-fail reports success

- **WHEN** `ForceFailSaga` cancels a `Pending` order and its stock is released
- **THEN** `success` is true and the saga view shows completed compensation

#### Scenario: A parked release is not reported as success

- **WHEN** `ForceFailSaga` cancels an order but the stock release fails and is parked
- **THEN** the order is `Cancelled`, `success` is false, and the message states the release is pending retry

#### Scenario: A non-cancellable order is refused

- **WHEN** `ForceFailSaga` is called for a `Shipped` order
- **THEN** the call fails with `FAILED_PRECONDITION` and the order is unchanged
