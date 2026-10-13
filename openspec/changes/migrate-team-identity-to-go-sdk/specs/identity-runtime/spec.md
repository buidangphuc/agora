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
- **THEN** the second is published, and the first is retried with backoff and stays pending (go-sdk v1 has no attempt limit; only a permanent publish error parks a row)
- **VERIFIED BY**: `team-identity/internal/app` `TestOutboxRelayedViaSDK` (testkit Postgres)

#### Scenario: team-identity outbox table upgrades from v0 keeping event order
- **WHEN** the migrations are applied to a v0 `identity_outbox_events` table that holds rows, including a retried row of an aggregate with newer events
- **THEN** `seq` numbers the rows by `created_at` and `event_id`, the v1 indexes exist, and the SDK relayer publishes each aggregate in creation order
- **VERIFIED BY**: `team-identity/migrations` `TestOutboxUpgradeKeepsOrderOfExistingRows` (testkit Postgres)

### Requirement: team-identity wire formats are unchanged

Tokens issued by team-identity SHALL decode into SDK `jwtx.Claims`, and its JWKS document SHALL keep its JSON shape.

#### Scenario: team-identity tokens and JWKS keep their wire shape
- **WHEN** identity issues a token and serves its JWKS
- **THEN** the token payload decodes into `jwtx.Claims` with the same claim names and numeric `exp` and `iat`, and the JWKS JSON is byte-identical to the previous document
- **VERIFIED BY**: `team-identity/internal/token` `TestIssuedTokenDecodesIntoJWTXClaims`, `TestJWKSEndpointJSONShape`

#### Scenario: team-identity rejects an invalid signing key or an empty key id
- **WHEN** the signer is built from a value that is not a PEM key, or with a blank `kid`
- **THEN** construction fails, so no token is issued without a key id the JWKS can serve
- **VERIFIED BY**: `team-identity/internal/token` `TestNewSignerRejectsBadInput`

### Requirement: team-identity behaviour is unchanged

All existing tests SHALL move with their packages and pass, with unchanged assertions except auth status message text (codes unchanged), removed-flag fields, and the outbox attempt-limit assertions, which follow go-sdk v1 (retry without limit, park only on a permanent error).

#### Scenario: team-identity existing suites do not regress and no assertion is weakened
- **WHEN** `make check` runs after the restructure
- **THEN** every pre-existing test passes, and no assertion is removed or loosened beyond removed-flag fields
- **VERIFIED BY**: `team-identity` `make check`
