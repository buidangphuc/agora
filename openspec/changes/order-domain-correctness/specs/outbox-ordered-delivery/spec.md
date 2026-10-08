## Purpose

Guarantees that a service's transactional outbox is relayed to Kafka in the order events were written for the same aggregate, so consumers that diff against the last state they saw (for example price-drop alerts) are not fooled by reordering.

## ADDED Requirements

### Requirement: Outbox events of one aggregate are produced in write order

Each service that relays a transactional outbox (`team-domain` to `listing.events`, `team-payment` to
`payment.events`) SHALL produce the events of a single aggregate to Kafka in the order they were written, including
when several events of that aggregate are claimed in the same batch. The guarantee is per aggregate only (events of
different aggregates have no relative order) and delivery remains at-least-once.

#### Scenario: Two events of one listing in the same batch are produced in write order

- **WHEN** a listing's `CREATED` event is written first and its `UPDATED` event second, and both are claimed in one batch
- **THEN** `CREATED` is produced before `UPDATED` and the claim returns them in that order

#### Scenario: A reversed physical row order does not reorder events

- **WHEN** the rows of one aggregate are stored or updated in an order different from their write order
- **THEN** the claimed batch is still in write order

#### Scenario: Events written by concurrent transactions follow the order the aggregate was changed

- **WHEN** two transactions change the same aggregate and the one that started first commits last
- **THEN** the events are produced in the order the changes were applied, not the order the transactions started

#### Scenario: Payment events follow the same guarantee

- **WHEN** two outbox events of the same order are claimed in one batch by `team-payment`
- **THEN** they are produced in write order

### Requirement: A newer event never overtakes an older unpublished event of the same aggregate

An event SHALL NOT be claimed while an older event of the same aggregate is still pending but not claimable (waiting
out a retry backoff or leased by another relayer). When a produce fails, the relayer SHALL NOT produce later events
of that aggregate in the same pass; it SHALL release their claim so they are retried right after the failed event,
and events of other aggregates SHALL proceed. A parked (`failed`) event SHALL NOT block its aggregate.

#### Scenario: A backed-off event holds back its aggregate only

- **WHEN** the older event of listing A fails to produce and is scheduled for retry, while a newer event of A and an event of listing B are pending
- **THEN** A's newer event is not produced before A's older event, and B's event is produced

#### Scenario: The relayer stops an aggregate at its first failure within a pass

- **WHEN** producing the first of two claimed events of the same aggregate fails
- **THEN** the second is not produced in that pass and its claim is released for the next pass

#### Scenario: A parked poison event does not block later events

- **WHEN** an older event of an aggregate has been parked as `failed`
- **THEN** later events of that aggregate are still claimed and produced
