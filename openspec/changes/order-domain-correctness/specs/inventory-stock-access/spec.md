## Purpose

Defines who may change stock through `team-domain`'s reservation RPCs and forbids the ledger-less reserve path, so stock cannot be drained or orphaned by a caller that is not the order saga.

## ADDED Requirements

### Requirement: Stock-changing RPCs require service authority

`team-domain` SHALL require the scope `inventory.write` on `ReserveStock`, `ReleaseStock` and `CommitReservation`.
A caller without a principal SHALL receive `UNAUTHENTICATED`; a caller whose principal lacks the scope (anonymous,
buyer, seller or admin) SHALL receive `PERMISSION_DENIED`; in both cases stock and reservations SHALL be unchanged.
The scope SHALL be carried only by the order service's own service principal and SHALL NOT be grantable to a user role.

#### Scenario: An anonymous caller cannot reserve stock

- **WHEN** `ReserveStock` is called with the anonymous principal
- **THEN** the call fails with `PERMISSION_DENIED` and stock is unchanged

#### Scenario: A buyer or seller cannot reserve, release or commit

- **WHEN** a user whose scopes do not include `inventory.write` calls `ReserveStock`, `ReleaseStock` or `CommitReservation`
- **THEN** each call fails with `PERMISSION_DENIED` and nothing changes

#### Scenario: No principal is unauthenticated

- **WHEN** a stock RPC is called with no principal metadata
- **THEN** the call fails with `UNAUTHENTICATED`

#### Scenario: The order service principal succeeds

- **WHEN** a principal of type service with scope `inventory.write` calls `ReserveStock` with a valid request
- **THEN** the reservation is created

### Requirement: Every reservation has an id

`ReserveStock` SHALL reject a request without a `reservation_id` with `INVALID_ARGUMENT`. There SHALL be no reserve
path that decrements stock without recording a reservation, so every decrement can be committed, released and swept.

#### Scenario: Reserve without a reservation id is rejected

- **WHEN** `ReserveStock` is called with an empty `reservation_id`
- **THEN** the call fails with `INVALID_ARGUMENT` and stock is unchanged

#### Scenario: Every successful reserve leaves a ledger row

- **WHEN** a `ReserveStock` call succeeds
- **THEN** a reservation with that id exists and can be released or swept

### Requirement: The order service calls stock RPCs as itself

`team-order` SHALL call `ReserveStock`, `ReleaseStock` and `CommitReservation` with its own service principal and
SHALL NOT forward the end user's principal to them, whether the call is made inside a buyer's request or in
background compensation and sweeping. Checkout, cancel and the sweep SHALL keep working when `team-domain` enforces
the scope.

#### Scenario: A buyer's checkout works when the gate is enforced

- **WHEN** a buyer without `inventory.write` checks out against a `team-domain` that enforces the scope
- **THEN** the order is created and its reservations are committed

#### Scenario: Background release works when the gate is enforced

- **WHEN** the reservation sweep releases a stale reservation against a `team-domain` that enforces the scope
- **THEN** the release succeeds
