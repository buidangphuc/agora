## ADDED Requirements

### Requirement: team-identity runs on go-sdk without always-on enable flags

team-identity SHALL wire go-sdk v0.2.1 in its `internal/app` composition root per ADR-0016. It SHALL declare no enable flag for a capability that every deploy runs, and SHALL fail boot in a deployed environment when its database cannot be reached.

#### Scenario: team-identity declares no always-on enable flags
- **WHEN** the declared env keys of team-identity's Settings are listed
- **THEN** none of `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` and `GRPC_REFLECTION_ENABLED` is declared
- **VERIFIED BY**: `TestNoEnableFlagsDeclared`

#### Scenario: team-identity fails boot on an unreachable database when deployed
- **WHEN** team-identity starts with `ENV=staging` and an unreachable `DATABASE_URL`
- **THEN** boot returns an error before any port opens
- **VERIFIED BY**: `TestBootFailsWhenDatabaseUnreachableDeployed`

### Requirement: team-identity messaging uses SDK semantics

team-identity's Kafka publishing and consuming SHALL go through SDK `kafkax`, `outbox` and `consume` where the proposal says so.

#### Scenario: team-identity outbox rows are relayed and a failing row does not block others
- **WHEN** two rows are enqueued and publishing the first fails
- **THEN** the second is published, and the first is retried with backoff and parked after `OUTBOX_MAX_ATTEMPTS`
- **VERIFIED BY**: `team-identity/internal/app` `TestOutboxRelayedViaSDK` (testkit Postgres)

### Requirement: team-identity behaviour is unchanged

All existing tests SHALL move with their packages and pass, with unchanged assertions except auth status message text (codes unchanged) and removed-flag fields.

#### Scenario: team-identity existing suites do not regress and no assertion is weakened
- **WHEN** `make check` runs after the restructure
- **THEN** every pre-existing test passes, and no assertion is removed or loosened beyond removed-flag fields
- **VERIFIED BY**: `team-identity` `make check`
