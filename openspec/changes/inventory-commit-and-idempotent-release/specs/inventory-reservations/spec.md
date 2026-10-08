## Purpose

Defines how stock is reserved, committed, released and expired across `team-domain` (owner of stock) and `team-order` (saga coordinator), so that a placed order keeps its stock and a failed or cancelled one gives it back exactly once.

## ADDED Requirements

### Requirement: A reservation has a single lifecycle owned by team-domain

`team-domain` SHALL track every stock reservation with exactly one status from
`active → committed | released`, and SHALL treat `committed` and `released` as terminal. Stock SHALL
be decremented once when a reservation is created and SHALL be incremented at most once per
reservation, and only when it moves `active → released`.

#### Scenario: Reserving decrements stock once

- **WHEN** a reservation is created for quantity 2 against a listing with stock 10
- **THEN** stock is 8 and the reservation status is `active`

#### Scenario: Reserving with an existing id does not decrement twice

- **WHEN** the same `reservation_id` is reserved again while its status is `active` or `committed`
- **THEN** the call succeeds and stock is unchanged

#### Scenario: Reserving with the id of a released reservation does not report a false success

- **WHEN** the same `reservation_id` is reserved again after its status became `released`
- **THEN** the call does not silently succeed without decrementing; it fails with a precondition error

### Requirement: Committing a reservation makes its stock permanent

`team-domain` SHALL expose a `CommitReservation` operation, idempotent on `reservation_id`, that moves
an `active` reservation to `committed`. A committed reservation SHALL NOT be restored by TTL expiry.
Committing a `released` reservation SHALL fail with a precondition error so the caller cannot place an
order whose stock has already been given back.

#### Scenario: Commit keeps stock after the TTL

- **WHEN** a reservation is committed and the 15-minute reservation TTL then elapses and the sweeper runs
- **THEN** stock is unchanged and the reservation is still `committed`

#### Scenario: Commit is idempotent

- **WHEN** `CommitReservation` is called twice with the same `reservation_id`
- **THEN** both calls succeed and the reservation is `committed`

#### Scenario: Commit of an already released reservation is refused

- **WHEN** `CommitReservation` is called for a reservation whose status is `released`
- **THEN** the call fails with `FAILED_PRECONDITION` and stock is unchanged

#### Scenario: Commit of an unknown reservation is refused

- **WHEN** `CommitReservation` is called with a `reservation_id` that does not exist
- **THEN** the call fails with `NOT_FOUND`

### Requirement: Release is idempotent on reservation_id

`team-domain` SHALL release stock by `reservation_id`, restoring the quantity recorded on that
reservation (not a caller-supplied quantity) exactly once. A repeated release, a release of an already
released or swept reservation, or a release after a retry whose first response was lost SHALL succeed
as a no-op without changing stock. A release request without a `reservation_id` SHALL be rejected with
`INVALID_ARGUMENT`.

#### Scenario: Duplicate release restores stock once

- **WHEN** `ReleaseStock` is called twice with the same `reservation_id` for a quantity-2 reservation on stock 8
- **THEN** stock is 10 after both calls and both calls succeed

#### Scenario: Release after TTL sweep is a no-op

- **WHEN** the sweeper has already released a reservation and `ReleaseStock` is then called for it
- **THEN** stock is unchanged and the call succeeds

#### Scenario: Release uses the stored quantity

- **WHEN** `ReleaseStock` is called with a quantity different from the one reserved
- **THEN** the restored amount equals the reserved quantity

#### Scenario: Release without a reservation id is rejected

- **WHEN** `ReleaseStock` is called with an empty `reservation_id`
- **THEN** the call fails with `INVALID_ARGUMENT` and stock is unchanged

### Requirement: Stock never goes negative

`team-domain` SHALL enforce at the database level that stock on listings and on variants is never
negative and that a reservation quantity is always positive, in addition to the conditional
`stock >= quantity` check on reserve.

#### Scenario: Insufficient stock is refused

- **WHEN** a reservation asks for more than the available stock
- **THEN** the reservation fails and stock is unchanged

#### Scenario: A direct write cannot make stock negative

- **WHEN** any statement would set a stock value below zero
- **THEN** the database rejects it

### Requirement: Stock changes are announced in the same transaction

Every stock change (reserve, release, TTL sweep) SHALL record a `ListingStockChanged` event on the
`team-domain` outbox inside the same transaction as the stock change, and a rolled-back change SHALL
leave no event.

#### Scenario: Reserve records a stock event atomically

- **WHEN** a reservation commits
- **THEN** exactly one pending `ListingStockChanged` outbox row exists for it, written in the same transaction

#### Scenario: Idempotent repeat records no extra event

- **WHEN** a release or reserve is a no-op because of idempotency
- **THEN** no additional outbox row is written

### Requirement: Placing an order commits its reservations

`team-order` SHALL call `CommitReservation` for each of the order's reservations when the order is
persisted, and SHALL NOT treat the order as placed if a commit fails because the reservation was
released. It SHALL reach `team-domain` only through gRPC.

#### Scenario: Placed order keeps its stock after the TTL

- **WHEN** a buyer checks out successfully and more than 15 minutes pass
- **THEN** the listing stock is still reduced by the ordered quantity

#### Scenario: Expired reservation blocks the order

- **WHEN** the reservation was released before the commit
- **THEN** the order is not placed and the buyer is told the item is no longer reserved

### Requirement: Cancelling an order returns stock exactly once and never loses it

Cancelling an order SHALL release the order's original reservations by their original
`reservation_id`. The order's move to `Cancelled` is claimed atomically first (see
`order-lifecycle-guards`) and only the caller that wins the claim releases stock. If a release fails,
`team-order` SHALL record it durably as a retryable failure (`RELEASE_FAILED`) so the existing sweep
retries it until it succeeds; a cancelled order SHALL never be left with an unreleased reservation and no
retryable record. Repeating the cancel SHALL NOT restore stock twice.

#### Scenario: Cancel restores stock once

- **WHEN** an order is cancelled twice
- **THEN** the listing stock increases by the ordered quantity exactly once

#### Scenario: Failed release is retried, not lost

- **WHEN** the release call fails during cancel
- **THEN** the failure is stored as `RELEASE_FAILED`, and a later sweep releases the stock

#### Scenario: Cancel after payment settlement still restores stock

- **WHEN** a committed (paid) order is cancelled
- **THEN** its committed reservation is released and stock is restored once
