## Purpose

Keeps `team-order`'s configuration honest (every variable it reads is declared, effective and documented) and keeps its event-consumer dedupe from hiding infrastructure failures.

## ADDED Requirements

### Requirement: Every configuration variable is declared, effective and documented

Every environment variable `team-order` reads SHALL be declared in its configuration and SHALL be documented in
`.env.example` and the README, and the two lists SHALL match exactly in both directions, enforced by an automated
test. A variable documented as tunable SHALL actually take effect; in particular `DB_MAX_CONNS` SHALL set the
database pool size, and the Kafka consumer variables SHALL be declared in configuration rather than read ad hoc.

#### Scenario: DB_MAX_CONNS sets the pool size

- **WHEN** the service starts with `DB_MAX_CONNS=25`
- **THEN** the loaded settings report a maximum of 25 connections, and without the variable they report the default of 10

#### Scenario: The Kafka consumer keys are declared

- **WHEN** the declared configuration keys are listed
- **THEN** they include `KAFKA_ENABLED`, `KAFKA_BROKERS`, `ORDER_PAYMENT_CONSUMER_GROUP`, `PAYMENT_EVENTS_TOPIC` and `PAYMENT_EVENTS_DLQ_TOPIC` with their current defaults

#### Scenario: RESERVATION_TTL sets the order-side reservation lifetime

- **WHEN** the service starts with `RESERVATION_TTL=2m`, then with an invalid or non-positive value, then without the variable
- **THEN** the reservation expiry is 2 minutes, then the 15 minute default with a logged warning, then the 15 minute default

#### Scenario: Env drift fails the test

- **WHEN** a key is declared in configuration but missing from `.env.example`, or documented there but not declared
- **THEN** the env-drift test fails and names the key

### Requirement: Dedupe lookups surface infrastructure failures

The consumer dedupe ledger SHALL report "not processed" only when no record exists. Any other failure of the lookup
SHALL be returned as an error so the consumer treats it as transient, does not commit the offset and retries. A
duplicate delivery SHALL remain an idempotent no-op.

#### Scenario: A missing record means not processed

- **WHEN** the ledger has no record for an event id
- **THEN** the lookup returns false with no error

#### Scenario: A database error is not reported as not-processed

- **WHEN** the ledger lookup fails with a database error
- **THEN** the lookup returns that error and the consumer does not apply the event or commit its offset

#### Scenario: A duplicate delivery is still a no-op

- **WHEN** an event id that was already applied is delivered again
- **THEN** the lookup returns true and the order is not changed
