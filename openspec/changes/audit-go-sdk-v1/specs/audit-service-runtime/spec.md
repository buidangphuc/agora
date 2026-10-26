## ADDED Requirements

### Requirement: team-audit builds on the tagged go-sdk with the ADR-0016 layout

`team-audit/go.mod` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at `v1.0.2` and SHALL contain no `replace` directive. The repo SHALL contain no `third_party` directory. The code SHALL be laid out as `cmd/server/main.go`, `internal/app`, `internal/config`, `internal/rpc`, `internal/audit` and `internal/store`; the packages `internal/handler`, `internal/service`, `internal/repository`, `internal/interceptor` and `internal/grpcserver` SHALL NOT exist. `internal/audit` SHALL NOT import `google.golang.org/grpc` or generated code. The `Makefile` SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`.

#### Scenario: The module requires the v1.0.2 tag
- **WHEN** the layout test reads `go.mod`
- **THEN** it finds the require line for the SDK module at `v1.0.2`, no `replace` line and no `third_party` directory
- **VERIFIED BY**: team-audit `internal/app` `TestModuleRequiresSDKTag`

#### Scenario: A legacy package or a transport import in the domain is refused
- **WHEN** the layout test lists `internal/` and the imports of `internal/audit`
- **THEN** none of the five legacy package directories exists and `internal/audit` imports neither `google.golang.org/grpc` nor `generated/`, otherwise the test fails
- **VERIFIED BY**: team-audit `internal/app` `TestLayoutFollowsADR0016`

#### Scenario: The Makefile uses the shared files without copies
- **WHEN** the layout test reads `Makefile` and lists the repo root
- **THEN** the Makefile includes `../platform-core/make/go-service.mk`, names `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, and no `.golangci.yml` or `go-service.mk` file exists under `team-audit`
- **VERIFIED BY**: team-audit `internal/app` `TestMakefileUsesSharedFiles`

### Requirement: Settings come from the environment without dependency flags or defaults for required values

`config.Load` SHALL read the SDK groups `Runtime`, `GRPC`, `Postgres` and `OTel`. `ENV`, `GRPC_PORT` and `DATABASE_URL` SHALL be required and SHALL have no default; a missing or empty value SHALL make `Load` return an error that names the variable. The service SHALL declare no `*_ENABLED` variable other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`, and SHALL declare neither `DATABASE_ENABLED` nor `MOCK_DATA`. `GRPC_REFLECTION_ENABLED` SHALL default to false and `Load` SHALL refuse true when `ENV` is staging, stage, prod or production, before any other validation. `.env.example` SHALL list exactly the declared keys.

#### Scenario: Load reads the environment
- **WHEN** `ENV=local`, `GRPC_PORT=51000` and `DATABASE_URL=postgres://x/y` are set
- **THEN** `Load` returns `Port` 51000 and `Postgres.URL` `postgres://x/y` with `DB_MAX_CONNS` 10 and `OTel.Enabled` false
- **VERIFIED BY**: team-audit `internal/config` `TestLoadReadsEnvironment`

#### Scenario: A missing database URL fails the load
- **WHEN** `ENV=local` and `GRPC_PORT=50066` are set and `DATABASE_URL` is missing
- **THEN** `Load` returns an error whose text contains `DATABASE_URL`
- **VERIFIED BY**: team-audit `internal/config` `TestLoadRequiresDatabaseURL`

#### Scenario: A missing port or environment fails the load
- **WHEN** `Load` runs once with `GRPC_PORT` empty and once with `ENV` empty
- **THEN** each call returns an error naming `GRPC_PORT` and `ENV` respectively
- **VERIFIED BY**: team-audit `internal/config` `TestLoadRequiresGRPCPort`

#### Scenario: Strict ENV refuses reflection before validation
- **WHEN** `ENV=production`, `GRPC_REFLECTION_ENABLED=true` and `DATABASE_URL` is missing
- **THEN** `Load` returns the reflection refusal and not the `DATABASE_URL` error
- **VERIFIED BY**: team-audit `internal/config` `TestStrictEnvRefusesReflectionBeforeValidation`

#### Scenario: Reflection is off by default and accepted locally
- **WHEN** `ENV=local` is loaded once without `GRPC_REFLECTION_ENABLED` and once with it set to true
- **THEN** the first load has `ReflectionEnabled` false and the second has it true
- **VERIFIED BY**: team-audit `internal/config` `TestReflectionDefaultsOffAndLocalAccepts`

#### Scenario: No dependency enable flag is declared
- **WHEN** the declared environment keys of `Settings` are listed
- **THEN** no key ending in `_ENABLED` other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED` exists and neither `DATABASE_ENABLED` nor `MOCK_DATA` is present
- **VERIFIED BY**: team-audit `internal/config` `TestNoDependencyEnableFlags`

#### Scenario: .env.example matches the declared keys
- **WHEN** `.env.example` is compared with the declared keys
- **THEN** every declared key appears once and no other key appears
- **VERIFIED BY**: team-audit `internal/config` `TestEnvExampleInSync`

### Requirement: The audit trail always uses Postgres and never an in-memory store

`app.Run` SHALL open Postgres with `postgres.Open`. When the database is unreachable `Run` SHALL return an error within 10 seconds in every `ENV`, including `local`, and SHALL NOT start a gRPC listener. No in-memory repository SHALL be linked into `cmd/server`; the in-memory repository SHALL exist only as the test fake `internal/store/storetest`. An acknowledged `WriteAuditEvent` SHALL be committed to the `audit_events` table before the response is sent.

#### Scenario: An unreachable database fails boot locally
- **WHEN** `Run` starts with `ENV=local` and a `DATABASE_URL` pointing at a closed port
- **THEN** `Run` returns an error within 10 seconds and nothing listens on `GRPC_PORT`
- **VERIFIED BY**: team-audit `internal/app` `TestRunFailsWhenDatabaseUnreachable`

#### Scenario: The server binary does not link the in-memory store
- **WHEN** the test lists the dependencies of `./cmd/server`
- **THEN** the list contains no package whose path ends in `storetest`, otherwise the test fails
- **VERIFIED BY**: team-audit `internal/app` `TestServerBinaryExcludesInMemoryStore`

#### Scenario: A written event is durable
- **WHEN** a service principal holding `audit.write` writes an event through a running `Run` and the test then reads `audit_events` directly
- **THEN** the table holds exactly one row with that action and actor
- **VERIFIED BY**: team-audit `internal/app` `TestRunWriteIsCommittedToPostgres`

### Requirement: Migrations are embedded and applied only by the migrate up subcommand

The migrations SHALL be embedded in the binary from `internal/store/migrations` with four-digit names `0001_create_audit_events` and `0002_audit_action_created_index`, keeping versions 1 and 2. The command `server migrate up` SHALL apply pending migrations through `migrate.Up` and exit 0; any migration failure, including a dirty `schema_migrations` row or an unreachable database, SHALL exit non-zero in every `ENV`. `app.Run` SHALL NOT apply migrations. A database already at version 2 SHALL be left unchanged.

#### Scenario: An empty database is migrated by migrate up
- **WHEN** `app.Migrate` (the `migrate up` command) runs against an empty Postgres database
- **THEN** it returns nil, the table `audit_events` and the index `idx_audit_events_action_created` exist and `schema_migrations.version` is 2
- **VERIFIED BY**: team-audit `internal/app` `TestMigrateUpMigratesEmptyDatabase`

#### Scenario: A database already at version 2 is unchanged
- **WHEN** `migrate up` runs against a database that holds version 2 and 3 audit rows
- **THEN** it returns nil, the rows are unchanged and the version stays 2
- **VERIFIED BY**: team-audit `internal/app` `TestMigrateUpLeavesMigratedDatabaseUnchanged`

#### Scenario: A dirty migration state fails migrate up
- **WHEN** `migrate up` runs against a database whose `schema_migrations` row is marked dirty
- **THEN** it returns an error and the schema is not changed
- **VERIFIED BY**: team-audit `internal/app` `TestMigrateUpFailsWhenMigrationStateIsDirty`

#### Scenario: Run does not migrate
- **WHEN** `Run` starts against an empty Postgres database
- **THEN** `audit_events` and `schema_migrations` do not exist after boot
- **VERIFIED BY**: team-audit `internal/app` `TestRunDoesNotMigrate`

#### Scenario: The migrate up subcommand is dispatched by main
- **WHEN** the built `cmd/server` binary runs with the arguments `migrate up` against an unreachable database
- **THEN** it exits non-zero within 10 seconds and starts no listener
- **VERIFIED BY**: team-audit `internal/app` `TestMigrateUpSubcommandExitsNonZeroOnUnreachableDatabase`

#### Scenario: Migration 002 creates and drops the action index
- **WHEN** migration 0002 is applied up and then down on a database holding migration 0001
- **THEN** the index exists after up and is absent after down, and existing rows are untouched
- **VERIFIED BY**: team-audit `internal/store` `TestMigration002ActionIndex`

### Requirement: Readiness follows Postgres and liveness does not

The server SHALL register a `postgres` health check. The overall status (`""`) SHALL be NOT_SERVING within one `healthx.DefaultInterval` after the check fails and SERVING again within one interval after it recovers. The `liveness` service SHALL stay SERVING while the check fails. On shutdown `Run` SHALL set both NOT_SERVING before the gRPC server stops, and SHALL stop within `SHUTDOWN_GRACE` plus `SHUTDOWN_DRAIN_DELAY`.

#### Scenario: Readiness fails while liveness stays serving
- **WHEN** the database pool is closed under a running service
- **THEN** the overall health status is NOT_SERVING within one interval and the `liveness` status is SERVING
- **VERIFIED BY**: team-audit `internal/app` `TestReadinessFollowsDatabaseLivenessDoesNot`

#### Scenario: Shutdown drains health first
- **WHEN** the context passed to `Run` is cancelled while a client holds a health watch open
- **THEN** the watch reports NOT_SERVING for both `""` and `liveness` while the stream is still open, and `Run` returns nil
- **VERIFIED BY**: team-audit `internal/app` `TestShutdownDrainsHealthFirst`

#### Scenario: An unknown health service name is not found
- **WHEN** a client checks health for the service name `nope`
- **THEN** the call fails with `NotFound`
- **VERIFIED BY**: team-audit `internal/app` `TestUnknownHealthServiceNotFound`

### Requirement: WriteAuditEvent is service-only and QueryAuditLog is admin-only through the SDK auth package

`WriteAuditEvent` SHALL call `auth.RequireService(ctx, "audit.write")` as its first statement: no or anonymous principal gets `Unauthenticated`; a user principal (an admin, or a user claiming the scope) and a service principal without `audit.write` get `PermissionDenied`; no event is stored for a refused call. `QueryAuditLog` SHALL call `auth.RequireScopes(ctx, "admin")` as its first statement: no or anonymous principal gets `Unauthenticated`, a principal without the exact scope `admin` gets `PermissionDenied`, and the repository is not called. Authorization SHALL precede request validation. The stored `actor_id` SHALL be the request `actor_id` when set, otherwise the calling service id.

#### Scenario: A service with audit.write writes on behalf of a user
- **WHEN** a service principal holding `audit.write` writes an event with `actor_id` `u-1`
- **THEN** the call succeeds and the stored event has actor `u-1`
- **VERIFIED BY**: team-audit `internal/rpc` `TestWriteAuditEventServicePrincipalMayActOnBehalf`

#### Scenario: An anonymous write is refused
- **WHEN** a caller with no principal or the anonymous principal calls `WriteAuditEvent`
- **THEN** the call fails with `Unauthenticated` and nothing is stored
- **VERIFIED BY**: team-audit `internal/rpc` `TestWriteAuditEventRequiresAuthentication`

#### Scenario: A user, an admin or a scopeless service cannot write
- **WHEN** a user, a user holding `admin` and `audit.write`, and a service without `audit.write` each call `WriteAuditEvent`
- **THEN** each call fails with `PermissionDenied` and nothing is stored
- **VERIFIED BY**: team-audit `internal/rpc` `TestWriteAuditEventRejectsNonServiceCallers`

#### Scenario: A non-admin cannot query
- **WHEN** a user principal without the scope `admin` calls `QueryAuditLog` with any filter
- **THEN** the call fails with `PermissionDenied` and the repository is not called
- **VERIFIED BY**: team-audit `internal/rpc` `TestQueryAuditLogAdminOnly`

#### Scenario: An anonymous query is refused
- **WHEN** a caller with no principal calls `QueryAuditLog`
- **THEN** the call fails with `Unauthenticated` and the repository is not called
- **VERIFIED BY**: team-audit `internal/rpc` `TestQueryAuditLogUnauthenticated`

#### Scenario: Authorization precedes validation
- **WHEN** a non-admin user queries with `created_after` later than `created_before`
- **THEN** the call fails with `PermissionDenied` and not `InvalidArgument`
- **VERIFIED BY**: team-audit `internal/rpc` `TestQueryAuditLogAuthBeforeValidation`

#### Scenario: The forwarded principal metadata is honoured over the wire
- **WHEN** over a real gRPC connection a bare call, a service call with `audit.write` and an admin call with `admin` are made
- **THEN** the bare calls fail with `Unauthenticated`, the service write succeeds and the admin query returns that one event
- **VERIFIED BY**: team-audit `internal/rpc` `TestWireAuthorization`

#### Scenario: Reflection is not served by default
- **WHEN** a client calls server reflection on a server built with default settings
- **THEN** the call fails with `Unimplemented`
- **VERIFIED BY**: team-audit `internal/rpc` `TestReflectionOffByDefault`

### Requirement: Errors are mapped once in transport through the grpcx table

`internal/rpc` SHALL convert every error with `grpcx.Status` and one `ErrorTable` holding `audit.ErrEmptyAction` and `audit.ErrInvalidTimeRange` as `InvalidArgument`. An unmapped error SHALL become `Internal` with the message `internal error` and its cause SHALL appear only in the server log. A cancelled context SHALL return `Canceled`, an exceeded deadline `DeadlineExceeded`, and a primary-key violation (SQLSTATE `23505`) `AlreadyExists`.

#### Scenario: An empty action is invalid
- **WHEN** a service principal holding `audit.write` writes an event with an empty action
- **THEN** the call fails with `InvalidArgument` and nothing is stored
- **VERIFIED BY**: team-audit `internal/rpc` `TestWriteAuditEventValidation`

#### Scenario: An inverted time range is invalid
- **WHEN** an admin queries with `created_after` 12:00:00 and `created_before` 10:00:00
- **THEN** the call fails with `InvalidArgument` and the repository is not called
- **VERIFIED BY**: team-audit `internal/rpc` `TestQueryAuditLogInvertedRangeInvalidArgument`

#### Scenario: A storage failure is generic and not leaked
- **WHEN** the repository fails with an error whose text contains `password=secret`
- **THEN** the call fails with `Internal` and message `internal error`, and the response text does not contain `secret`
- **VERIFIED BY**: team-audit `internal/rpc` `TestInternalErrorIsGenericAndNotLeaked`

#### Scenario: A cancelled request returns Canceled
- **WHEN** the repository returns `context.Canceled` for an admin query
- **THEN** the call fails with `Canceled` and not `Internal`
- **VERIFIED BY**: team-audit `internal/rpc` `TestCanceledRequestMapsToCanceled`

#### Scenario: A timed-out request returns DeadlineExceeded
- **WHEN** the repository returns `context.DeadlineExceeded` for a write
- **THEN** the call fails with `DeadlineExceeded` and not `Internal`
- **VERIFIED BY**: team-audit `internal/rpc` `TestDeadlineExceededMapsToDeadlineExceeded`

#### Scenario: A duplicate primary key returns AlreadyExists
- **WHEN** Postgres rejects an insert with SQLSTATE `23505` on `audit_events_pkey`
- **THEN** the call fails with `AlreadyExists` and message `already exists`
- **VERIFIED BY**: team-audit `internal/rpc` `TestUniqueViolationMapsToAlreadyExists`

### Requirement: The query cursor and page size keep their format

`QueryAuditLog` SHALL use `pagination.Clamp(size, 20, 100)` for the page size. The `next_cursor` SHALL be the decimal row offset (`strconv.Itoa`) and SHALL be empty when no row remains; an empty, malformed or negative cursor SHALL mean offset 0. The SDK base64 offset token SHALL NOT be used. Ordering SHALL remain `created_at DESC, id DESC` and `page.total` SHALL count the matches before paging.

#### Scenario: The cursor is a decimal offset
- **WHEN** an admin queries a target holding 25 events with page size 20
- **THEN** `next_cursor` equals `20`, `page.total` equals 25 and the second page holds 5 events with an empty `next_cursor`
- **VERIFIED BY**: team-audit `internal/rpc` `TestQueryAuditLogCursorPaging`

#### Scenario: A malformed or negative cursor starts at the first page
- **WHEN** an admin queries with cursor `zz` and then with cursor `-5`
- **THEN** both calls succeed and return the same first page as an empty cursor
- **VERIFIED BY**: team-audit `internal/rpc` `TestMalformedCursorStartsAtFirstPage`

#### Scenario: Page size is clamped to the default and the maximum
- **WHEN** an admin queries with page size 0, 100 and 101 over 150 events
- **THEN** the pages hold 20, 100 and 100 events
- **VERIFIED BY**: team-audit `internal/rpc` `TestPageSizeClamp`

### Requirement: Query behaviour is identical on Postgres and the test fake

The Postgres repository and `storetest.Memory` SHALL return the same event ids in the same order and the same total for the same filter. The Postgres half SHALL run in `make check` against a `testkit.Postgres` database and SHALL fail, not skip, when Docker is unavailable.

#### Scenario: The same filter table gives the same result on both stores
- **WHEN** the same table of filters runs against both stores over the same seeded events
- **THEN** both return the same event ids in the same order and the same total
- **VERIFIED BY**: team-audit `internal/store` `TestRepositoryFilterParity`

#### Scenario: A hostile filter value is matched literally
- **WHEN** the Postgres store queries with action `x' OR '1'='1` over a table holding other events
- **THEN** it returns an empty page without an error and every stored row is intact
- **VERIFIED BY**: team-audit `internal/store` `TestQueryFilterValueIsBound`

### Requirement: Logs carry SDK identifiers and omit filter values

The service SHALL log through `logx` (JSON, `service=team-audit`, principal and trace ids from the request context). A log line SHALL NOT contain the value of an `actor_id`, `target_id` or `action` filter, or any event metadata value. The server SHALL recover a handler panic as `Internal` without stopping the process.

#### Scenario: A query does not log its filter values
- **WHEN** an admin queries with action `secret-action` and target id `secret-target` and the repository fails
- **THEN** the captured log output contains no `secret-action` and no `secret-target`
- **VERIFIED BY**: team-audit `internal/rpc` `TestQueryFiltersAreNotLogged`

#### Scenario: A refused query logs no filter values
- **WHEN** a user without the scope `admin` queries with action `secret-action`
- **THEN** the call fails with `PermissionDenied` and the captured log output contains no `secret-action`
- **VERIFIED BY**: team-audit `internal/rpc` `TestRefusedQueryLogsNoFilterValues`

#### Scenario: A panicking repository does not kill the process
- **WHEN** the repository panics during an admin query and the next query is made
- **THEN** the first call fails with `Internal` and the second call succeeds
- **VERIFIED BY**: team-audit `internal/rpc` `TestPanicIsRecoveredAsInternal`
