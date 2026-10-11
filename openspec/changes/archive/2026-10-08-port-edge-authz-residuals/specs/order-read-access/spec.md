## MODIFIED Requirements

### Requirement: ForceFailSaga validates its step and reports what it did

`ForceFailSaga` SHALL be callable only by a principal holding the `admin` scope; any other caller SHALL get
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
