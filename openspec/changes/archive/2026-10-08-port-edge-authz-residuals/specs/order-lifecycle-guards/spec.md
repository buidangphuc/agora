## MODIFIED Requirements

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
