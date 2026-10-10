## MODIFIED Requirements

### Requirement: ForceFailSaga validates its step and reports what it did

`ForceFailSaga` SHALL be callable only by a principal holding both the `admin` scope and the `order.admin` scope; any
other caller (including an owner, a buyer, a SERVICE principal, and an admin token that lacks `order.admin`) SHALL get
`PERMISSION_DENIED` and the order SHALL be unchanged. Buyers cancel through `CancelOrder`. `ForceFailSaga` SHALL accept
`fail_step` empty, `payment` or `shipping` and reject any other value with `INVALID_ARGUMENT`, leaving the order
unchanged. It SHALL cancel through the normal cancel path (claim, stock release, voucher release) and answer
`success=true` only when the order's reservations were all released; when the order was cancelled but a release is
parked for retry it SHALL answer `success=false` with a message saying the release is pending retry. The returned saga
view SHALL be the persisted one. An order that cannot be cancelled SHALL fail with `FAILED_PRECONDITION`.

#### Scenario: An unknown fail_step is rejected

- **WHEN** the admin calls `ForceFailSaga` with `fail_step` "banana" on a buyer's `Pending` order
- **THEN** the call fails with `invalid_argument` and the order is still `Pending` with its stock still held

#### Scenario: A clean force-fail reports success

- **WHEN** the admin calls `ForceFailSaga` with `fail_step` "payment" on a buyer's `Pending` order for quantity 2 of a
  listing with stock 8
- **THEN** `success` is true, the order is `Cancelled`, the returned saga view is compensated, and the stock is 10

#### Scenario: A force-fail with a parked release reports failure

- **WHEN** the admin calls `ForceFailSaga` on a buyer's `Pending` order while `team-domain` is stopped
- **THEN** `success` is false, the message says the stock release is pending retry, and the order is `Cancelled`

#### Scenario: Force-failing a shipped order is refused

- **WHEN** the admin calls `ForceFailSaga` on a buyer's `Shipped` order
- **THEN** the call fails with `failed_precondition` and the order is still `Shipped`

#### Scenario: An admin token without order.admin cannot force-fail

- **WHEN** a caller whose token carries the `admin` scope but not `order.admin` calls `ForceFailSaga` on a buyer's
  `Pending` order
- **THEN** the call fails with `permission_denied` and the order is still `Pending` with its stock still held

## ADDED Requirements

### Requirement: Admin overrides on order RPCs require order.admin

Wherever an order RPC lets an admin act on an order that is not theirs (`GetOrder`, `GetSagaState`, `UpdateOrderStatus`,
`CreateShipment`, the return-request RPCs), team-order SHALL recognise the admin by the `order.admin` scope only. The
bare `admin` scope SHALL NOT open an order. The order's own buyer or seller keeps their access, and a SERVICE principal
holding `order.read` keeps `GetOrder`.

#### Scenario: An admin token without order.admin cannot read another user's saga

- **WHEN** a caller whose token carries `admin` but not `order.admin` calls `GetSagaState` on a buyer's order
- **THEN** the call fails with `permission_denied`

#### Scenario: The admin can still read another user's saga

- **WHEN** the seeded admin calls `GetSagaState` on a buyer's order
- **THEN** the call succeeds and returns that order's saga view

#### Scenario: A buyer cannot force-fail another buyer's order

- **WHEN** buyer B calls `ForceFailSaga` on buyer A's `Pending` order
- **THEN** the gateway answers 403 and A's order is still `Pending`
