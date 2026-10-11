## Purpose

Defines how `team-domain` reserves, commits, releases and expires stock, and what `team-order` must do with a
reservation, so that a placed order keeps its stock and a failed or cancelled one gives it back exactly once.

## ADDED Requirements

### Requirement: A reservation has one lifecycle owned by team-domain

`team-domain` SHALL record every stock decrement as a reservation keyed by a caller-supplied `reservation_id`, with
exactly one status out of `active`, `committed` and `released`, moving only `active → committed`, `active → released` or
`committed → released`. Stock SHALL be decremented once when the reservation is created and incremented at most once per
reservation, only when it moves to `released`, by the quantity stored on the reservation. Reserving again under the id
of an `active` or `committed` reservation SHALL succeed without changing stock; reserving under the id of a `released`
reservation SHALL fail with `FAILED_PRECONDITION` and leave stock unchanged; reserving without a `reservation_id` SHALL
fail with `INVALID_ARGUMENT`. Stock SHALL never be negative.

#### Scenario: A checkout holds the ordered quantity exactly once

- **WHEN** a buyer checks out quantity 2 of a listing whose stock is 10 through the gateway
- **THEN** the listing's stock read through the gateway is 8

### Requirement: Committing a reservation makes its stock permanent

`team-domain` SHALL expose `CommitReservation`, idempotent on `reservation_id`: an `active` reservation becomes
`committed`; a `committed` one succeeds unchanged; a `released` one fails with `FAILED_PRECONDITION`; an unknown one fails
with `NOT_FOUND`; an empty id fails with `INVALID_ARGUMENT`. Only an `active` reservation SHALL be restored by the TTL
sweep, so a committed reservation keeps its stock until it is explicitly released. The RPC SHALL require the same service
authority as the other stock RPCs. `team-order` SHALL commit every reservation of a checkout before it places any of its
orders, and SHALL NOT place an order whose commit failed.

#### Scenario: A placed order keeps its stock after the reservation TTL

- **WHEN** a buyer checks out quantity 2 of a listing with stock 10, and more than the configured reservation TTL plus
  two sweep intervals of both services pass
- **THEN** the listing's stock read through the gateway is still 8 and the order is still `Pending`

### Requirement: Release is idempotent on reservation_id

`team-domain` SHALL release stock only by `reservation_id`, restoring the quantity stored on that reservation (never a
caller-supplied quantity) exactly once. A repeated release, a release of a reservation the TTL sweep already returned,
and a release of an unknown id SHALL succeed without changing stock. A release without a `reservation_id` SHALL fail with
`INVALID_ARGUMENT`. The TTL sweep and a release of the same reservation SHALL never both restore its stock.

#### Scenario: A failed checkout returns its stock once even after the TTL sweep

- **WHEN** a buyer's two-seller checkout fails because the second seller's item is out of stock, and more than the
  configured reservation TTL plus two sweep intervals then pass
- **THEN** the first seller's listing stock read through the gateway equals its stock before the checkout

#### Scenario: A cancelled order's stock is returned once even after the TTL sweep

- **WHEN** a buyer cancels a `Pending` order for quantity 2 of a listing with stock 10, and more than the configured
  reservation TTL plus two sweep intervals then pass
- **THEN** the listing's stock read through the gateway is 10

### Requirement: Reservation lifetime and sweep cadence are configurable

`team-domain` and `team-order` SHALL each read `RESERVATION_TTL` (default `15m`) and `RESERVATION_SWEEP_INTERVAL`
(default `1m`) as Go durations, document both in `.env.example`, and log the effective values when the sweeper starts.
A missing, unparsable or non-positive value SHALL fall back to the default with a logged warning and SHALL NOT stop the
service from starting.

#### Scenario: An invalid reservation TTL falls back to the default

- **WHEN** the `team-domain` image is started with `RESERVATION_TTL=banana` and `RESERVATION_SWEEP_INTERVAL=0s`
- **THEN** it starts, logs a warning naming each variable, and logs the effective TTL `15m0s` and interval `1m0s`

#### Scenario: The e2e stack runs the sweeper on the configured short cadence

- **WHEN** the stack is started with the e2e overlay that sets a short `RESERVATION_TTL` and `RESERVATION_SWEEP_INTERVAL`
  for `team-domain` and `team-order`
- **THEN** each service logs the overlay values as its effective reservation TTL and sweep interval

### Requirement: Stock changes are announced in the same transaction

Every reserve, release and TTL sweep that changes stock SHALL write one `ListingStockChanged` event per affected listing
to the `team-domain` outbox inside the same transaction as the stock change, carrying the listing's stock after the
change (and the affected variant's stock when a variant changed), relayed as an `EventEnvelope` to `listing.events`
keyed by `listing_id`. A rolled-back change and an idempotent no-op (repeated reserve, repeated or unknown release,
commit) SHALL write no event.

#### Scenario: A checkout announces the listing's new stock

- **WHEN** a buyer checks out quantity 2 of a listing with stock 10
- **THEN** a `ListingStockChanged` envelope keyed by that listing id with stock 8 appears on `listing.events`

#### Scenario: A cancel announces the restored stock

- **WHEN** the buyer then cancels that order
- **THEN** a later `ListingStockChanged` envelope for the listing with stock 10 appears on `listing.events`
