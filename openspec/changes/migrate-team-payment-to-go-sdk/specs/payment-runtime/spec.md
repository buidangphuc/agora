## ADDED Requirements

### Requirement: team-payment runs on go-sdk without always-on enable flags

team-payment SHALL wire go-sdk v1.0.0 in its `internal/app` composition root per ADR-0016. It SHALL declare no enable flag for a capability that every deploy runs, and SHALL fail boot in a deployed environment when its database cannot be reached.

#### Scenario: team-payment declares no always-on enable flags
- **WHEN** the declared env keys of team-payment's Settings are listed
- **THEN** none of `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` and `GRPC_REFLECTION_ENABLED` is declared
- **VERIFIED BY**: `TestNoEnableFlagsDeclared`

#### Scenario: team-payment fails boot on an unreachable database when deployed
- **WHEN** team-payment starts with `ENV=staging` and an unreachable `DATABASE_URL`
- **THEN** boot returns an error before any port opens
- **VERIFIED BY**: `TestBootFailsWhenDatabaseUnreachableDeployed`

### Requirement: team-payment messaging uses SDK semantics

team-payment's Kafka publishing and consuming SHALL go through SDK `kafkax`, `outbox` and `consume` where the proposal says so.

#### Scenario: team-payment outbox rows are relayed and a failing row does not block others
- **WHEN** two rows are enqueued and publishing the first fails
- **THEN** the second is published, and the first is retried with backoff and is never parked by a transient error (go-sdk v1 has no attempt limit; only an error permanent for the record parks a row)
- **VERIFIED BY**: `team-payment/internal/app` `TestOutboxRelayedViaSDK` (testkit Postgres)

#### Scenario: team-payment outbox table upgrades to the go-sdk v1 schema in creation order
- **WHEN** the forward migrations 0008-0017 run on an outbox table of the previous schema that holds rows, including a retried row and rows of one order
- **THEN** every row has a unique `seq` in `(created_at, event_id)` order, `seq` is NOT NULL with a default for new rows, the v1 indexes exist and the v0 claim index is gone, and the SDK relayer publishes an order's rows in creation order
- **VERIFIED BY**: `team-payment/internal/store` `TestOutboxUpgradeFromV0_Postgres` (testkit Postgres)

#### Scenario: team-payment consumer does not commit when the DLQ produce fails
- **WHEN** handling a record fails permanently and the DLQ produce also fails
- **THEN** the consumer stops with an error and the record is redelivered after restart
- **VERIFIED BY**: `TestConsumerDoesNotCommitOnDLQFailure` (testkit Redpanda)

### Requirement: team-payment behaviour is unchanged

All existing tests SHALL move with their packages and pass, with unchanged assertions except auth status message text (codes unchanged) and removed-flag fields.

#### Scenario: team-payment keeps its client-visible error text when the order lookup fails
- **WHEN** team-order is down or refuses `GetOrder` during `GetPayment`, `RefundPayment` or `CreatePayment`
- **THEN** the caller gets `Unavailable` "order service unavailable" or `NotFound` "order not found", never team-order's own code or text
- **VERIFIED BY**: `TestOrderLookupFailureKeepsClientText`

#### Scenario: team-payment existing suites do not regress and no assertion is weakened
- **WHEN** `make check` runs after the restructure
- **THEN** every pre-existing test passes, and no assertion is removed or loosened beyond removed-flag fields
- **VERIFIED BY**: `team-payment` `make check`

### Requirement: Boot validation names every invalid setting

team-payment's boot validation (`config.Load`: the SDK's group validation joined with `Settings.Validate`) SHALL report every invalid setting in one error rather than only the first, so a refused boot names each offending key. Any error still refuses to start.

#### Scenario: Invalid hold window is named even without a database URL
- **WHEN** `DATABASE_URL` is empty and `PAYOUT_HOLD_WINDOW` is invalid
- **THEN** the error of `Load` (SDK group validation plus `Settings.Validate`) mentions both `DATABASE_URL` and `PAYOUT_HOLD_WINDOW`, each once
- **VERIFIED BY**: `TestValidateNamesEveryInvalidSetting`
