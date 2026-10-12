## ADDED Requirements

### Requirement: team-order runs its messaging on go-sdk

team-order SHALL wire go-sdk v0.2.0 in `internal/app.Run(ctx, settings)`:
- the outbox relayer is SDK `outbox.Relayer` over `order_outbox_events`;
- the PaymentSettled consumer is SDK `consume.Consumer`, with dedupe through SDK `consume.Dedupe` in the same transaction as the order transition.

It SHALL declare no enable flags for always-on capabilities.

#### Scenario: PaymentSettled is applied once even when redelivered
- **WHEN** the same PaymentSettled event is delivered twice to the consumer
- **THEN** the order moves to PAID once, and the second delivery is acknowledged without a second transition
- **VERIFIED BY**: `team-order/internal/events` `TestPaymentSettledAppliedOnceOnRedelivery` (testkit Postgres)

#### Scenario: A failed DLQ produce does not lose the payment event
- **WHEN** handling a PaymentSettled fails permanently and the DLQ produce also fails
- **THEN** the consumer stops with an error and the event is redelivered after restart
- **VERIFIED BY**: `TestPaymentEventNotLostWhenDLQFails` (testkit Redpanda + Postgres)

#### Scenario: Rolled-back transition is not marked processed
- **WHEN** the PAID transition fails inside the handler transaction
- **THEN** the event is not recorded in `processed_events` and a later delivery is processed
- **VERIFIED BY**: `TestRolledBackTransitionIsNotDeduped`

#### Scenario: team-order declares no enable flags
- **WHEN** the declared env keys of team-order's Settings are listed
- **THEN** none of `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` and `GRPC_REFLECTION_ENABLED` is declared
- **VERIFIED BY**: `TestNoEnableFlagsDeclared`

### Requirement: Dedupe is keyed by consumer

Migration `0008` SHALL change the `processed_events` primary key to `(consumer, event_id)` and backfill `consumer` for existing rows with `team-order.payment`.

#### Scenario: Existing processed rows keep deduplicating after the migration
- **WHEN** a row processed before `0008` exists and the same event arrives after it
- **THEN** it is recognised as already processed, not applied again
- **VERIFIED BY**: `team-order/internal/store` `TestMigration0008KeepsExistingDedupe`

### Requirement: team-order behaviour is unchanged

All existing tests SHALL move with their packages and pass, with unchanged assertions except auth status message text (codes unchanged).

#### Scenario: team-order existing suites do not regress and no assertion is weakened
- **WHEN** `make check` runs after the restructure
- **THEN** every pre-existing test passes, and no assertion is removed or loosened
- **VERIFIED BY**: `team-order` `make check`
