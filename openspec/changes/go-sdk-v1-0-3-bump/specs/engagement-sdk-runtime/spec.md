## MODIFIED Requirements

### Requirement: Settings come from the SDK groups and name every variable

team-engagement SHALL load its settings with `config.Load` of go-sdk from the groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox`, `Audit`, `LogKeyFP` and the service groups `Events` (`KAFKA_CONSUMER_GROUP`, `KAFKA_LISTING_TOPIC`, `ENGAGEMENT_EVENTS_TOPIC`), `Upstream` (`UPSTREAM_ORDER_ADDR`) and `Reports` (`REPORT_RETENTION_DAYS`, `REPORT_SWEEP_INTERVAL`). `ENV`, `GRPC_PORT` (positive, no SDK default), `DATABASE_URL`, `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` SHALL be required in every `ENV`: every upstream the service calls has a required address and there is no mode without it; the SDK `Audit` group alone would accept an empty address in `local` and `test`, so the service SHALL add its own check after the SDK groups. The service image and `.env.example` SHALL keep supplying `GRPC_PORT=50054`. The service SHALL NOT declare `DATABASE_ENABLED`, `KAFKA_ENABLED`, `ENGAGEMENT_OUTBOX_RELAY_INTERVAL`, `SHUTDOWN_GRACE_SECONDS` or `UPSTREAM_CALL_TIMEOUT_SECONDS`: every call to team-order and team-audit is bounded by the SDK's fixed 2 s. `Load` SHALL refuse `GRPC_REFLECTION_ENABLED=true` whenever `ENV` is anything other than `local` or `test` (staging, prod and any unrecognised value such as `dev`). When `ENV` is staging, stage, prod or production (case-insensitive), `Load` SHALL also refuse an empty `KAFKA_BROKERS` or `LOG_KEY_FP_SECRET`; an unrecognised `ENV` is not strict for those two. `.env.example` SHALL list exactly the declared variables.

#### Scenario: Engagement SDK: Missing ENV is refused
- **WHEN** `Load` runs with `ENV` unset and `GRPC_PORT=50054`, `DATABASE_URL`, `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` set
- **THEN** it returns an error that names `ENV`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesMissingEnv`

#### Scenario: Engagement SDK: Empty database url is refused in every ENV
- **WHEN** `Load` runs with `ENV=local`, `GRPC_PORT=50054`, `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` set and an empty `DATABASE_URL`
- **THEN** it returns an error that names `DATABASE_URL`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesEmptyDatabaseURL`

#### Scenario: Engagement SDK: Empty order address is refused in every ENV
- **WHEN** `Load` runs with `ENV=local`, `GRPC_PORT=50054`, `DATABASE_URL` and `UPSTREAM_AUDIT_ADDR` set and an empty `UPSTREAM_ORDER_ADDR`
- **THEN** it returns an error that names `UPSTREAM_ORDER_ADDR`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesEmptyOrderAddr`

#### Scenario: Engagement SDK: Empty audit address is refused in every ENV
- **WHEN** `Load` runs with otherwise valid settings and an empty `UPSTREAM_AUDIT_ADDR`, once with `ENV=local`, once with `ENV=test` and once with `ENV=dev`
- **THEN** each call returns an error that names `UPSTREAM_AUDIT_ADDR`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesEmptyAuditAddr`

#### Scenario: Engagement SDK: Missing port is refused
- **WHEN** `Load` runs with `ENV=local`, `DATABASE_URL`, `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` set and `GRPC_PORT` unset, and then with `GRPC_PORT=` empty
- **THEN** both calls return an error that names `GRPC_PORT`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesMissingGRPCPort`

#### Scenario: Engagement SDK: Removed switches are not declared and have no effect
- **WHEN** `Load` runs with valid settings including `GRPC_PORT=50054` and also `DATABASE_ENABLED=false`, `KAFKA_ENABLED=false`, `ENGAGEMENT_OUTBOX_RELAY_INTERVAL=1h` and `UPSTREAM_CALL_TIMEOUT_SECONDS=30`
- **THEN** it succeeds with the database still required, `DeclaredEnvKeys` contains none of the five removed names, and the outbox poll interval is the `OUTBOX_POLL_INTERVAL` default of 1 s
- **VERIFIED BY**: team-engagement `internal/config` `TestRemovedSwitchesAreNotDeclared`

#### Scenario: Engagement SDK: Strict ENV refuses reflection
- **WHEN** `Load` runs with otherwise valid settings and `GRPC_REFLECTION_ENABLED=true`, once with `ENV=staging` and once with `ENV=prod`
- **THEN** each call returns an error that names `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-engagement `internal/config` `TestStrictEnvRefusesReflection`

#### Scenario: Engagement SDK: Unrecognised ENV refuses reflection but is not strict
- **WHEN** `Load` runs with `ENV=dev`, `GRPC_REFLECTION_ENABLED=true` and then with `ENV=dev`, `GRPC_REFLECTION_ENABLED=false` and `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` empty (and `UPSTREAM_AUDIT_ADDR` set)
- **THEN** the first call returns an error that names `GRPC_REFLECTION_ENABLED` and the second succeeds with the SDK local Kafka default
- **VERIFIED BY**: team-engagement `internal/config` `TestUnrecognisedEnvRefusesReflectionButIsNotStrict`

#### Scenario: Engagement SDK: Local and test ENV allow reflection
- **WHEN** `Load` runs with `GRPC_REFLECTION_ENABLED=true` and `ENV=local`, and then with `ENV=test`
- **THEN** both calls succeed with reflection true
- **VERIFIED BY**: team-engagement `internal/config` `TestLocalAndTestAllowReflection`

#### Scenario: Engagement SDK: Strict ENV requires brokers, audit address and fingerprint secret
- **WHEN** `Load` runs with `ENV=prod`, `GRPC_PORT=50054`, `DATABASE_URL` and `UPSTREAM_ORDER_ADDR` set and `KAFKA_BROKERS`, `UPSTREAM_AUDIT_ADDR` and `LOG_KEY_FP_SECRET` all empty
- **THEN** it returns one error that names all three variables (the audit address is refused in every `ENV`, brokers and secret only in a strict one)
- **VERIFIED BY**: team-engagement `internal/config` `TestStrictEnvRequiresBrokersAuditAndSecret`

#### Scenario: Engagement SDK: Local defaults
- **WHEN** `Load` runs with `ENV=local`, `GRPC_PORT=50054`, a `DATABASE_URL`, a `UPSTREAM_ORDER_ADDR` and a `UPSTREAM_AUDIT_ADDR` only
- **THEN** it succeeds, `GRPC_PORT` is 50054, reflection is false, the listing topic is `listing.events`, the consumer group is `team-engagement-feed`, the events topic is `engagement.events` and `REPORT_RETENTION_DAYS` is 730
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadLocalDefaults`

#### Scenario: Engagement SDK: Example file matches the declared variables
- **WHEN** `.env.example` and the declared variables of the settings are compared in both directions
- **THEN** no variable is missing on either side
- **VERIFIED BY**: team-engagement `internal/config` `TestEnvExampleInSync`

### Requirement: The service follows the SDK layout and keeps no runtime fallback

The module SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), with no `replace` directive and no `third_party` directory. The Makefile SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`; `golangci-lint` SHALL be pinned as a `tool` in `go.mod`. `cmd/server/main.go` SHALL only dispatch the `migrate up` subcommand to `internal/app.Migrate` or load settings and call `internal/app.Run`. The packages `bootstrap`, `grpcserver`, `observability`, `interceptor`, `handler`, `service`, `repository` and `consumer` SHALL NOT exist. Non-test Go files SHALL NOT contain `InMemory`, `Noop` or `RequireDurableStorage`, no non-test file SHALL import the fake-store package `internal/store/storetest`, and the service SHALL NOT read an environment variable that switches Postgres, Kafka, the outbox, a consumer or the sweep on or off.

#### Scenario: Engagement SDK: Module requires the tag and nothing else
- **WHEN** `go.mod` is parsed and the directory tree is listed
- **THEN** there is exactly one SDK requirement, at a version matching `^v\d+\.\d+\.\d+$`, a golangci-lint `tool` line, no `replace` line and no `third_party` directory
- **VERIFIED BY**: team-engagement `internal/docs` `TestModuleUsesTaggedSDK`

#### Scenario: Engagement SDK: Old packages are gone
- **WHEN** the packages under `internal/` are listed
- **THEN** none of `bootstrap`, `grpcserver`, `observability`, `interceptor`, `handler`, `service`, `repository`, `consumer` is present and `app`, `config`, `rpc`, `engagement`, `store`, `events`, `upstream` are
- **VERIFIED BY**: team-engagement `internal/docs` `TestLayoutFollowsADR0016`

#### Scenario: Engagement SDK: No fallback code in the binary
- **WHEN** every non-test `.go` file under `cmd` and `internal` is scanned
- **THEN** none contains `InMemory`, `Noop`, `RequireDurableStorage`, `DATABASE_ENABLED`, `KAFKA_ENABLED` or `_ENABLED` other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`, and none outside `internal/store/storetest` imports `internal/store/storetest`
- **VERIFIED BY**: team-engagement `internal/docs` `TestNoRuntimeFallbacks`

#### Scenario: Engagement SDK: A reintroduced enable flag fails the scan
- **WHEN** the scan runs over a fixture tree whose non-test file declares `KAFKA_ENABLED` or constructs `NewInMemoryRepository`
- **THEN** the scan reports a violation for that file and the layout gate fails
- **VERIFIED BY**: team-engagement `internal/docs` `TestNoRuntimeFallbacksDetectsViolation`

#### Scenario: Engagement SDK: Makefile uses the shared files
- **WHEN** the Makefile is read
- **THEN** it includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and no `.golangci.yml` exists in the service
- **VERIFIED BY**: team-engagement `internal/docs` `TestMakefileUsesSharedFiles`
