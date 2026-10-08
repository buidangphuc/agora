## Purpose

Defines the server-side business facts that reach analytics through each owner's transactional outbox and topic:
order lifecycle on `order.events`, engagement actions on `engagement.events`, and the existing settlement facts on
`payment.events`, and how analytics consumes them without touching any other service's database.

## ADDED Requirements

### Requirement: team-order publishes order lifecycle facts through an outbox

`team-order` SHALL write an `OrderPlaced` fact in the same database transaction that creates an order, and an
`OrderStatusChanged` fact in the same transaction as every transition to `PAID`, `SHIPPED`, `COMPLETED` or
`CANCELLED`, and a relayer SHALL publish them as `EventEnvelope`s on `order.events` keyed by `order_id`. A fact SHALL
carry ids, line items (listing id, quantity, unit price), totals and status, and SHALL NOT carry addresses, phone
numbers or names. A rolled-back transaction SHALL publish nothing.

#### Scenario: Placing an order emits OrderPlaced

- **WHEN** a buyer places an order for two listings
- **THEN** exactly one `OrderPlaced` envelope for that order appears on `order.events` with both line items and the
  buyer and seller ids, and no shipping address

#### Scenario: Payment, shipping and cancellation emit status changes

- **WHEN** an order is paid, then shipped; and another order is cancelled
- **THEN** `order.events` carries `OrderStatusChanged` facts `PENDING→PAID`, `PAID→SHIPPED` for the first and
  `→CANCELLED` for the second, in that per-order order

#### Scenario: A failed order write emits nothing

- **WHEN** order creation fails and its transaction rolls back
- **THEN** no `OrderPlaced` fact for it is ever published

### Requirement: team-engagement publishes engagement facts through an outbox

`team-engagement` SHALL write `FavoriteChanged` (added or removed), `ReviewCreated` (rating, listing, order; never
the review text) and `FollowChanged` (followed or unfollowed) facts in the same transaction as the change, and a
relayer SHALL publish them on `engagement.events`, keyed by the acting user id for favorites and follows and by
listing id for reviews. An idempotent no-op (favoriting an already-favorited listing) SHALL emit nothing.

#### Scenario: Favorite and unfavorite are published in order

- **WHEN** a buyer favorites a listing and then removes it
- **THEN** `engagement.events` carries `FavoriteChanged(added)` then `FavoriteChanged(removed)` for that buyer and
  listing

#### Scenario: A review fact carries no text

- **WHEN** a buyer creates a 4-star review with a comment
- **THEN** the `ReviewCreated` fact carries rating 4, listing id and order id, and no comment text

#### Scenario: Repeating a favorite emits nothing

- **WHEN** a buyer favorites a listing that is already in their favorites
- **THEN** no new `FavoriteChanged` fact is published

### Requirement: Analytics ingests order, engagement and payment facts from topics only

`team-analytics` SHALL consume `order.events`, `engagement.events` and `payment.events` with its own consumer groups,
store each fact once (deduplicated by `event_id`), skip envelope types it does not know without failing, and SHALL
NOT connect to any other service's database. Seller funnel order counts SHALL come from these facts.

#### Scenario: An order fact lands in the warehouse

- **WHEN** a buyer places and pays for an order
- **THEN** the warehouse holds one order fact with its line items and a `PAID` status change for that order

#### Scenario: A redelivered fact is stored once

- **WHEN** the same `OrderPlaced` envelope is delivered twice
- **THEN** the warehouse holds exactly one row for it

#### Scenario: The seller funnel counts real orders

- **WHEN** a seller's listing receives one paid order in the queried window
- **THEN** `GetSellerFunnel` reports `orders = 1` for that seller
