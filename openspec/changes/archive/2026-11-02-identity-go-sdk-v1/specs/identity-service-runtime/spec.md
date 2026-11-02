## ADDED Requirements

### Requirement: team-identity builds on the tagged go-sdk with the ADR-0016 layout

`team-identity/go.mod` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at `v1.0.2`, SHALL contain no `replace` directive and SHALL pin golangci-lint with a `tool` directive. The repo SHALL contain no `third_party` directory. The code SHALL be laid out as `cmd/server/main.go`, `internal/app`, `internal/config`, `internal/rpc`, `internal/identity`, `internal/store`, `internal/events`, `internal/upstream`, `internal/token` and `internal/authz`; the packages `internal/handler`, `internal/service`, `internal/repository`, `internal/interceptor`, `internal/grpcserver`, `internal/bootstrap` and `internal/observability` SHALL NOT exist. `internal/identity` SHALL NOT import `google.golang.org/grpc`. The `Makefile` SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`.

#### Scenario: The module requires the v1.0.2 tag
- **WHEN** the layout test reads `go.mod`
- **THEN** it finds the require line for the SDK module at `v1.0.2`, the golangci-lint `tool` line, no `replace` line and no `third_party` directory
- **VERIFIED BY**: team-identity `internal/app` `TestModuleRequiresSDKTag`

#### Scenario: A legacy package or a transport import in the domain is refused
- **WHEN** the layout test lists `internal/` and the imports of `internal/identity`
- **THEN** none of the seven legacy package directories exists and `internal/identity` does not import `google.golang.org/grpc`, otherwise the test fails
- **VERIFIED BY**: team-identity `internal/app` `TestLayoutFollowsADR0016`

#### Scenario: The Makefile uses the shared files without copies
- **WHEN** the layout test reads `Makefile` and lists the repo root
- **THEN** the Makefile includes `../platform-core/make/go-service.mk`, names `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, and no `.golangci.yml` or `go-service.mk` file exists under `team-identity`
- **VERIFIED BY**: team-identity `internal/app` `TestMakefileUsesSharedFiles`

### Requirement: Settings come from the environment without dependency flags

`config.Load` SHALL read the SDK groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox`, `Audit` and `LogKeyFP` and the service groups for `IDENTITY_EVENTS_TOPIC`, the JWT and JWKS settings, `SEED_ADMIN_*`, `PASSWORD_RESET_EXPOSE_TOKEN`, the retention durations and the outbox purge (`OUTBOX_PURGE_RETENTION`, `OUTBOX_PURGE_INTERVAL`). `UPSTREAM_AUDIT_ADDR` SHALL be required in every ENV (the audit client is an upstream the service uses; no fallback mode). A migrate-only loader (`config.LoadMigrate`: `ENV` and `DATABASE_URL`) SHALL serve the `migrate up` subcommand. `ENV`, `GRPC_PORT`, `DATABASE_URL`, `JWT_PRIVATE_KEY` and `JWT_KID` SHALL be required with no default; a missing or empty value SHALL make `Load` return an error that names the variable. The service SHALL declare no `*_ENABLED` variable other than `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` and `SEED_ADMIN_ENABLED`, SHALL NOT declare `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `OUTBOX_MAX_ATTEMPTS`, `OUTBOX_CLAIM_LOCK_SECONDS`, `SESSION_SWEEP_ENABLED`, `UPSTREAM_CALL_TIMEOUT_SECONDS`, `SHUTDOWN_GRACE_SECONDS` or `MOCK_DATA`, and SHALL ignore their values when set. `.env.example` SHALL list exactly the declared keys.

#### Scenario: Load reads the environment
- **WHEN** `ENV=local`, `GRPC_PORT=51053`, `DATABASE_URL=postgres://x/y`, `UPSTREAM_AUDIT_ADDR=audit:50061`, a valid `JWT_PRIVATE_KEY` and `JWT_KID=k1` are set
- **THEN** `Load` returns `GRPC.Port` 51053, `Postgres.URL` `postgres://x/y`, `Postgres.MaxConns` 10, `OTel.Enabled` false, `JWT.TTLSeconds` 3600, `JWKS` port 50063, `Kafka.Brokers` `localhost:9092`, `Audit.Addr` the given `UPSTREAM_AUDIT_ADDR` and `Outbox` purge retention 168 h every 1 h
- **VERIFIED BY**: team-identity `internal/config` `TestLoadReadsEnvironment`

#### Scenario: A missing database URL fails the load
- **WHEN** every required variable except `DATABASE_URL` is set
- **THEN** `Load` returns an error whose text contains `DATABASE_URL`
- **VERIFIED BY**: team-identity `internal/config` `TestLoadRequiresDatabaseURL`

#### Scenario: A missing port, environment, key or key id fails the load
- **WHEN** `Load` runs four times, each with one of `GRPC_PORT`, `ENV`, `JWT_PRIVATE_KEY` and `JWT_KID` empty
- **THEN** each call returns an error naming that variable
- **VERIFIED BY**: team-identity `internal/config` `TestValidateRequiresPrivateKeyAndKID`

#### Scenario: No dependency enable flag is declared
- **WHEN** the declared environment keys of `Settings` are listed
- **THEN** every key ending in `_ENABLED` is one of `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` and `SEED_ADMIN_ENABLED`, and none of the nine removed variable names is present
- **VERIFIED BY**: team-identity `internal/config` `TestNoDependencyEnableFlags`

#### Scenario: Removed variables are ignored when set
- **WHEN** `DATABASE_ENABLED=false`, `KAFKA_ENABLED=false`, `OUTBOX_ENABLED=false` and `SESSION_SWEEP_ENABLED=false` are set with a valid environment
- **THEN** `Load` succeeds and the resulting settings equal those of the same environment without them
- **VERIFIED BY**: team-identity `internal/config` `TestRemovedVariablesAreIgnored`

#### Scenario: The audit address is required in every ENV
- **WHEN** `Load` runs with every other variable valid and `UPSTREAM_AUDIT_ADDR` empty, once with `ENV=local` and once with `ENV=production`
- **THEN** both calls return an error naming `UPSTREAM_AUDIT_ADDR`
- **VERIFIED BY**: team-identity `internal/config` `TestAuditAddrRequiredInEveryEnv`

#### Scenario: Purge settings are validated
- **WHEN** `OUTBOX_PURGE_RETENTION=0s`, and in a second case `OUTBOX_PURGE_INTERVAL=0s`, is loaded
- **THEN** each call returns an error naming the variable
- **VERIFIED BY**: team-identity `internal/config` `TestOutboxPurgeSettingsValidated`

#### Scenario: The migrate loader needs only the environment and the database
- **WHEN** `LoadMigrate` runs with only `ENV=local` and `DATABASE_URL` set, and again with `DATABASE_URL` empty
- **THEN** the first returns `Postgres.URL` and no error, and the second returns an error naming `DATABASE_URL`
- **VERIFIED BY**: team-identity `internal/config` `TestLoadMigrateNeedsOnlyDatabase`

#### Scenario: .env.example matches the declared keys
- **WHEN** `.env.example` is compared with the declared keys
- **THEN** every declared key appears once and no other key appears
- **VERIFIED BY**: team-identity `internal/config` `TestEnvExampleInSync`

### Requirement: Unsafe configuration is refused in staging and production

When `Runtime.IsStrict()` (ENV `staging`, `stage`, `prod`, `production`, any case), `config.Load` SHALL refuse `GRPC_REFLECTION_ENABLED=true`, `PASSWORD_RESET_EXPOSE_TOKEN=true`, an empty `KAFKA_BROKERS` and an empty `LOG_KEY_FP_SECRET`, each with an error naming the variable, SHALL refuse in the same call `JWT_KID` `dev-2026` and the committed development signing key (matched by public-key fingerprint, whatever the kid) with an error whose text contains `refusing to start` and names `JWT_KID` or `JWT_PRIVATE_KEY`, SHALL report every refusal of one load together instead of stopping at the first, and the reflection refusal SHALL come from the resolve step that runs before any group validation. `app.Run` SHALL repeat the signing-key refusal, with the same text, before it opens any connection. When `ENV` is `local` or `test` the same values SHALL be accepted, `KAFKA_BROKERS` SHALL default to `localhost:9092`. `GRPC_REFLECTION_ENABLED` SHALL default to false.

#### Scenario: Strict ENV refuses reflection before validation
- **WHEN** `ENV=production`, `GRPC_REFLECTION_ENABLED=true` and `DATABASE_URL` is missing
- **THEN** `Load` returns the reflection refusal and not the `DATABASE_URL` error
- **VERIFIED BY**: team-identity `internal/config` `TestStrictEnvRefusesReflectionBeforeValidation`

#### Scenario: Reflection is off by default and accepted locally
- **WHEN** `ENV=local` is loaded once without `GRPC_REFLECTION_ENABLED` and once with it set to true
- **THEN** the first load has `ReflectionEnabled` false and the second has it true
- **VERIFIED BY**: team-identity `internal/config` `TestReflectionDefaultsOffAndLocalAccepts`

#### Scenario: Strict ENV requires the brokers and log fingerprint secret
- **WHEN** `ENV` is `staging` and, in turn, `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` are empty with every other variable valid
- **THEN** each `Load` returns an error naming the empty variable, and the same two cases with `ENV=local` succeed
- **VERIFIED BY**: team-identity `internal/config` `TestStrictEnvRequiresDependencyAddresses`

#### Scenario: The reset token is never exposed in a deployed ENV
- **WHEN** `PASSWORD_RESET_EXPOSE_TOKEN=true` is loaded with `ENV=staging` and with `ENV=production`, and with `ENV=local`
- **THEN** the first two loads return an error naming `PASSWORD_RESET_EXPOSE_TOKEN` and the third succeeds with the flag true
- **VERIFIED BY**: team-identity `internal/config` `TestPasswordResetExposeTokenDefaultsOffAndRefusedInProd`

#### Scenario: The development signing key is refused when settings load in a deployed ENV
- **WHEN** `ENV=production` is loaded with `JWT_KID=dev-2026`, and in a second case with the committed development key under another kid, in both cases with `LOG_KEY_FP_SECRET` empty
- **THEN** each `Load` returns one error whose text contains `refusing to start` and names `JWT_KID` or `JWT_PRIVATE_KEY` and also names `LOG_KEY_FP_SECRET`, and contains no key material
- **VERIFIED BY**: team-identity `internal/config` `TestStrictEnvRefusesDevSigningKey`

#### Scenario: Every strict refusal of one load is reported together
- **WHEN** `ENV=staging` is loaded with `PASSWORD_RESET_EXPOSE_TOKEN=true`, the development key, and empty `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET`
- **THEN** the single returned error names all four causes
- **VERIFIED BY**: team-identity `internal/config` `TestStrictEnvReportsAllRefusals`

#### Scenario: The development signing key is refused by Run in a deployed ENV
- **WHEN** `app.Run` starts with `ENV=production` and, in turn, `JWT_KID=dev-2026` with a fresh key, and the committed development key under another kid
- **THEN** each call returns an error whose text contains `refusing to start` and names `JWT_KID` or `JWT_PRIVATE_KEY` before any database connection is attempted, and the error text contains no key material
- **VERIFIED BY**: team-identity `internal/app` `TestRunRefusesDevSigningKeyBeforeConnecting`

#### Scenario: An unusable signing key fails boot in every ENV
- **WHEN** `app.Run` starts with `ENV=local` and `JWT_PRIVATE_KEY` that is not a PEM RSA key
- **THEN** it returns an error before connecting and nothing listens
- **VERIFIED BY**: team-identity `internal/app` `TestRunFailsOnUnparsableSigningKey`

#### Scenario: Seed admin keeps its guard
- **WHEN** `SEED_ADMIN_ENABLED=true` is loaded with `SEED_ADMIN_PASSWORD` empty, with 11 characters, with 12 characters, and `SEED_ADMIN_ENABLED` is unset
- **THEN** the first two loads return an error naming `SEED_ADMIN_PASSWORD`, the third succeeds, and the fourth succeeds with seeding off
- **VERIFIED BY**: team-identity `internal/config` `TestSeedAdminValidation`

### Requirement: The database is always on and migrations run only through the migrate subcommand

`app.Run` SHALL open Postgres with `postgres.Open` and SHALL return an error, in every ENV, when the database is unreachable. The service SHALL NOT apply migrations at boot. The binary SHALL provide the subcommand `server migrate up`, which applies the embedded migrations (`0001` to `0019`) with the SDK `migrate.Up` and exits 0 on success, and SHALL return a non-zero exit when a migration fails or the migration state is dirty. Compose SHALL run `team-identity-migrate` from the service image with `migrate up`, and gitops SHALL run it as an Argo PreSync Job (both in change `deploy-go-sdk-v1`). A database already migrated to version 8 by the old compose job SHALL be upgraded in place without changing existing rows of the identity tables; outbox rows the old relayer parked as `failed` SHALL be requeued as pending, and every unpublished outbox row SHALL join the per-subject aggregate, by migration `0019`, before the new relayer starts (the first rollout stops the old pods before the new ones start, see design D4). The server binary SHALL NOT link the in-memory fakes of `internal/store/storetest`.

#### Scenario: An unreachable database fails boot locally
- **WHEN** `app.Run` starts with `ENV=local` and a `DATABASE_URL` pointing at a closed port
- **THEN** it returns an error within 10 s and nothing listens on the gRPC or JWKS port
- **VERIFIED BY**: team-identity `internal/app` `TestRunFailsWhenDatabaseUnreachable`

#### Scenario: The server binary does not link the in-memory fakes
- **WHEN** `go list -deps ./cmd/server` runs
- **THEN** the output contains no package path ending in `storetest`
- **VERIFIED BY**: team-identity `internal/app` `TestServerBinaryExcludesFakes`

#### Scenario: An empty database is migrated by the subcommand
- **WHEN** `app.Migrate` (the code behind `server migrate up`) runs against an empty Postgres, and then `app.Run` starts
- **THEN** `schema_migrations` reports version 19 not dirty, and `Register` followed by `Login` through gRPC succeeds
- **VERIFIED BY**: team-identity `internal/app` `TestMigrateUpAppliesAllMigrations`

#### Scenario: The service never migrates at boot
- **WHEN** `app.Run` starts against an empty Postgres that no one migrated
- **THEN** the `schema_migrations` table and the `users` table do not exist after `Run` has started and been cancelled
- **VERIFIED BY**: team-identity `internal/app` `TestRunDoesNotMigrate`

#### Scenario: The binary dispatches migrate up
- **WHEN** `main` is run with the arguments `migrate up` against an empty Postgres, with `migrate` alone, and with `migrate down`
- **THEN** the first exits 0 and leaves version 19, the other two exit non-zero with a usage line and change nothing, and none of the three starts a listener
- **VERIFIED BY**: team-identity `cmd/server` `TestMigrateSubcommandDispatch`

#### Scenario: A database already at version 8 is upgraded without data loss
- **WHEN** a database migrated to version 8 holds 3 users, 2 sessions and 4 pending outbox rows, and `app.Migrate` runs
- **THEN** the users and sessions are unchanged, the 4 outbox rows are still pending, `schema_migrations` reports version 19, and every row has a unique `seq` in `(created_at, event_id)` order
- **VERIFIED BY**: team-identity `internal/app` `TestMigrateUpgradesVersionEightDatabase`

#### Scenario: A dirty migration state fails the subcommand
- **WHEN** `schema_migrations.dirty` is true and `app.Migrate` runs
- **THEN** it returns an error naming the dirty version and applies nothing
- **VERIFIED BY**: team-identity `internal/app` `TestMigrateFailsWhenStateIsDirty`

#### Scenario: A failed migration leaves no half-started server
- **WHEN** migration `0019` fails during `app.Migrate`
- **THEN** it returns an error and nothing listens on the gRPC or JWKS port, because `Migrate` never starts a listener
- **VERIFIED BY**: team-identity `internal/app` `TestMigrateFailsWhenStateIsDirty`

#### Scenario: The outbox migrations equal the SDK upgrade steps
- **WHEN** the statements of migrations `0009` to `0018` are concatenated
- **THEN** they equal the statements of `outbox.UpgradeSQL("identity_outbox_events")` in order, and every `CREATE INDEX CONCURRENTLY` and the `DROP INDEX CONCURRENTLY` is alone in its file
- **VERIFIED BY**: team-identity `internal/store` `TestOutboxMigrationsEqualUpgradeSQL`

#### Scenario: The outbox upgrade keeps pending events in order
- **WHEN** rows `a`, `b`, `c` of one user are pending in that order before migration `0009` (rows `a` and `b` with `aggregate_type` `Session`, `c` with `User`) and a row `d` is enqueued after `0019`
- **THEN** the relayer publishes `a`, `b`, `c`, `d` in that order
- **VERIFIED BY**: team-identity `internal/store` `TestOutboxUpgradeKeepsPendingOrder`

#### Scenario: A row parked by the old relayer is relayed after the upgrade
- **WHEN** a database at version 8 holds a `SessionRevoked` row with `status` `failed`, `attempts` 10 and an error text, followed by a later pending `UserStatusChanged` row of the same user, and migrations `0009` to `0019` run
- **THEN** the first row has `status` `pending`, `attempts` 0 and no error, and a relayer pass publishes it before the `UserStatusChanged` record
- **VERIFIED BY**: team-identity `internal/store` `TestOutboxUpgradeRequeuesParkedRows`

#### Scenario: Unpublished rows of the old aggregate join the user aggregate
- **WHEN** a database at version 8 holds a pending `SessionRevoked` row with `aggregate_type` `Session` and a published one, and migration `0019` runs, then a `UserStatusChanged` row of the same user is enqueued
- **THEN** the pending row has `aggregate_type` `User`, the published row is unchanged, and the relayer publishes the `SessionRevoked` record before the `UserStatusChanged` record
- **VERIFIED BY**: team-identity `internal/store` `TestOutboxUpgradeUnifiesAggregateType`

#### Scenario: Migrations 0001 to 0008 keep their down steps
- **WHEN** migration `0007_user_status` runs up on a database holding users and then down
- **THEN** every existing user reads `active` after up, and down drops the four columns and the index
- **VERIFIED BY**: team-identity `internal/store` `TestMigration0007UserStatusUpDown`

### Requirement: Readiness follows Postgres and liveness does not

`healthx` SHALL serve the overall status under the empty service name (readiness) and a `liveness` service that stays SERVING while the process runs. The only registered readiness check SHALL be `postgres`; Kafka is not a readiness check in any service. A Kafka outage SHALL NOT change readiness. On shutdown both names SHALL turn NOT_SERVING before the gRPC server stops.

#### Scenario: Readiness fails while liveness stays serving
- **WHEN** the connection pool of a running service is closed
- **THEN** within `healthx.DefaultInterval` plus 1 s the empty service name reports NOT_SERVING and `liveness` still reports SERVING
- **VERIFIED BY**: team-identity `internal/app` `TestReadinessFollowsDatabaseLivenessDoesNot`

#### Scenario: A Kafka outage does not fail readiness or login
- **WHEN** the brokers are unreachable and a user calls `Login` and `RevokeSession`
- **THEN** both calls succeed, the `SessionRevoked` row stays pending, and the empty service name reports SERVING
- **VERIFIED BY**: team-identity `internal/app` `TestKafkaOutageKeepsReadinessAndAuth`

#### Scenario: Shutdown drains health first
- **WHEN** the process context is cancelled while a call is in flight
- **THEN** both health names report NOT_SERVING before the in-flight call completes and `Run` returns nil within `SHUTDOWN_GRACE`
- **VERIFIED BY**: team-identity `internal/app` `TestShutdownDrainsHealthFirst`

#### Scenario: An unknown health service name is not found
- **WHEN** a health check asks for the service name `nope`
- **THEN** the call returns `NotFound`
- **VERIFIED BY**: team-identity `internal/app` `TestUnknownHealthServiceNotFound`

### Requirement: Authorization and errors go through the SDK

Every handler SHALL read the caller with the SDK `auth` package, and SHALL authorize before it validates input. `Login`, `Register` and `RefreshSession` SHALL need no principal. `SessionService`, `AddressService` and the self-service `AuthService` RPCs SHALL need an authenticated user principal. `AccountAdminService` SHALL need scope `admin`. `GetPublicProfiles` SHALL need a service principal holding `identity.read`. A missing or anonymous principal SHALL return `Unauthenticated`; a wrong principal type or a missing scope SHALL return `PermissionDenied`. Errors SHALL leave through one `grpcx.ErrorTable`: a cancelled request SHALL return `Canceled`, a timed-out request `DeadlineExceeded`, an unmapped unique violation (`23505`) `AlreadyExists`, and any other unmapped error `Internal` with the message `internal error` and no driver text. The explicit messages `tài khoản bị khóa`, `username already exists`, `invalid credentials` and the generic refresh message SHALL be unchanged.

#### Scenario: Public RPCs need no principal
- **WHEN** `Register`, `Login` and `RefreshSession` are called over the wire with no principal metadata
- **THEN** none returns `Unauthenticated` for the missing principal
- **VERIFIED BY**: team-identity `internal/rpc` `TestRefreshSessionIsPublic`

#### Scenario: A protected RPC refuses an anonymous or wrong caller
- **WHEN** each protected RPC is called with no principal, with the gateway's anonymous principal, with a user lacking `admin` (admin RPCs) and with a user principal (profile RPC)
- **THEN** the no-principal and anonymous calls return `Unauthenticated` and the others return `PermissionDenied`
- **VERIFIED BY**: team-identity `internal/rpc` `TestAuthorizationBoundary`

#### Scenario: Authorization precedes validation
- **WHEN** a non-admin calls `SetUserStatus` and `ListUsers` with invalid input
- **THEN** both return `PermissionDenied` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/rpc` `TestSetUserStatusRequiresAdminScope` and team-identity `internal/rpc` `TestListUsersRequiresAdminScope`

#### Scenario: A cancelled request returns Canceled
- **WHEN** a client cancels `ListSessions` while the repository blocks
- **THEN** the call ends with code `Canceled`
- **VERIFIED BY**: team-identity `internal/rpc` `TestCanceledRequestMapsToCanceled`

#### Scenario: A timed-out request returns DeadlineExceeded
- **WHEN** a client calls `ListSessions` with a 50 ms deadline while the repository blocks
- **THEN** the call ends with code `DeadlineExceeded`
- **VERIFIED BY**: team-identity `internal/rpc` `TestDeadlineExceededMapsToDeadlineExceeded`

#### Scenario: A duplicate username returns the explicit message
- **WHEN** a user registers with a username that exists
- **THEN** the call returns `AlreadyExists` with the message `username already exists`
- **VERIFIED BY**: team-identity `internal/rpc` `TestRegisterDuplicateUsernameMessage`

#### Scenario: An unmapped unique violation returns AlreadyExists
- **WHEN** a fake store returns a `23505` error that no sentinel claims during `CreateAddress`
- **THEN** the call returns `AlreadyExists` and the message does not contain the constraint name
- **VERIFIED BY**: team-identity `internal/rpc` `TestUniqueViolationMapsToAlreadyExists`

#### Scenario: A storage failure is generic and not leaked
- **WHEN** a fake store fails with an error whose text contains `password=secret`
- **THEN** the call returns `Internal` with the message `internal error` and neither the response nor the captured log line returned to the client contains `password=secret`
- **VERIFIED BY**: team-identity `internal/rpc` `TestAddressHandlerInternalErrorsAreGeneric`

#### Scenario: A panicking handler does not stop the server
- **WHEN** a repository panics during one call and a second call follows
- **THEN** the first returns `Internal` and the second succeeds
- **VERIFIED BY**: team-identity `internal/rpc` `TestPanicIsRecoveredAsInternal`

#### Scenario: Reflection is not served by default
- **WHEN** a running service built from default settings is asked to list services over reflection
- **THEN** the call returns `Unimplemented`
- **VERIFIED BY**: team-identity `internal/app` `TestReflectionOffByDefault`

#### Scenario: Locked accounts keep their message
- **WHEN** a banned user logs in with the correct password
- **THEN** the call returns `PermissionDenied` with the message `tài khoản bị khóa`
- **VERIFIED BY**: team-identity `internal/rpc` `TestLoginBannedKeepsLockedMessage`

### Requirement: Identity events go through the SDK outbox

Every write that today enqueues an event (`RevokeSession`, logout, absolute-lifetime revoke, reuse revoke, refresh of a banned or suspended user, `SetUserStatus`, suspension lift) SHALL call `outbox.Store.EnqueueTx` on the transaction of the business write, with `AggregateType` `User`, `AggregateID` the user id, the unchanged `EventID` and the unchanged envelope bytes, so the row commits or rolls back with the write. The `outbox.Relayer` over `kafkax.Producer` SHALL publish the rows to `IDENTITY_EVENTS_TOPIC` keyed by the user id with the headers `event-id`, `event-type`, `x-request-id` and `traceparent`. The service SHALL NOT contain a noop publisher or a flag that stops the relayer, and published rows older than the purge retention SHALL be removed (see the purge requirement). A `UserStatusChanged` row SHALL be published after every `SessionRevoked` row of the same call.

#### Scenario: A revoke commits with its outbox row
- **WHEN** `RevokeSession` runs against Postgres
- **THEN** the session has `revoked = true` and exactly one pending row of type `platform.identity.v1.SessionRevoked` with `aggregate_id` the user id exists, committed in the same transaction
- **VERIFIED BY**: team-identity `internal/store` `TestRevokeEnqueuesOutboxRowAtomically`

#### Scenario: An outbox failure rolls the write back
- **WHEN** the outbox insert fails during a revoke
- **THEN** the session stays active and no outbox row exists
- **VERIFIED BY**: team-identity `internal/store` `TestRevokeRollsBackWhenOutboxFails`

#### Scenario: Both event types share one aggregate and keep order
- **WHEN** `SetUserStatus` suspends a user with 3 active sessions
- **THEN** the 4 rows share `aggregate_type` `User` and the user id, and a relayer pass publishes the 3 `SessionRevoked` records before the `UserStatusChanged` record
- **VERIFIED BY**: team-identity `internal/store` `TestSetUserStatusOutboxOrderPG`

#### Scenario: Published records carry key, topic and headers
- **WHEN** the relayer publishes a `SessionRevoked` row enqueued with a request id and a trace
- **THEN** the record is on `IDENTITY_EVENTS_TOPIC`, its key is the user id, its value equals the stored envelope bytes, and it has the headers `event-id`, `event-type`, `x-request-id` and `traceparent`
- **VERIFIED BY**: team-identity `internal/app` `TestRelayedRecordCarriesKeyTopicAndHeaders`

#### Scenario: A broker outage keeps rows pending and retries without a limit
- **WHEN** the publisher fails 12 times with a transient error and then succeeds
- **THEN** the row is still pending after the 12th failure, is never parked, and is published on the 13th attempt
- **VERIFIED BY**: team-identity `internal/app` `TestTransientPublishFailuresNeverPark`

#### Scenario: The event payload stays minimal
- **WHEN** a `UserStatusChanged` envelope is decoded after a ban with a note
- **THEN** its fields are only the contract fields and the serialized payload contains neither the note text nor the username
- **VERIFIED BY**: team-identity `internal/events` `TestUserStatusChangedPayloadMinimal`

#### Scenario: A refused caller leaves no outbox row
- **WHEN** a caller without the admin scope calls `SetUserStatus` on a user with active sessions
- **THEN** the call returns `PermissionDenied`, the user and sessions are unchanged and the outbox holds 0 new rows
- **VERIFIED BY**: team-identity `internal/rpc` `TestSetUserStatusRequiresAdminScope`

#### Scenario: Relay failure logs carry a fingerprint, not the user id
- **WHEN** a row of user `u-123` fails to publish with `LOG_KEY_FP_SECRET=s3cret`
- **THEN** the log line carries `key_fp` equal to the first 12 hex of HMAC-SHA256(`s3cret`, `u-123`) and the whole captured output does not contain `u-123`
- **VERIFIED BY**: team-identity `internal/events` `TestRelayLogsFingerprintNotUserID`

### Requirement: Published outbox rows are purged on a schedule

The service SHALL run `outbox.Store.Purge` as a `lifecycle.Every` component with `RunAtStart` every `OUTBOX_PURGE_INTERVAL` (default 1 h), deleting rows published longer than `OUTBOX_PURGE_RETENTION` (default 168 h, 7 days) ago. It SHALL never delete a pending or parked row, SHALL log only the number deleted, SHALL keep running after a failed purge, and SHALL stop before the pool closes.

#### Scenario: Published rows older than seven days are purged
- **WHEN** the purge component ticks with rows published 8 days ago, rows published 6 days ago, a pending row and a parked row
- **THEN** only the 8-day-old rows are deleted and the other three remain
- **VERIFIED BY**: team-identity `internal/app` `TestOutboxPurgeKeepsRecentAndUnpublished`

#### Scenario: A failed purge is retried and logs counts only
- **WHEN** the purge fails on the first tick and succeeds on the second
- **THEN** the first tick logs one warning without the error text and returns nil, and the second deletes the eligible rows and logs `rows_purged`
- **VERIFIED BY**: team-identity `internal/app` `TestOutboxPurgeSurvivesError`

### Requirement: Privileged status changes are audit-first through the SDK gate

`SetUserStatus` SHALL run its change only through `auditx.Gate.Do`, so the audit write precedes the change, and a write error or a write timeout SHALL return `Unavailable` with the message `audit trail unavailable` without calling the change. The audit client SHALL send principal id `service-team-identity`, type `service` and scope `audit.write`. The gate logs SHALL contain no `target_id` and no error text.

#### Scenario: An audit failure refuses the change with the SDK message
- **WHEN** the audit writer returns an error during `SetUserStatus`
- **THEN** the call returns `Unavailable` with the message `audit trail unavailable` and the user row, sessions and outbox are unchanged
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusAuditDownUnavailableNoChange`

#### Scenario: The audit gate logs no target and no cause
- **WHEN** the audit write fails for a user whose id is `u-secret`
- **THEN** the captured log contains `action` and `target_type` and does not contain `u-secret` or the writer's error text
- **VERIFIED BY**: team-identity `internal/identity` `TestAuditGateLogsNoTargetID`

#### Scenario: A caller without the admin scope triggers no audit write
- **WHEN** a seller principal, and in a second case no principal, calls `SetUserStatus`
- **THEN** the first call returns `PermissionDenied`, the second returns `Unauthenticated`, and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/rpc` `TestSetUserStatusRequiresAdminScope`

#### Scenario: The audit client sends the service principal
- **WHEN** the audit writer writes an event while the incoming request carries an admin principal
- **THEN** the server side of the fake audit service receives `service-team-identity`, `service` and `audit.write` and not the admin principal
- **VERIFIED BY**: team-identity `internal/upstream` `TestAuditUsesServicePrincipal`

### Requirement: Tokens and the JWKS document follow the shared jwtx contract

The signer SHALL sign RS256 JWTs whose claims decode as `jwtx.Claims` (`name`, `typ`, `scopes`, `sid` only when non-empty, `sub`, `iat`, `exp`) and whose header carries the configured `kid`. The JWKS document SHALL be a `jwtx.JWKS` with one entry per public key (`kty` `RSA`, `use` `sig`, `alg` `RS256`, `kid`, `n`, `e`) and no private material. The listener on `JWKS_HTTP_PORT` SHALL serve `GET` and `HEAD` on `/.well-known/jwks.json` with `Cache-Control: public, max-age=300`, SHALL answer other methods with 405 and `Allow: GET, HEAD`, and SHALL stop with the process within `SHUTDOWN_GRACE`.

#### Scenario: A signed token decodes as the shared claims
- **WHEN** a token is signed for a buyer session and decoded with `jwtx.Claims`
- **THEN** `typ` is `user`, `sid` equals the session id, `scopes` equal the role scopes, `exp - iat` equals the TTL and the header `alg` is `RS256` with the configured `kid`
- **VERIFIED BY**: team-identity `internal/token` `TestSignRS256`

#### Scenario: A service token carries no sid
- **WHEN** a token is signed without a session id
- **THEN** the JSON payload has no `sid` key
- **VERIFIED BY**: team-identity `internal/token` `TestSignedTokenOmitsEmptySID`

#### Scenario: The JWKS document is public keys only
- **WHEN** the JWKS handler serves a key set of two keys
- **THEN** the body decodes as `jwtx.JWKS` with two entries carrying only `kty`, `use`, `alg`, `kid`, `n`, `e`, and its bytes equal the document served before the migration for the same key
- **VERIFIED BY**: team-identity `internal/token` `TestBuildJWKS`

#### Scenario: The JWKS listener refuses other methods
- **WHEN** a client sends `POST` to `/.well-known/jwks.json` and `GET` to `/other`
- **THEN** the first returns 405 with `Allow: GET, HEAD` and the second returns 404
- **VERIFIED BY**: team-identity `internal/token` `TestJWKSHandlerMethodsAndPaths`

#### Scenario: The listener starts with the service and drains on shutdown
- **WHEN** the service runs and then its context is cancelled
- **THEN** `GET /.well-known/jwks.json` on `JWKS_HTTP_PORT` returns 200 with the configured `kid` while running, and after `Run` returns the port refuses connections
- **VERIFIED BY**: team-identity `internal/app` `TestJWKSListenerServesAndDrains`

### Requirement: Logs identify the request and never carry secrets

Every service log line SHALL be JSON with `service` `team-identity`, and call logs SHALL carry the request id and, for an authenticated call, the principal id. The service SHALL NOT log a password, an access token, a refresh token, a refresh hash, a reset token, an address field, an ip, a user agent or a status note, in a log line or a span attribute.

#### Scenario: Call logs carry request id and principal id
- **WHEN** an authenticated `ListSessions` call with `x-request-id` `abc` completes
- **THEN** the captured call log has `request_id` `abc`, the principal id and `service` `team-identity`
- **VERIFIED BY**: team-identity `internal/app` `TestCallLogsCarryRequestAndPrincipalID`

#### Scenario: A refused call logs the principal and no request body
- **WHEN** a seller principal calls `SetUserStatus` with a marker note and is refused with `PermissionDenied` (403)
- **THEN** the captured call log carries the principal id and the code `PermissionDenied`, and zero captured lines or span attributes contain the note marker
- **VERIFIED BY**: team-identity `internal/app` `TestDeniedCallLogsCarryNoRequestBody`

#### Scenario: Secrets never reach logs or spans
- **WHEN** Register, Login, RefreshSession (rotation, grace replay and reuse), RequestPasswordReset and SetUserStatus with a marker note run with log and span capture enabled
- **THEN** zero captured log lines and span attributes contain the password, either token, the refresh hash, the reset token or the note marker
- **VERIFIED BY**: team-identity `internal/app` `TestSecretsNeverReachLogsOrSpans`

### Requirement: The retention sweeper is a lifecycle component

The service SHALL run the retention sweep as a `lifecycle.Every` component with `RunAtStart`, SHALL log only counts and durations, and SHALL keep running after a failed sweep.

#### Scenario: A failed sweep is retried on the next tick
- **WHEN** the purge fails on the first tick and succeeds on the second
- **THEN** the component logs one warning with counts only, does not return an error, and the second tick purges the eligible rows
- **VERIFIED BY**: team-identity `internal/identity` `TestSweeperSurvivesError`

#### Scenario: A non-positive interval is refused at load
- **WHEN** `SESSION_SWEEP_INTERVAL=0s` is loaded
- **THEN** `Load` returns an error naming `SESSION_SWEEP_INTERVAL`
- **VERIFIED BY**: team-identity `internal/config` `TestRetentionInvalidRejected`

### Requirement: The first admin is still seeded only on request

When `SEED_ADMIN_ENABLED=true`, `app.Run` SHALL ensure the admin account on the already migrated database before serving, using `SEED_ADMIN_USERNAME` and `SEED_ADMIN_PASSWORD`, and SHALL log a seeding error as a warning without the password. When it is unset or false it SHALL create no account.

#### Scenario: Default configuration creates no admin
- **WHEN** `app.Migrate` has run and `app.Run` starts without `SEED_ADMIN_ENABLED`
- **THEN** the `users` table has 0 rows with role `admin`
- **VERIFIED BY**: team-identity `internal/app` `TestRunSeedsNoAdminByDefault`

#### Scenario: Enabled seeding creates the admin once
- **WHEN** `app.Run` starts twice with `SEED_ADMIN_ENABLED=true` and a 12-character password
- **THEN** exactly one admin user exists and `Login` with those credentials returns an `admin` scope
- **VERIFIED BY**: team-identity `internal/app` `TestRunSeedsAdminOnce`
