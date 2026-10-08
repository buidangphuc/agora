## Purpose

Defines how `team-order` turns a cart into orders: every order of a checkout or none, each attempt holding its own
stock, a client-supplied idempotency key that makes a retried checkout return the same orders, and a storefront that
sends one key per checkout attempt.

## ADDED Requirements

### Requirement: A checkout places all of its orders or none

`team-order` SHALL reserve and commit the stock of every seller group of a checkout before creating any order, processing
seller groups in a deterministic order, and SHALL then create every order of the checkout and bind each reservation to
its order in a single `order_db` transaction. If any seller group cannot be reserved or committed, or the transaction
fails, the checkout SHALL fail with no order created for any seller, every stock and voucher hold it took released, and
the cart unchanged. If the transaction outcome is ambiguous (an error after a possible commit), `team-order` SHALL look the
pre-assigned order ids up before compensating: all present → return them as placed; none present → compensate; anything
else → fail with `INTERNAL` without releasing stock. An insufficient-stock refusal SHALL fail with `RESOURCE_EXHAUSTED`,
and a commit refused because the reservation was already released SHALL fail with `FAILED_PRECONDITION`.

#### Scenario: One seller out of stock fails the whole two-seller checkout

- **WHEN** a buyer checks out a cart with one item from seller A (stock available) and one item from seller B whose
  quantity exceeds B's stock
- **THEN** the call fails with `resource_exhausted`, the buyer has no new order from either seller, seller A's listing
  stock is back to its value before the checkout, and the cart still holds both items

#### Scenario: A two-seller checkout places one order per seller

- **WHEN** a buyer checks out a cart with in-stock items from two sellers
- **THEN** exactly two `Pending` orders are returned, one per seller, each listing's stock is reduced by its quantity, and
  the checked-out items are removed from the cart

### Requirement: Every checkout attempt holds its own reservations

`team-order` SHALL derive each reservation id from the checkout attempt (its saga) and the cart item, so two attempts
never share a reservation and an attempt that follows a compensated one reserves stock normally instead of hitting a
released reservation. The idempotency key SHALL NOT be part of the reservation id. A checkout attempt left in progress
past the reservation TTL (crash) SHALL be settled by the `team-order` sweep: its held reservations released, the attempt
marked compensated and its idempotency key freed.

#### Scenario: An unkeyed retry after a failed checkout succeeds

- **WHEN** a buyer's two-seller checkout without an `Idempotency-Key` fails because seller B is out of stock, seller B
  restocks the listing, and the buyer checks out the same cart again without a key
- **THEN** the second checkout returns one order per seller and seller A's listing stock is reduced by its quantity once

### Requirement: Checkout is idempotent on the client's idempotency key

`CreateOrder` SHALL read the `idempotency-key` request metadata forwarded by the gateway (rejecting a value that is not
1–255 printable ASCII bytes with `INVALID_ARGUMENT` before reserving anything). For one buyer, the first request with a
key SHALL run the checkout; a later request with the same key SHALL return the orders of the first request without
reserving stock, creating an order or touching the voucher again, and SHALL re-attempt the cart clear. A request whose
first attempt is still running SHALL fail with `ABORTED` (retryable). A key whose checkout failed and was compensated
SHALL be reusable for a fresh checkout. Keys SHALL be scoped to the buyer. A request without a key SHALL run a fresh
checkout every time.

#### Scenario: The same idempotency key returns the same orders

- **WHEN** a buyer calls `CreateOrder` twice with the same `Idempotency-Key` for a cart with quantity 2 of a listing with
  stock 10
- **THEN** both calls return the same order ids, the buyer has exactly one new order, and the listing's stock is 8

#### Scenario: Concurrent checkouts with one key create one set of orders

- **WHEN** a buyer sends two `CreateOrder` calls with the same `Idempotency-Key` at the same time
- **THEN** each call either returns the same order ids or fails with `aborted`, at least one returns orders, and exactly
  one order exists with its stock reduced once

#### Scenario: A failed checkout frees its idempotency key

- **WHEN** a buyer's checkout with key K fails because an item is out of stock, the seller restocks it, and the buyer
  checks out with key K again
- **THEN** the second call creates the order

#### Scenario: Idempotency keys are scoped to the buyer

- **WHEN** two different buyers each check out with the same `Idempotency-Key` value
- **THEN** each buyer receives their own new order

### Requirement: The storefront sends one idempotency key per checkout attempt

`team-frontend` SHALL generate a fresh random key for each checkout attempt, send it as the `Idempotency-Key` header on
the gateway `CreateOrder` call, and reuse it for every resubmission of the same attempt (double click, retry after an
error or a timeout, a replayed submission). It SHALL start a new key once a checkout succeeds or when the submitted
address, payment method, voucher or items change. An `aborted` answer SHALL be shown as a checkout still in progress that
the buyer can retry.

#### Scenario: A replayed checkout submission creates no second order

- **WHEN** a buyer places an order on the checkout page and the same checkout submission is sent a second time
- **THEN** the buyer has exactly one new order and the listing's stock is reduced once

#### Scenario: A new checkout after a completed one creates a new order

- **WHEN** a buyer completes a checkout, adds the listing to the cart again and places a second order from the checkout
  page
- **THEN** the buyer has two distinct new orders
