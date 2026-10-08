## Purpose

Defines which order status changes are legal and who may make them, how `team-order` enforces them atomically under concurrency, how a checkout retry avoids creating a second order, and how the service refuses to run on in-memory storage in staging and production.

## ADDED Requirements

### Requirement: Order status changes follow an allowed-transition table

`team-order` SHALL accept an order status change only if it is in the allowed table: `Pending -> Paid`,
`Pending -> Cancelled`, `Paid -> Shipped`, `Paid -> Cancelled`, `Shipped -> Completed`. `Completed` and
`Cancelled` SHALL be terminal. Any other requested change, including a change to the current status, SHALL be
rejected with `FAILED_PRECONDITION` and SHALL leave the order unchanged. The table SHALL be applied on every
write path: the `UpdateOrderStatus` RPC, `CancelOrder`, the `PaymentSettled` consumer and shipment creation.

#### Scenario: Completed order cannot be reopened

- **WHEN** a caller requests `UpdateOrderStatus` to `Pending` on a `Completed` order
- **THEN** the call fails with `FAILED_PRECONDITION` and the order is still `Completed`

#### Scenario: A legal transition succeeds

- **WHEN** a seller requests `UpdateOrderStatus` to `Shipped` on a `Paid` order
- **THEN** the order becomes `Shipped`

#### Scenario: Skipping a step is rejected

- **WHEN** a caller requests `UpdateOrderStatus` to `Completed` on a `Paid` order
- **THEN** the call fails with `FAILED_PRECONDITION` and the order is still `Paid`

#### Scenario: Terminal states stay terminal

- **WHEN** a caller requests any status change on a `Cancelled` order
- **THEN** the call fails with `FAILED_PRECONDITION` and the order is still `Cancelled`

#### Scenario: Shipment creation does not hide a failed status update

- **WHEN** a shipment is created for an order whose status cannot move to `Shipped`
- **THEN** the failure is returned or recorded as an error, not discarded, and the order status is unchanged

### Requirement: Buyers may only cancel

A buyer SHALL be able to move their own order only to `Cancelled`, and only from `Pending` or `Paid`. Every
other transition SHALL require the order's seller or a system caller, except that `Pending -> Paid` SHALL be
written only by a system caller (a settled payment), never by the seller or the buyer. A caller who is neither
the buyer nor the seller of the order SHALL be rejected with `PERMISSION_DENIED`.

#### Scenario: Seller cannot mark an order paid

- **WHEN** the seller requests `UpdateOrderStatus` to `Paid` on their `Pending` order
- **THEN** the call fails with `PERMISSION_DENIED` and the order is still `Pending`

#### Scenario: A user cannot buy their own listing

- **WHEN** a buyer checks out a cart containing a listing they sell
- **THEN** the checkout fails with `FAILED_PRECONDITION` before any stock is reserved and no idempotency key is held

#### Scenario: Buyer cannot mark their own order completed

- **WHEN** the buyer requests `UpdateOrderStatus` to `Completed` on their `Shipped` order
- **THEN** the call fails with `PERMISSION_DENIED` and the order is still `Shipped`

#### Scenario: Buyer can cancel a pending order

- **WHEN** the buyer cancels their own `Pending` order
- **THEN** the order becomes `Cancelled`

#### Scenario: Other users are rejected

- **WHEN** a user who is neither buyer nor seller requests any status change
- **THEN** the call fails with `PERMISSION_DENIED`

### Requirement: Status changes are atomic compare-and-set

`team-order` SHALL apply a status change only if the order is still in one of the statuses the transition
allows at the moment of the write, decided inside the database write itself and not from an earlier read. When
two callers race, at most one SHALL succeed and the other SHALL receive `FAILED_PRECONDITION`. The database
SHALL also reject any `orders.status` value outside the defined set.

#### Scenario: Concurrent cancels release stock once

- **WHEN** two `CancelOrder` calls for the same `Pending` order run at the same time
- **THEN** exactly one succeeds, the other fails with `FAILED_PRECONDITION`, and the order's stock is released once

#### Scenario: Cancel racing shipment

- **WHEN** a seller ships a `Paid` order while the buyer cancels it concurrently
- **THEN** exactly one of the two transitions is applied and the final status is either `Shipped` or `Cancelled`, never a mix

#### Scenario: An undefined status cannot be stored

- **WHEN** any statement would set `orders.status` to a value outside `Pending..Cancelled`
- **THEN** the database rejects it

### Requirement: Cancelling releases stock only for the winning cancel

`CancelOrder` SHALL first claim the `Cancelled` transition atomically and only the caller that wins the claim
SHALL release the order's reservations. If releasing a reservation fails, the failure SHALL be recorded
durably so it is retried, and the order SHALL still be `Cancelled`. An order left `Cancelled` with
reservations that still hold stock SHALL be reconciled by the existing reservation sweep.

#### Scenario: Paid order cancel releases its stock

- **WHEN** a buyer cancels a `Paid` order
- **THEN** the order becomes `Cancelled` and its reservations are released once

#### Scenario: Failed release after claim is retried

- **WHEN** the cancel claim succeeds and the release call then fails
- **THEN** the order is `Cancelled`, the failed release is recorded as retryable, and a later sweep releases the stock

#### Scenario: A rejected cancel releases nothing

- **WHEN** a buyer cancels a `Shipped` order
- **THEN** the call fails with `FAILED_PRECONDITION` and no reservation is released

### Requirement: Stale payment events do not override order state

The `PaymentSettled` consumer SHALL move an order to `Paid` only when the order is still `Pending` at the
moment of the write. A settled event for an order in any other status (cancelled, already paid, shipped)
SHALL be acknowledged without changing the order and SHALL be logged with the order id and current status.
Redelivery of the same event SHALL remain a no-op.

#### Scenario: Late payment success after cancel

- **WHEN** `PaymentSettled` arrives for an order that was already `Cancelled`
- **THEN** the order stays `Cancelled`, the event is acknowledged, and an ignored-event log entry exists

#### Scenario: Payment racing cancel

- **WHEN** `PaymentSettled` and `CancelOrder` run concurrently on a `Pending` order
- **THEN** the order ends `Cancelled` in either interleaving (payment ignored, or cancel applied from `Paid`), its stock is released once, and no `Paid` order exists without its stock

#### Scenario: Redelivered payment event

- **WHEN** the same `PaymentSettled` event is delivered twice for a `Pending` order
- **THEN** the order is `Paid` after the first and unchanged after the second

### Requirement: Checkout is idempotent on the client's idempotency key

`CreateOrder` SHALL accept an `Idempotency-Key` supplied by the client as request metadata. For a given buyer,
the first request with a key SHALL create the orders; any later request with the same key SHALL return the
orders already created and SHALL NOT create, reserve stock for, or charge another order. A request whose first
attempt is still in progress SHALL fail with a retryable `ABORTED` rather than start a second saga. A key whose
saga failed and was compensated SHALL be reusable. A key SHALL be scoped to the buyer, so two buyers may use
the same value independently. A request without a key SHALL behave as before. A key longer than 128
characters or containing non-printable characters SHALL be rejected with `INVALID_ARGUMENT`.

#### Scenario: Retried checkout returns the same orders

- **WHEN** a buyer submits checkout twice with the same key and the same cart
- **THEN** both calls return the same order ids and exactly one order exists

#### Scenario: Stock is decremented once per key

- **WHEN** checkout is submitted twice with the same key for quantity 2 against stock 10
- **THEN** stock is 8

#### Scenario: Cart clear failure does not duplicate the order

- **WHEN** the orders are created but removing the checked-out items from the cart fails, and the client retries with the same key
- **THEN** the retry returns the original orders, re-attempts the cart clear, and no second order exists

#### Scenario: Concurrent duplicate requests

- **WHEN** two checkout requests with the same key arrive at the same time
- **THEN** one creates the orders and the other either returns them or fails with a retryable `ABORTED`, and only one set of orders exists

#### Scenario: Failed checkout frees the key

- **WHEN** a checkout with key K fails and is compensated, and the buyer retries with key K
- **THEN** the retry runs as a fresh checkout

#### Scenario: Keys are per buyer

- **WHEN** two different buyers check out with the same key value
- **THEN** each gets their own orders

#### Scenario: Malformed key is rejected

- **WHEN** a checkout request carries a key longer than 128 characters
- **THEN** the call fails with `INVALID_ARGUMENT` and nothing is reserved

### Requirement: The edge and the UI only carry the key

`team-gateway` SHALL forward the client's `Idempotency-Key` header to `team-order` as gRPC metadata on
`CreateOrder` and SHALL NOT store, compare, or generate keys. `team-frontend` SHALL generate one key per
checkout attempt and send it on submit; resubmitting the same attempt, such as a double click or a retry after
a timeout, SHALL reuse that key, and a new attempt after a completed checkout SHALL use a new key.

#### Scenario: Double click places one order

- **WHEN** the buyer double-clicks the place-order button
- **THEN** exactly one order is created

#### Scenario: Gateway forwards without deciding

- **WHEN** the gateway receives `CreateOrder` with an `Idempotency-Key` header
- **THEN** `team-order` receives the same value as metadata and the gateway holds no key state

### Requirement: Staging and production refuse to run on in-memory storage

`team-order` SHALL fail to start, with a clear error, when its `ENV` is `staging` or `production` and the
database is disabled or unavailable. In-memory repositories SHALL be available only when `ENV` is `local` or
`test`. When a database is configured, the saga repository, payment consumer and reservation sweeper SHALL run
as before.

#### Scenario: Production without a database does not boot

- **WHEN** the service starts with `ENV=production` and `DATABASE_ENABLED=false`
- **THEN** startup fails with an error naming the environment and the database setting, and no port is opened

#### Scenario: Staging without a database does not boot

- **WHEN** the service starts with `ENV=staging` and `DATABASE_ENABLED=false`
- **THEN** startup fails with the same error

#### Scenario: Local development may use memory

- **WHEN** the service starts with `ENV=local` and `DATABASE_ENABLED=false`
- **THEN** it starts with in-memory repositories and logs a warning that data is not durable

#### Scenario: Production with a database boots normally

- **WHEN** the service starts with `ENV=production` and a reachable database
- **THEN** it starts with Postgres repositories, the sweeper and the payment consumer
