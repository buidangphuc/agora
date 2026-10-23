## ADDED Requirements

### Requirement: team-domain depends on the tagged go-sdk and follows the ADR-0016 layout

`team-domain/go.mod` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.2`, SHALL contain no `replace` directive, and the repository SHALL contain no `team-domain/third_party` directory. golangci-lint SHALL be pinned as a `tool` directive in `go.mod`. `team-domain/Makefile` SHALL include `../platform-core/make/go-service.mk`, set `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, and `team-domain` SHALL hold no copy of either file. `cmd/server/main.go` SHALL only load settings and call `internal/app.Run(ctx, settings)` (or `app.Migrate` for `migrate up`). The packages `internal/bootstrap`, `internal/grpcserver`, `internal/interceptor`, `internal/observability`, `internal/handler`, `internal/service`, `internal/repository`, `internal/consumer`, `internal/upstream` and `internal/storage` SHALL NOT exist; their code SHALL live in `internal/app`, `internal/rpc`, `internal/listing`, `internal/store`, `internal/events`, `internal/screen`, `internal/audit` and `internal/objects`.

#### Scenario: Domain SDK: go.mod pins the tag without replace
- **WHEN** `team-domain/go.mod` is parsed
- **THEN** it requires go-sdk `v1.0.2`, has 0 `replace` directives, declares golangci-lint as a `tool`, and `team-domain/third_party` does not exist
- **VERIFIED BY**: team-domain `internal/app` `TestGoModRequiresTaggedSDK`

#### Scenario: Domain SDK: Makefile references the shared files
- **WHEN** `team-domain/Makefile` is read and the directory is listed
- **THEN** the Makefile includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and `team-domain/.golangci.yml` and `team-domain/make/` do not exist
- **VERIFIED BY**: team-domain `internal/app` `TestMakefileUsesSharedBuildFiles`

#### Scenario: Domain SDK: Superseded packages are gone
- **WHEN** the directories under `team-domain/internal` are listed
- **THEN** none of `bootstrap`, `grpcserver`, `interceptor`, `observability`, `handler`, `service`, `repository`, `consumer`, `upstream`, `storage` exists and each of `app`, `config`, `rpc`, `listing`, `store`, `events`, `objects`, `screen`, `audit` does
- **VERIFIED BY**: team-domain `internal/app` `TestLegacyPackagesRemoved`

### Requirement: Settings load through the SDK with ENV required and no dependency flags

`team-domain` SHALL load its settings with `config.Load` over the SDK groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox`, `Audit` and `LogKeyFP` plus its own groups for topics, object storage, reservations, the team-ai upstream and the screening term file. `ENV` SHALL have no default, `GRPC_PORT` SHALL have no default and be required, and `DATABASE_URL`, `UPSTREAM_AI_ADDR` and `UPSTREAM_AUDIT_ADDR` SHALL be required in every `ENV` (an empty value refuses start-up naming the variable; no fallback mode exists for an unset upstream). The settings SHALL NOT declare `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `IDENTITY_CONSUMER_ENABLED`, `OUTBOX_MAX_ATTEMPTS`, `OUTBOX_CLAIM_LOCK_SECONDS`, `SHUTDOWN_GRACE_SECONDS` or `UPSTREAM_CALL_TIMEOUT_SECONDS`, and the only variable ending in `_ENABLED` SHALL be `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`. `GRPC_REFLECTION_ENABLED` SHALL default to false. When `ENV` is staging, stage, prod or production the service SHALL refuse to start, before any other group is validated and naming the variable, when `GRPC_REFLECTION_ENABLED` is true, `KAFKA_BROKERS` or `LOG_KEY_FP_SECRET` is empty, or `STORAGE_ACCESS_KEY` or `STORAGE_SECRET_KEY` is `minioadmin`. In `ENV=local` or `test` reflection and an empty `KAFKA_BROKERS` or `LOG_KEY_FP_SECRET` SHALL be accepted (defaults fill in), while the two upstream addresses stay required. A non-empty `SCREEN_TERMS_FILE` that cannot be read SHALL be refused naming the variable, a negative `SHUTDOWN_GRACE` or `SHUTDOWN_DRAIN_DELAY` SHALL be refused naming the variable, and a `GRPC_PORT` of 0 or less is refused by the SDK. The environment name SHALL be normalised by trimming whitespace and lower-casing, so `ENV=" Production "` is strict. `.env.example` SHALL list exactly the declared variables.

#### Scenario: Domain SDK: A missing ENV is refused
- **WHEN** settings load with `ENV` unset and `DATABASE_URL`, `GRPC_PORT` set
- **THEN** `Load` returns an error naming `ENV` and no default environment is assumed
- **VERIFIED BY**: team-domain `internal/config` `TestLoadRequiresEnv`

#### Scenario: Domain SDK: A missing GRPC_PORT is refused
- **WHEN** settings load with `ENV=local` and `DATABASE_URL` set and `GRPC_PORT` unset
- **THEN** `Load` returns an error naming `GRPC_PORT` and no port is assumed
- **VERIFIED BY**: team-domain `internal/config` `TestLoadRequiresGRPCPort`

#### Scenario: Domain SDK: The telemetry service name is team-domain
- **WHEN** `app.Run` starts with `OTEL_SERVICE_NAME` unset and a capturing tracer exporter, and again with `OTEL_SERVICE_NAME=custom`
- **THEN** the first run exports spans with resource `service.name` `team-domain` and the second with `custom`
- **VERIFIED BY**: team-domain `internal/app` `TestOtelServiceNameDefaultsToTeamDomain`

#### Scenario: Domain SDK: A missing database URL is refused in every environment
- **WHEN** settings load with `ENV=local` and `DATABASE_URL` unset
- **THEN** `Load` returns an error naming `DATABASE_URL`
- **VERIFIED BY**: team-domain `internal/config` `TestLoadRequiresDatabaseURL`

#### Scenario: Domain SDK: A missing screening terms file is refused
- **WHEN** settings load with `ENV=local` and `SCREEN_TERMS_FILE` set to a path that does not exist, and again with it set to an existing file and with it unset
- **THEN** the first load returns an error naming `SCREEN_TERMS_FILE`; the other two succeed
- **VERIFIED BY**: team-domain `internal/config` `TestScreenTermsFileMissingFails`

#### Scenario: Domain SDK: Port and shutdown durations are range checked
- **WHEN** settings load with `GRPC_PORT=0`, and again with `SHUTDOWN_GRACE=-1s`, and again with `SHUTDOWN_DRAIN_DELAY=-1s`
- **THEN** each load returns an error naming `GRPC_PORT`, `SHUTDOWN_GRACE` and `SHUTDOWN_DRAIN_DELAY` respectively
- **VERIFIED BY**: team-domain `internal/config` `TestLoadRejectsBadPortAndNegativeDurations`

#### Scenario: Domain SDK: Removed variables are not declared
- **WHEN** the declared variable names of `Settings` are listed
- **THEN** none of `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `IDENTITY_CONSUMER_ENABLED`, `OUTBOX_MAX_ATTEMPTS`, `OUTBOX_CLAIM_LOCK_SECONDS`, `SHUTDOWN_GRACE_SECONDS`, `UPSTREAM_CALL_TIMEOUT_SECONDS` is present, and the declared names ending in `_ENABLED` are exactly `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-domain `internal/config` `TestNoRemovedEnvKeysDeclared`

#### Scenario: Domain SDK: A strict ENV refuses each unsafe value
- **WHEN** settings load with `ENV` in staging, stage, prod, production and `" Production "` (surrounding whitespace, mixed case), once per case: `GRPC_REFLECTION_ENABLED=true`; `KAFKA_BROKERS` empty; `LOG_KEY_FP_SECRET` empty; `UPSTREAM_AUDIT_ADDR` empty; `UPSTREAM_AI_ADDR` empty; `STORAGE_ACCESS_KEY=minioadmin`; `STORAGE_SECRET_KEY=minioadmin`; `DATABASE_URL` empty
- **THEN** every case returns an error that names its variable (the SDK only refuses an empty `DATABASE_URL`; it has no notion of a default URL, so none is refused), and the SDK-refused variables are reported even when `DATABASE_URL` is unset
- **VERIFIED BY**: team-domain `internal/config` `TestStrictEnvRefusesUnsafeSettings`

#### Scenario: Domain SDK: A local ENV accepts reflection and fills defaults
- **WHEN** settings load with `ENV=local`, `UPSTREAM_AUDIT_ADDR` and `UPSTREAM_AI_ADDR` set, `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` unset, and again with `GRPC_REFLECTION_ENABLED=true`
- **THEN** both loads succeed, `KAFKA_BROKERS` is `localhost:9092`, reflection is false in the first load and true in the second
- **VERIFIED BY**: team-domain `internal/config` `TestLocalEnvAcceptsDefaultsAndReflection`

#### Scenario: Domain SDK: An unset upstream address is refused in every environment
- **WHEN** settings load with `ENV=local` and `UPSTREAM_AI_ADDR` unset, and again with `ENV=local` and `UPSTREAM_AUDIT_ADDR` unset (everything else valid)
- **THEN** each load returns an error naming `UPSTREAM_AI_ADDR` and `UPSTREAM_AUDIT_ADDR` respectively
- **VERIFIED BY**: team-domain `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

#### Scenario: Domain SDK: .env.example matches the declared variables
- **WHEN** `make check-env` runs `TestEnvExampleInSync`
- **THEN** every declared variable is in `.env.example` and no key outside the declared set is, so a stale `KAFKA_ENABLED` line fails the check
- **VERIFIED BY**: team-domain `internal/config` `TestEnvExampleInSync`

### Requirement: Every dependency is used and none has a runtime fallback

`app.Run` SHALL require both upstreams (team-ai through `UPSTREAM_AI_ADDR`, team-audit through `UPSTREAM_AUDIT_ADDR`) in every `ENV`, SHALL open Postgres with `postgres.Open`, build the Kafka producer, and start the outbox relayer, the reservation sweeper and the identity consumer in every `ENV`, with no variable that disables any of them. An unreachable database SHALL stop the process with a non-nil error within 5 s (the `postgres.Open` ping bound) plus 1 s margin in every `ENV`. Non-test code under `team-domain` SHALL contain no `NoopPublisher`, `NewInMemory`, `RequireDurableStorage` or a nil-pool branch; in-memory fakes SHALL live only in `internal/store/storetest`, imported only from `_test.go` files. team-domain has no mock switch, so no `MOCK_DATA` adapter is declared; the real code path always runs.

#### Scenario: Domain SDK: An unreachable database stops boot
- **WHEN** `app.Run` starts with `ENV=local` and a `DATABASE_URL` that refuses connections
- **THEN** `Run` returns an error naming the database within 6 s (5 s ping bound plus 1 s margin) and no listener was opened
- **VERIFIED BY**: team-domain `internal/app` `TestRunFailsOnUnreachableDatabase`

#### Scenario: Domain SDK: The sweeper releases expired reservations without any switch
- **WHEN** the service runs with `RESERVATION_SWEEP_INTERVAL=200ms` and an active reservation past its TTL exists
- **THEN** the reservation is `released` and the stock restored within 5 s
- **VERIFIED BY**: team-domain `internal/app` `TestRunSweepsExpiredReservations`

#### Scenario: Domain SDK: Non-test code has no runtime fallback
- **WHEN** every non-test `.go` file under `team-domain` is scanned
- **THEN** none contains `NoopPublisher`, `NewInMemory`, `RequireDurableStorage`, an import of `internal/store/storetest` or the strings `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `IDENTITY_CONSUMER_ENABLED`; the scan allows no nil-valued substitute for a dependency: a nil `screener`, a nil audit writer or any other nil-substitute for the pool, producer, audit writer or team-ai client fails the scan
- **VERIFIED BY**: team-domain `internal/app` `TestNoRuntimeFallbacksInSource`

#### Scenario: Domain SDK: In-memory fakes are imported by tests only
- **WHEN** the importers of `internal/store/storetest` are listed
- **THEN** every importing file ends in `_test.go`
- **VERIFIED BY**: team-domain `internal/app` `TestFakesImportedOnlyFromTests`

### Requirement: Readiness reflects dependencies and liveness does not

`app.Run` SHALL serve the gRPC health service from `healthx` with the single check `postgres` (Postgres only: Kafka is not a readiness check, and team-ai and team-audit failures are handled per call, so they are not readiness checks either). The overall status (`""`) SHALL be NOT_SERVING within two check intervals after a check fails and SERVING again after it recovers. The service name `liveness` SHALL be SERVING from construction, SHALL NOT depend on any check, and SHALL turn NOT_SERVING only when shutdown begins. On shutdown the health SHALL drain before the gRPC server stops. The deployed pod SHALL probe `liveness` for its liveness check (gitops `probes.livenessService: liveness`).

#### Scenario: Domain SDK: A database outage fails readiness and keeps liveness
- **WHEN** the postgres check fails while the service runs
- **THEN** a health `Check` with service `""` returns NOT_SERVING within 10 s and a `Check` with service `liveness` returns SERVING
- **VERIFIED BY**: team-domain `internal/app` `TestReadinessFailsLivenessHoldsWhenPostgresDown`

#### Scenario: Domain SDK: A Kafka outage does not fail readiness
- **WHEN** the Kafka broker is unreachable for 15 s while Postgres is healthy and the service runs
- **THEN** a health `Check` with service `""` stays SERVING and with service `liveness` stays SERVING, and the relayer keeps the rows `pending`
- **VERIFIED BY**: team-domain `internal/app` `TestReadinessIgnoresKafkaOutage`

#### Scenario: Domain SDK: Shutdown drains health before the server stops
- **WHEN** the run context is cancelled with `SHUTDOWN_DRAIN_DELAY=500ms`
- **THEN** health reports NOT_SERVING for both `""` and `liveness` at least 400 ms before the gRPC listener stops accepting
- **VERIFIED BY**: team-domain `internal/app` `TestShutdownDrainsHealthBeforeServerStops`

### Requirement: Components start and stop in a defined order

`app.Run` SHALL run the gRPC server, the outbox relayer, the reservation sweeper and the identity consumer under `lifecycle.Run`, with the relayer, sweeper and consumer stopping only after the gRPC server has stopped. A component that returns an error SHALL stop every other component and `Run` SHALL return that error. Closing SHALL release the producer, the pool and telemetry in reverse order of opening. The whole shutdown SHALL finish within `SHUTDOWN_DRAIN_DELAY` + `SHUTDOWN_GRACE`.

#### Scenario: Domain SDK: An in-flight write is relayed during shutdown
- **WHEN** the run context is cancelled while a CreateListing call is in flight
- **THEN** the call completes with OK and the `ListingChanged` row it enqueued is `published` before `Run` returns
- **VERIFIED BY**: team-domain `internal/app` `TestShutdownRelaysInFlightWrite`

#### Scenario: Domain SDK: A failing component stops the process with its error
- **WHEN** the identity consumer returns an error after a record could be neither handled nor parked
- **THEN** `Run` returns that error, the gRPC server has stopped and the pool is closed
- **VERIFIED BY**: team-domain `internal/app` `TestConsumerFailureStopsRun`

#### Scenario: Domain SDK: A hanging call cannot outlive the grace period
- **WHEN** the run context is cancelled while a handler blocks forever and `SHUTDOWN_GRACE=1s`
- **THEN** `Run` returns within 1.5 s plus `SHUTDOWN_DRAIN_DELAY`
- **VERIFIED BY**: team-domain `internal/app` `TestShutdownGraceBoundsHangingCall`

### Requirement: The gRPC server uses the SDK chain, principal checks and error table

`team-domain` SHALL build its server with `grpcx.NewServer` and register `ListingService` on it, resolving the caller from the gateway-forwarded `x-principal-id`, `x-principal-type` and `x-principal-scopes` metadata with the SDK `auth` interceptor. The authorization matrix SHALL stay as it is: `listing.read` for reads, `listing.write` for writes of the owner (owner or `admin` scope for update and delete), the `admin` scope for `ModerateListing` and `ListModerationQueue`, and a SERVICE principal with `listing.write` for `ReserveStock`, `ReleaseStock` and `CommitReservation`. No principal SHALL give `Unauthenticated`; a missing scope or a principal of the wrong type SHALL give `PermissionDenied`. Every handler SHALL return errors through `grpcx.Status` with one ordered error table: domain sentinels keep their current codes and messages; a PostgreSQL unique violation (SQLSTATE 23505) SHALL give `AlreadyExists`; a cancelled context SHALL give `Canceled`; an expired deadline SHALL give `DeadlineExceeded`; any other error SHALL give `Internal` "internal error" with no driver or upstream text. gRPC reflection SHALL be off unless `GRPC_REFLECTION_ENABLED=true`.

#### Scenario: Domain SDK: A call without a principal is Unauthenticated
- **WHEN** an RPC that requires a scope (CreateListing, UpdateListing, ModerateListing, ReserveStock) is called with no `x-principal-id`
- **THEN** each returns `Unauthenticated`
- **VERIFIED BY**: team-domain `internal/rpc` `TestRPCsWithoutPrincipalAreUnauthenticated`

#### Scenario: Domain SDK: A user principal cannot call a reservation RPC
- **WHEN** a seller principal of type `user` holding `listing.write` calls ReserveStock, ReleaseStock and CommitReservation
- **THEN** each returns `PermissionDenied` and the stock is unchanged
- **VERIFIED BY**: team-domain `internal/rpc` `TestUserPrincipalCannotCallReservationRPCs`

#### Scenario: Domain SDK: A seller cannot moderate
- **WHEN** a seller principal without the `admin` scope calls ModerateListing and ListModerationQueue
- **THEN** each returns `PermissionDenied` and no audit write is attempted
- **VERIFIED BY**: team-domain `internal/rpc` `TestSellerCannotModerateOrReadQueue`

#### Scenario: Domain SDK: A unique violation is AlreadyExists
- **WHEN** a store call returns a `pgconn.PgError` with SQLSTATE 23505 that no sentinel wraps
- **THEN** the RPC returns `AlreadyExists` and the message does not contain the constraint name
- **VERIFIED BY**: team-domain `internal/rpc` `TestUniqueViolationMapsAlreadyExists`

#### Scenario: Domain SDK: A duplicate storefront slug is still AlreadyExists
- **WHEN** a seller upserts a storefront whose slug another seller holds
- **THEN** the RPC returns `AlreadyExists` with message `slug already taken`
- **VERIFIED BY**: team-domain `internal/rpc` `TestUpsertStorefrontSlugTakenAlreadyExists`

#### Scenario: Domain SDK: Cancelled and expired calls keep their codes
- **WHEN** the handler's context is cancelled mid-call, and in a second case its deadline expires
- **THEN** the first returns `Canceled` and the second `DeadlineExceeded`, neither is logged at error level
- **VERIFIED BY**: team-domain `internal/rpc` `TestContextErrorsKeepTheirCodes`

#### Scenario: Domain SDK: An unknown error hides driver text
- **WHEN** a store call fails with an error whose text is `connection to 10.0.0.9 refused`
- **THEN** the RPC returns `Internal` with message `internal error`, which does not contain `10.0.0.9`
- **VERIFIED BY**: team-domain `internal/rpc` `TestInternalErrorHidesDriverText`

#### Scenario: Domain SDK: Domain sentinels keep their codes
- **WHEN** a GetListing for an unknown id, a UpdateListing on a `held` listing by its owner, and a ReserveStock for a released `reservation_id` are called
- **THEN** they return `NotFound` "not_found", `FailedPrecondition` and `FailedPrecondition` respectively
- **VERIFIED BY**: team-domain `internal/rpc` `TestDomainSentinelsKeepCodes`

#### Scenario: Domain SDK: Reflection is off by default
- **WHEN** the server is built from settings with `GRPC_REFLECTION_ENABLED` unset and a client lists services over reflection
- **THEN** the call returns `Unimplemented`
- **VERIFIED BY**: team-domain `internal/app` `TestReflectionOffByDefault`

### Requirement: Page sizes use the SDK clamp and cursors keep their format

`ListListings`, `ListMyListings` and `ListModerationQueue` SHALL clamp the requested page size with `pagination.Clamp(size, 50, 100)`: a size of 0 or less gives 50 and a size above 100 gives 100. The keyset cursors SHALL keep their current format (the last listing id for listings; the opaque `(submitted_at, id)` cursor for the moderation queue), so a cursor issued before this change SHALL still be accepted. A moderation-queue cursor that the store did not issue SHALL return `InvalidArgument`.

#### Scenario: Domain SDK: Page size defaults and caps
- **WHEN** ListListings is called with page size 0, 100, 101 and -5 over 150 published listings
- **THEN** the page lengths are 50, 100, 100 and 50
- **VERIFIED BY**: team-domain `internal/store` `TestPageSizeClamp`

#### Scenario: Domain SDK: A cursor issued before the migration is still accepted
- **WHEN** ListListings is called with the listing-id cursor value and ListModerationQueue with a cursor string captured from the pre-migration code
- **THEN** both return the page that follows the cursor with the same ordering as before
- **VERIFIED BY**: team-domain `internal/store` `TestExistingCursorFormatsAccepted`

#### Scenario: Domain SDK: A malformed queue cursor is rejected
- **WHEN** an admin calls ListModerationQueue with cursor `not-a-cursor`
- **THEN** the RPC returns `InvalidArgument`
- **VERIFIED BY**: team-domain `internal/rpc` `TestListModerationQueueInvalidCursor`

### Requirement: The binary applies migrations and the outbox table moves to the SDK v1 shape

The service binary SHALL provide the subcommand `migrate up` (SDK `pkg/migrate` over embedded migrations), which is the only way schema changes are applied: `app.Run` SHALL NOT run, create or alter any migration or table at boot, in any `ENV`. `server migrate up` SHALL load only `Runtime` and `Postgres` settings (so it needs `ENV` and `DATABASE_URL` and does not need `GRPC_PORT` or any Kafka, audit or storage variable), apply the embedded `migrations/*.sql` with `migrate.Up`, and exit 0 when nothing is pending. Migrations `0013_outbox_v1_columns`, `0014_outbox_v1_backfill`, `0015_outbox_v1_switch` and `0016_outbox_v1_finish` SHALL follow `outbox.UpgradeSQL("outbox_events")` (Prepare, Backfill, Switch, Finish), number existing rows by `(created_at, event_id)`, leave earlier migrations unmodified, and have down files that restore the previous columns and indexes. The migration set SHALL requeue rows parked as `failed` (status `pending`, attempts 0, last error cleared) in `0016_outbox_v1_finish`, whose down file cannot restore the `failed` marking. After the upgrade, rows that were `pending` or `failed` SHALL be published by the SDK relayer in `seq` order per listing id. An unreachable database SHALL make `migrate up` exit non-zero.

#### Scenario: Domain SDK: Migrations build the v1 outbox on an empty database
- **WHEN** `migrate up` runs against an empty database
- **THEN** `outbox_events` has the columns `topic`, `traceparent` and `seq` (NOT NULL, default from the sequence) and the indexes `outbox_events_agg_idx` and `outbox_events_pending_idx`, and `schema_migrations` is at 16
- **VERIFIED BY**: team-domain `migrations` `TestMigrateUpBuildsV1Outbox`

#### Scenario: Domain SDK: Existing rows are numbered by creation order
- **WHEN** a database at migration 0012 holds 5 `outbox_events` rows with `created_at` out of insertion order and `migrate up` runs
- **THEN** `seq` is strictly increasing in `(created_at, event_id)` order across all 5 rows
- **VERIFIED BY**: team-domain `migrations` `TestOutboxUpgradeNumbersRowsByCreatedAt`

#### Scenario: Domain SDK: Upgraded pending rows are relayed
- **WHEN** the 5 upgraded rows are `pending` and the service starts
- **THEN** all 5 are published to `listing.events` in `seq` order per listing id
- **VERIFIED BY**: team-domain `internal/app` `TestUpgradedPendingRowsArePublished`

#### Scenario: Domain SDK: Upgraded failed rows are requeued and published
- **WHEN** a database at migration 0012 holds one `failed` row for listing A followed by a `pending` row for listing A, and a `pending` row for listing B, and `migrate up` runs
- **THEN** immediately after `migrate up` all 3 rows have `status = 'pending'` and attempts 0, and once the service starts all 3 are `published` within 10 s, the two listing A rows in `seq` order
- **VERIFIED BY**: team-domain `internal/app` `TestUpgradedFailedRowsAreRequeuedAndPublished`

#### Scenario: Domain SDK: Boot does not migrate
- **WHEN** `app.Run` starts against an empty database and is cancelled after 3 s
- **THEN** the database still has no table (no `schema_migrations`, no `outbox_events`) and `Run` has created none
- **VERIFIED BY**: team-domain `internal/app` `TestRunDoesNotMigrate`

#### Scenario: Domain SDK: The migrate subcommand applies the schema
- **WHEN** the binary entry point runs with arguments `migrate up` and only `ENV=local` and `DATABASE_URL` set, against an empty database
- **THEN** it exits 0 and `schema_migrations` is at 16
- **VERIFIED BY**: team-domain `internal/app` `TestMigrateUpSubcommandAppliesSchema`

#### Scenario: Domain SDK: A second migrate up changes nothing
- **WHEN** `migrate up` runs twice in a row
- **THEN** the second run exits 0 and `schema_migrations` and the table contents are unchanged
- **VERIFIED BY**: team-domain `migrations` `TestMigrateUpIsIdempotent`

#### Scenario: Domain SDK: Migrate without a database fails
- **WHEN** `migrate up` runs with `DATABASE_URL` pointing at a closed port
- **THEN** it returns an error and the exit status is non-zero
- **VERIFIED BY**: team-domain `internal/app` `TestMigrateFailsWithoutDatabase`

### Requirement: Upstream calls use the SDK clients and audited actions fail closed

team-ai screening SHALL be called through `upstream.NewClient` with the service principal `service-team-domain`, type `service`, scope `ai.classify`, replacing any incoming principal, bounded by `UPSTREAM_AI_TIMEOUT_SECONDS` and by the inbound deadline minus 500 ms. An AI error or a timeout SHALL send the listing to `pending_review` (unchanged); an unset `UPSTREAM_AI_ADDR` is refused at start-up (no fallback). The audited actions (`ModerateListing`, admin `UpdateListing`, admin `DeleteListing`) SHALL run through `auditx.Gate.Do`: the audit event (`listing.moderate`, `listing.admin_update`, `listing.admin_delete`, target type `listing`, same metadata keys as before) SHALL be written before the change; on any write failure or timeout the RPC SHALL return `Unavailable` with message `audit trail unavailable`, the listing SHALL be unchanged and no outbox row SHALL be written; the message SHALL NOT contain the audit address or cause.

#### Scenario: Domain SDK: Screening calls present the service principal
- **WHEN** a seller publishes a listing and team-ai (fake) records the call metadata
- **THEN** the metadata has `x-principal-id` `service-team-domain`, `x-principal-type` `service`, `x-principal-scopes` `ai.classify` and nothing of the seller
- **VERIFIED BY**: team-domain `internal/screen` `TestScreenListingUsesServicePrincipal`

#### Scenario: Domain SDK: A hung team-ai is cut off at the bound
- **WHEN** team-ai does not answer within 300 ms with `UPSTREAM_AI_TIMEOUT_SECONDS=0.3`
- **THEN** the call returns `DeadlineExceeded` within 2 s and the publish falls back to `pending_review`
- **VERIFIED BY**: team-domain `internal/screen` `TestScreenListingTimeoutBounded`

#### Scenario: Domain SDK: An admin delete is refused when audit is down
- **WHEN** team-audit returns an error and an admin calls DeleteListing on another seller's listing
- **THEN** the RPC returns `Unavailable` with message `audit trail unavailable`, the listing still exists and 0 outbox rows are added
- **VERIFIED BY**: team-domain `internal/rpc` `TestAdminDeleteAuditDownUnavailableNoChange`

#### Scenario: Domain SDK: The audit failure message hides the cause
- **WHEN** the audit write fails with an error text containing `10.0.0.9`
- **THEN** the returned message is exactly `audit trail unavailable`
- **VERIFIED BY**: team-domain `internal/rpc` `TestAuditFailureHidesCause`

#### Scenario: Domain SDK: A non-admin owner write does not touch audit
- **WHEN** a seller updates their own listing while team-audit is down
- **THEN** the RPC returns OK and the audit fake received 0 calls
- **VERIFIED BY**: team-domain `internal/rpc` `TestOwnerUpdateIndependentOfAudit`
