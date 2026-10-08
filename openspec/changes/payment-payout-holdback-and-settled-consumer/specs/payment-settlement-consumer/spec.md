## Purpose

Defines how the seller's settlement credit is driven by the durable `PaymentSettled` event instead of only a best-effort inline call.

## ADDED Requirements

### Requirement: PaymentSettled credits the seller idempotently

`team-payment` SHALL consume `PaymentSettled` events from `payment.events` in its own consumer group and, for each PAID event, append one `ORDER_SETTLEMENT` credit equal to the payment transaction amount to the seller of the order, keyed on `(ORDER_SETTLEMENT, payment id)`. The seller SHALL be read from `team-order` `GetOrder` as service principal `service-team-payment` with scope `order.read`. Redelivery of the same event, or the inline credit having run first, SHALL leave exactly one credit. Non-PAID events and other event types SHALL be ignored.

#### Scenario: Seller resolved and credited once

- **WHEN** a PAID `PaymentSettled` event arrives for a transaction of 500000 whose order's seller is `seller-1`
- **THEN** `seller-1` has one `ORDER_SETTLEMENT` entry of +500000 referencing the payment id

#### Scenario: Redelivery does not double-credit

- **WHEN** the same event is delivered three times
- **THEN** the seller still has exactly one `ORDER_SETTLEMENT` entry for that payment

#### Scenario: The inline credit and the consumer do not both credit

- **WHEN** the inline credit already wrote the entry and the event is then consumed
- **THEN** the balance is unchanged and the event is acknowledged

#### Scenario: A non-PAID event is ignored

- **WHEN** a `PaymentSettled` event with status FAILED arrives
- **THEN** no ledger entry is written and the event is acknowledged

### Requirement: Failures are retried, then parked on a dead-letter topic

A transient failure (including a missing order, an order without a seller, or an unreachable `team-order`) SHALL be retried with backoff up to a bounded number of attempts and then produced to the DLQ topic. A permanent failure (malformed envelope or payload, missing ids, unknown or not-settled transaction, non-positive amount) SHALL go to the DLQ without retry. The consumer SHALL commit the offset only after the record was applied or produced to the DLQ. If the DLQ produce fails, the consumer SHALL retry the same record and SHALL NOT process or commit any later record of the partition until it succeeds.

#### Scenario: Missing order is retried then dead-lettered

- **WHEN** the order of a settled payment cannot be found on every attempt
- **THEN** the lookup is attempted the maximum number of times, the record is produced to the DLQ topic, the offset is committed, and no credit is written

#### Scenario: Transient failure recovers

- **WHEN** the first lookup fails with `UNAVAILABLE` and the second succeeds
- **THEN** the credit is written once and nothing is dead-lettered

#### Scenario: Malformed record is dead-lettered without retry

- **WHEN** a record value is not a valid `EventEnvelope`
- **THEN** it is produced to the DLQ after one attempt

### Requirement: The consumer is opt-in with documented configuration

The consumer SHALL run only when `KAFKA_ENABLED=true`, the database is enabled and `PAYMENT_SETTLEMENT_CONSUMER_ENABLED` is not false. Its group and DLQ topic SHALL be configured by `PAYMENT_SETTLEMENT_CONSUMER_GROUP` (default `team-payment.settlement`) and `PAYMENT_SETTLEMENT_DLQ_TOPIC` (default `payment.events.settlement.dlq`), and every Kafka key `team-payment` reads SHALL be documented in `.env.example`.

#### Scenario: Kafka disabled

- **WHEN** `KAFKA_ENABLED` is unset
- **THEN** no consumer starts and settlement still credits through the inline path

#### Scenario: Env documentation

- **WHEN** the env drift test runs
- **THEN** every key read by `config` and by the Kafka bootstrap appears in `.env.example`

#### Scenario: DLQ outage does not lose a record

- **WHEN** a record cannot be applied and the DLQ produce fails, and a later record exists on the same partition
- **THEN** the later record is neither processed nor committed until the first record has been produced to the DLQ
