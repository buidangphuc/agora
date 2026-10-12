## ADDED Requirements

### Requirement: team-analytics runs on go-sdk without always-on enable flags

team-analytics SHALL wire go-sdk v0.2.1 in its `internal/app` composition root per ADR-0016. It SHALL declare no enable flag for a capability that every deploy runs, and SHALL fail boot in a deployed environment when its database cannot be reached.

#### Scenario: team-analytics declares no always-on enable flags
- **WHEN** the declared env keys of team-analytics's Settings are listed
- **THEN** none of `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` and `GRPC_REFLECTION_ENABLED` is declared
- **VERIFIED BY**: `TestNoEnableFlagsDeclared`

#### Scenario: team-analytics fails boot on an unreachable database when deployed
- **WHEN** team-analytics starts with `ENV=staging` and an unreachable `DATABASE_URL`
- **THEN** boot returns an error before any port opens
- **VERIFIED BY**: `TestBootFailsWhenDatabaseUnreachableDeployed`

### Requirement: team-analytics messaging uses SDK semantics

team-analytics's Kafka publishing and consuming SHALL go through SDK `kafkax`, `outbox` and `consume` where the proposal says so.

#### Scenario: team-analytics consumer does not commit when the DLQ produce fails
- **WHEN** handling a record fails permanently and the DLQ produce also fails
- **THEN** the consumer stops with an error and the record is redelivered after restart
- **VERIFIED BY**: `TestConsumerDoesNotCommitOnDLQFailure` (testkit Redpanda)

### Requirement: team-analytics behaviour is unchanged

All existing tests SHALL move with their packages and pass, with unchanged assertions except auth status message text (codes unchanged) and removed-flag fields.

#### Scenario: team-analytics existing suites do not regress and no assertion is weakened
- **WHEN** `make check` runs after the restructure
- **THEN** every pre-existing test passes, and no assertion is removed or loosened beyond removed-flag fields
- **VERIFIED BY**: `team-analytics` `make check`
