## ADDED Requirements

### Requirement: team-chat runs on go-sdk without always-on enable flags

team-chat SHALL wire go-sdk v0.2.1 in its `internal/app` composition root per ADR-0016. It SHALL declare no enable flag for a capability that every deploy runs, and SHALL fail boot in a deployed environment when its database cannot be reached.

#### Scenario: team-chat declares no always-on enable flags
- **WHEN** the declared env keys of team-chat's Settings are listed
- **THEN** none of `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` and `GRPC_REFLECTION_ENABLED` is declared
- **VERIFIED BY**: `TestNoEnableFlagsDeclared`

#### Scenario: team-chat fails boot on an unreachable database when deployed
- **WHEN** team-chat starts with `ENV=staging` and an unreachable `DATABASE_URL`
- **THEN** boot returns an error before any port opens
- **VERIFIED BY**: `TestBootFailsWhenDatabaseUnreachableDeployed`

### Requirement: team-chat messaging uses SDK semantics

team-chat's Kafka publishing and consuming SHALL go through SDK `kafkax`, `outbox` and `consume` where the proposal says so.

#### Scenario: team-chat outbox rows are relayed and a failing row does not block others
- **WHEN** two rows are enqueued and publishing the first fails
- **THEN** the second is published, and the first is retried with backoff and is never parked by a transient error (go-sdk v1 has no attempt limit; only a permanent record error parks a row)
- **VERIFIED BY**: `team-chat/internal/app` `TestOutboxRelayedViaSDK` (testkit Postgres)

#### Scenario: team-chat outbox table upgrades to the go-sdk v1 schema in creation order
- **WHEN** the forward migrations run on an outbox table of the previous schema that holds rows, including a retried row and rows of one thread
- **THEN** every row has a unique `seq` in `(created_at, event_id)` order, `seq` is NOT NULL with a default for new rows, the v1 indexes exist and the v0 claim index is gone, and the SDK relayer publishes a thread's rows in creation order
- **VERIFIED BY**: `team-chat/internal/store` `TestOutboxV1UpgradeKeepsCreationOrder_Postgres` (testkit Postgres)

### Requirement: team-chat behaviour is unchanged

All existing tests SHALL move with their packages and pass, with unchanged assertions except auth status message text (codes unchanged) and removed-flag fields.

#### Scenario: team-chat existing suites do not regress and no assertion is weakened
- **WHEN** `make check` runs after the restructure
- **THEN** every pre-existing test passes, and no assertion is removed or loosened beyond removed-flag fields
- **VERIFIED BY**: `team-chat` `make check`
