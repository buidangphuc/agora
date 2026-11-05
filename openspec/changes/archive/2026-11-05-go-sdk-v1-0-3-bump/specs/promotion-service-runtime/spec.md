## MODIFIED Requirements

### Requirement: Settings load through the SDK and refuse unsafe configuration

team-promotion SHALL load its settings with `config.Load` from go-sdk. `ENV`, `GRPC_PORT`, `DATABASE_URL`, `UPSTREAM_DOMAIN_ADDR` and `UPSTREAM_AUDIT_ADDR` SHALL be required with no default in every `ENV`; an upstream the service calls has no mode where its address may be empty. The service SHALL read no environment variable that switches a dependency on or off; the only allowed flags are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`. `GRPC_REFLECTION_ENABLED` SHALL default to false and SHALL be refused unless `ENV` is `local` or `test`. IF `ENV` is staging or prod, THEN `KAFKA_BROKERS` SHALL be required. `MAX_AD_BID` and `MAX_AD_BUDGET` SHALL be greater than zero. `UPSTREAM_CALL_TIMEOUT_SECONDS` SHALL NOT exist: upstream calls use the SDK 2 s bound. Durations (`SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY`, `FEATURE_FLAGS_EVAL_TIMEOUT`, `OUTBOX_*`) SHALL be Go durations. The subcommand `migrate up` SHALL need only `ENV` and `DATABASE_URL`. `.env.example` SHALL list every declared key.

#### Scenario: Missing ENV is refused
- **WHEN** settings load with `GRPC_PORT`, `DATABASE_URL` and `UPSTREAM_DOMAIN_ADDR` set and `ENV` unset
- **THEN** `Load` returns an error that names `ENV` and no settings are returned
- **VERIFIED BY**: team-promotion `internal/config` `TestLoadRequiresEnv`

#### Scenario: Missing database URL or upstream address is refused
- **WHEN** settings load under `ENV=local`, once without `DATABASE_URL`, once without `UPSTREAM_DOMAIN_ADDR` and once without `UPSTREAM_AUDIT_ADDR`
- **THEN** each load returns an error naming the missing key
- **VERIFIED BY**: team-promotion `internal/config` `TestLoadRequiresDatabaseURLAndUpstreamAddrs`

#### Scenario: Retired enable flags are not read
- **WHEN** `DATABASE_ENABLED=false`, `KAFKA_ENABLED=false`, `FEATURE_FLAGS_ENABLED=false` and `OUTBOX_ENABLED=false` are set and `DATABASE_URL` is unset
- **THEN** `Load` still returns the missing `DATABASE_URL` error, and no `Settings` field carries an `env` tag ending in `_ENABLED` other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-promotion `internal/config` `TestNoDependencyEnableFlags`

#### Scenario: Reflection is off by default and refused outside local
- **WHEN** settings load under `ENV=local` with no reflection variable, and again under `ENV=prod` with `GRPC_REFLECTION_ENABLED=true`
- **THEN** the first load has `ReflectionEnabled` false and the second returns an error naming `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-promotion `internal/config` `TestReflectionDefaultOffAndRefusedInStrictEnv`

#### Scenario: Strict ENV requires brokers
- **WHEN** settings load under `ENV=staging` without `KAFKA_BROKERS`
- **THEN** the load returns an error naming `KAFKA_BROKERS`
- **VERIFIED BY**: team-promotion `internal/config` `TestStrictEnvRequiresKafkaBrokers`

#### Scenario: Ad limits must be positive
- **WHEN** `MAX_AD_BID` or `MAX_AD_BUDGET` is `0`, `-5` or `abc`, and then `1`
- **THEN** the first three values are rejected and `1` is accepted for each key
- **VERIFIED BY**: team-promotion `internal/config` `TestAdCapsDefaultsAndValidation`

#### Scenario: The retired call timeout key is not read
- **WHEN** `UPSTREAM_CALL_TIMEOUT_SECONDS=0.01` is set and `DATABASE_URL` is unset
- **THEN** `Load` returns the missing `DATABASE_URL` error and no `Settings` field carries the key
- **VERIFIED BY**: team-promotion `internal/config` `TestRetiredCallTimeoutKeyNotRead`

#### Scenario: Migrate settings need only ENV and the database URL
- **WHEN** the migrate settings load with only `ENV=local` and `DATABASE_URL` set, and again with `DATABASE_URL` unset
- **THEN** the first load succeeds and the second returns an error naming `DATABASE_URL`
- **VERIFIED BY**: team-promotion `internal/config` `TestLoadMigrateNeedsOnlyEnvAndDatabaseURL`

#### Scenario: .env.example matches the settings
- **WHEN** the drift check runs over `Settings` and `.env.example`
- **THEN** every declared key is present once in the file and every key in the file is declared
- **VERIFIED BY**: team-promotion `internal/config` `TestEnvExampleInSync`
