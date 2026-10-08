# order-read-access Specification

## Purpose
Requires `team-order`'s saga view and its force-fail helper to describe what actually happened to an order, built from
persisted order, reservation and payment facts instead of a simulated story.

## Requirements

### Requirement: The saga view reflects persisted state

`GetSagaState` SHALL build its steps from the order's persisted status, its reservation records and its `paid_at`, and
SHALL NOT report a step as succeeded unless it happened. Step statuses SHALL be `SUCCESS`, `PENDING`, `SKIPPED` or
`COMPENSATED`: order created and stock reserved are `SUCCESS`; payment is `PENDING` while the order is `Pending`,
`SUCCESS` once it reached `Paid` (with the `paid_at` time when recorded), and `SKIPPED` for an order cancelled before
payment; confirmation (non-cancelled orders) is `PENDING` while `Pending`, else `SUCCESS`; compensation (cancelled orders)
is `COMPENSATED` with `is_compensated` true only when every reservation of the order is released, and `PENDING` while any
release awaits retry. A step whose time is not recorded SHALL carry no timestamp. The compensation reason SHALL be the
neutral "order cancelled". Access stays buyer-or-admin.

#### Scenario: A pending order's saga view shows payment pending

- **WHEN** the buyer reads the saga view of their `Pending` order
- **THEN** the created and reserved steps are `SUCCESS`, the payment and confirmation steps are `PENDING`, and
  `is_compensated` is false

#### Scenario: A paid order's saga view shows the payment time

- **WHEN** the buyer reads the saga view of an order whose online payment succeeded
- **THEN** the payment step is `SUCCESS` with a timestamp and no compensation is reported

#### Scenario: A cancelled order with released stock shows completed compensation

- **WHEN** the buyer reads the saga view of their `Pending` order after cancelling it
- **THEN** the payment step is `SKIPPED`, the compensation step is `COMPENSATED`, and `is_compensated` is true

#### Scenario: A cancelled order awaiting a stock release shows compensation pending

- **WHEN** the buyer reads the saga view of an order cancelled while `team-domain` was stopped, before it is started again
- **THEN** the compensation step is `PENDING` and `is_compensated` is false

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
