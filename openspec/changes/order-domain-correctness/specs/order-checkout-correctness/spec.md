## Purpose

Defines how `team-order` turns a cart into orders without ever returning the stock of a placed order, without leaving a half-placed multi-seller checkout, and with truthful answers when stock cannot be reserved. It also fixes what cancelling an order does to its voucher hold.

## ADDED Requirements

### Requirement: A checkout's orders and their reservations are placed atomically

`team-order` SHALL persist all orders of one checkout and mark all of their reservations as belonging to those
orders in a single database transaction: either every order exists with every reservation bound to it, or none of
them exists. There SHALL be no observable state in which an order exists while one of its reservations still
counts as un-ordered stock. A reservation SHALL carry the id of the order it is reserved for from the moment it is
created, so that a stale reservation can be told apart from an abandoned one.

#### Scenario: Order and reservation binding commit together

- **WHEN** a buyer checks out one item and the order is persisted
- **THEN** the order exists and its reservation is bound to that order in the same committed transaction

#### Scenario: A failed binding leaves no order and returns the stock

- **WHEN** the database write that binds the reservations to the order fails during placement
- **THEN** no order exists for the checkout, the stock held by its reservations is released, and the buyer receives an error

#### Scenario: The sweep never releases the stock of an order that exists

- **WHEN** the reservation sweep finds a reservation past its TTL whose intended order exists and is not cancelled
- **THEN** the reservation is repaired to belong to that order, `ReleaseStock` is not called for it, and the stock stays held

#### Scenario: The sweep still releases a reservation whose order was never placed

- **WHEN** the reservation sweep finds a reservation past its TTL whose intended order does not exist
- **THEN** the reservation is released and its stock returns

#### Scenario: An ambiguous placement outcome never releases a placed order's stock

- **WHEN** the order transaction returns an error but the orders were in fact committed
- **THEN** the checkout is treated as placed (the orders are returned) and no reservation is released

### Requirement: A multi-seller checkout is all-or-nothing

When a checkout spans several sellers, `team-order` SHALL reserve and confirm the stock of every seller group before
creating any order, and SHALL create all orders together. If any seller group cannot be reserved or confirmed, the
checkout SHALL fail with no order created for any seller, every stock hold released, the cart left unchanged, and any
idempotency key reusable. A retry with the same key after such a failure SHALL create exactly one order per seller,
never a duplicate of an earlier partial result.

#### Scenario: One seller out of stock fails the whole checkout

- **WHEN** a buyer checks out items from two sellers and the second seller's item cannot be reserved
- **THEN** the call fails, no order exists for either seller, the first seller's stock is released, and the cart still contains both items

#### Scenario: A retry after a failed multi-seller checkout does not duplicate

- **WHEN** the failed multi-seller checkout is retried with the same idempotency key after stock became available
- **THEN** exactly one order per seller is created and the cart is emptied

#### Scenario: A successful multi-seller checkout places every order

- **WHEN** a buyer checks out items from two sellers and all can be reserved
- **THEN** one order per seller is created, the cart is emptied, and a repeat with the same key returns the same orders

### Requirement: An unkeyed checkout retry can reserve again after a compensated attempt

For a checkout without an `Idempotency-Key`, every attempt SHALL use reservation ids that no earlier attempt used, so
a retry after an attempt that was compensated (its stock released) reserves stock normally instead of being refused
because of a previously released reservation id.

#### Scenario: Retry after a compensated attempt succeeds

- **WHEN** an unkeyed checkout fails and is compensated, stock is available again, and the buyer checks out the same cart again
- **THEN** the second checkout creates the order and decrements stock once

#### Scenario: Two unkeyed attempts never share a reservation

- **WHEN** the same cart item is checked out by two separate unkeyed attempts
- **THEN** each attempt holds its own reservation and stock is decremented once per attempt

### Requirement: Stock reservation outcomes are reported truthfully

`team-order` SHALL classify the answer of the stock reservation step by what it means. An explicit "insufficient
stock" answer SHALL fail the checkout as `RESOURCE_EXHAUSTED` before any order is placed. A refused reservation
(`FAILED_PRECONDITION`, `NOT_FOUND`) SHALL fail as `FAILED_PRECONDITION` (item no longer available), and an
unreachable or erroring stock service SHALL fail as `UNAVAILABLE`, never as insufficient stock. In every failure the
checkout SHALL create no order and release whatever it had reserved.

#### Scenario: Insufficient stock is ResourceExhausted

- **WHEN** a buyer checks out a quantity larger than the available stock
- **THEN** the call fails with `RESOURCE_EXHAUSTED`, no order exists, and stock is unchanged

#### Scenario: A refused reservation is not reported as insufficient stock

- **WHEN** the stock service refuses the reservation with `FAILED_PRECONDITION`
- **THEN** the call fails with `FAILED_PRECONDITION` and no order exists

#### Scenario: An unreachable stock service is Unavailable

- **WHEN** the stock service cannot be reached while reserving
- **THEN** the call fails with `UNAVAILABLE`, no order exists, and earlier reservations of the checkout are released

#### Scenario: An out-of-stock item never reaches order placement

- **WHEN** the stock service answers "not reserved" without an error
- **THEN** the checkout stops at the reservation step and never confirms a reservation or creates an order for that item

### Requirement: Cancelling an order releases its voucher hold

After it wins the cancel claim, `CancelOrder` SHALL release the voucher hold that was placed for the order at
checkout, by every path that cancels (the `CancelOrder` RPC, `UpdateOrderStatus` to `Cancelled`, and `ForceFailSaga`).
Releasing the hold SHALL be best-effort and idempotent: a failure SHALL be logged and SHALL NOT fail the cancel or
prevent the stock release. A redemption already committed by a settled payment is not reversed by this change.

#### Scenario: Cancelling a pending order with a voucher releases the hold

- **WHEN** a buyer cancels their `Pending` order that carries a voucher
- **THEN** the order becomes `Cancelled` and the voucher hold placed for it is released

#### Scenario: Cancelling an order without a voucher does not touch promotion

- **WHEN** a buyer cancels an order that has no voucher
- **THEN** no voucher release is attempted

#### Scenario: A failed voucher release does not block the cancel

- **WHEN** the promotion service cannot be reached while a voucher order is cancelled
- **THEN** the order is still `Cancelled`, its stock is released, and the failure is logged

#### Scenario: A losing cancel releases no voucher

- **WHEN** a second cancel of the same order loses the cancel claim
- **THEN** it fails with `FAILED_PRECONDITION` and no voucher release is attempted by it

#### Scenario: A committed redemption stays consumed on cancel

- **WHEN** a `Paid` order whose voucher was committed at settlement is cancelled
- **THEN** the order is `Cancelled`, its stock is released once, and the committed redemption is unchanged
