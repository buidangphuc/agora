# order-lifecycle-guards Specification

## Purpose
Defines which order status changes are legal and who may make them, how `team-order` enforces them atomically under
concurrency on every write path, what a cancel does to stock and vouchers, and how late payment events are handled.

## Requirements

### Requirement: Order status changes follow one transition table with actor classes

`team-order` SHALL accept a status change only if it is in this table, made by an allowed actor class:

| From | To | Actor |
|---|---|---|
| `Pending` | `Paid` | system: the payment-settlement consumer only |
| `Pending` | `Shipped` | seller (cash-on-delivery hand-over) |
| `Paid` | `Shipped` | seller |
| `Shipped` | `Completed` | seller |
| `Pending`, `Paid` | `Cancelled` | buyer (`CancelOrder`); admin (`ForceFailSaga`) |

`Completed` and `Cancelled` SHALL be terminal. An admin SHALL act with the seller's class on `UpdateOrderStatus`. A
caller who is not the order's buyer, seller or an admin SHALL get `PERMISSION_DENIED` before any status is revealed; a
target the caller's class may never request (for example `Paid`, or `Cancelled` through `UpdateOrderStatus`) SHALL get
`PERMISSION_DENIED`; a target the class may request but not from the current status SHALL get `FAILED_PRECONDITION`. A
rejected change SHALL leave the order unchanged.

#### Scenario: A completed order cannot be reopened

- **WHEN** the seller calls `UpdateOrderStatus` to `Pending` on their `Completed` order
- **THEN** the call fails and the order read through the gateway is still `Completed`

#### Scenario: A seller cannot mark an order paid

- **WHEN** the seller calls `UpdateOrderStatus` to `Paid` on their `Pending` order
- **THEN** the call fails with `permission_denied` and the order is still `Pending`

#### Scenario: Skipping from paid straight to completed is refused

- **WHEN** the seller calls `UpdateOrderStatus` to `Completed` on their `Paid` order
- **THEN** the call fails with `failed_precondition` and the order is still `Paid`

#### Scenario: The seller ships a paid order

- **WHEN** the seller calls `UpdateOrderStatus` to `Shipped` on their `Paid` order
- **THEN** the order is `Shipped`

#### Scenario: A stranger cannot change an order's status

- **WHEN** a buyer who is neither the order's buyer nor its seller calls `UpdateOrderStatus` on it
- **THEN** the call fails with `permission_denied` and the order is unchanged

### Requirement: Status changes are compare-and-set

Every status write (`UpdateOrderStatus`, `CancelOrder`, `ForceFailSaga`, `CreateShipment` and the payment consumer)
SHALL apply only if the order is still in one of the statuses the transition allows at the moment of the write, decided
by the write itself and not by an earlier read. Of two racing conflicting writes exactly one SHALL succeed and the other
SHALL fail with `FAILED_PRECONDITION` (or, for the consumer, be ignored). Writes that already emit an outbox event with
the transition (`Paid`, `Shipped`) SHALL keep doing so in the same transaction. The database SHALL reject any
`orders.status` outside `Pending..Cancelled` (`orders_status_check`).

#### Scenario: Concurrent cancels restore stock once

- **WHEN** a buyer sends two `CancelOrder` calls for the same `Pending` order (quantity 2, listing stock 8) at the same time
- **THEN** exactly one succeeds, the other fails with `failed_precondition`, and the listing's stock is 10

#### Scenario: A cancel racing a shipment applies exactly one of them

- **WHEN** the seller ships a `Paid` order while its buyer cancels it at the same time
- **THEN** exactly one call succeeds; the order is `Shipped` with stock unchanged, or `Cancelled` with its stock restored once

### Requirement: Cancelling claims the order first, then returns its stock and voucher hold

A cancel SHALL first move the order to `Cancelled` with the compare-and-set write; only the caller that wins SHALL release
the order's own reservations by their original reservation ids and then release the voucher hold placed for the order.
A release that fails SHALL be recorded durably and retried by the `team-order` sweep until it succeeds; the sweep SHALL
also release reservations still held by a `Cancelled` order (a crash between claim and release). A voucher release
failure SHALL be logged and SHALL NOT fail the cancel. A cancel of an order that is not `Pending` or `Paid` SHALL fail with
`FAILED_PRECONDITION` and release nothing.

#### Scenario: Cancelling a paid order restores its stock once

- **WHEN** a buyer cancels their `Paid` order for quantity 2 of a listing with stock 8
- **THEN** the order is `Cancelled` and the listing's stock is 10

#### Scenario: Cancelling a shipped order is refused and keeps its stock

- **WHEN** a buyer cancels their `Shipped` order
- **THEN** the call fails with `failed_precondition`, the order is still `Shipped` and the listing's stock is unchanged

#### Scenario: A cancel whose stock release fails is retried until the stock returns

- **WHEN** a buyer cancels a `Pending` order for quantity 2 of a listing with stock 8 while `team-domain` is stopped, and
  `team-domain` is then started again
- **THEN** the cancel succeeds with the order `Cancelled`, and within the configured reservation TTL plus two sweep
  intervals the listing's stock is 10, and stays 10

### Requirement: Only a still-pending order is marked paid by settlement

The `PaymentSettled` consumer SHALL move an order to `Paid` only from `Pending`, in the compare-and-set write that also
records `paid_at`. A settled event for an order in any other status SHALL be acknowledged without changing the order and
logged with the order id and its current status. The consumer SHALL commit the order's voucher hold only for an order
that reached `Paid` (`paid_at` set), so a cancelled order's voucher is never redeemed by a late payment. Redelivery SHALL
remain a no-op.

#### Scenario: A late payment after cancel is ignored

- **WHEN** a buyer starts an online payment for a `Pending` order, cancels the order, and the payment then succeeds
- **THEN** the order read through the gateway stays `Cancelled` and its listing's stock is restored once

#### Scenario: Payment racing cancel ends cancelled with stock restored once

- **WHEN** a buyer's online payment succeeds while the buyer cancels the same `Pending` order at the same time
- **THEN** the order ends `Cancelled` and its listing's stock is restored once

#### Scenario: A late payment of a cancelled voucher order leaves the voucher unused

- **WHEN** a buyer checks out with a voucher whose quota is 1, cancels the order, and a payment for that order then succeeds
- **THEN** the order stays `Cancelled` and another buyer can still check out with that voucher

### Requirement: Shipment creation moves the order through the table

`CreateShipment` SHALL first move the order to `Shipped` with the compare-and-set write (from `Pending` or `Paid`) and
create the shipment only if that write won. For an order in any other status it SHALL fail with `FAILED_PRECONDITION`,
create no shipment and leave the order unchanged. It SHALL NOT discard a failed status write.

#### Scenario: Shipping a cancelled order is refused

- **WHEN** the seller calls `CreateShipment` for an order the buyer has cancelled
- **THEN** the call fails with `failed_precondition`, no shipment exists for the order, and the order is still `Cancelled`
