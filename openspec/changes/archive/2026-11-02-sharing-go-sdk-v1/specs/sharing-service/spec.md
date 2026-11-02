## ADDED Requirements

### Requirement: Share links are created and resolved with unchanged behaviour

`platform.sharing.v1.SharingService` SHALL keep serving `CreateShareLink` and `ResolveShareLink` on the configured gRPC port (50065 in compose) after the move to the SDK. `CreateShareLink(target_type, target_id, utm)` SHALL return `InvalidArgument` when `target_type` or `target_id` is empty, SHALL mint a 7-character base62 `short_code` with `crypto/rand`, SHALL retry up to 5 times on a code collision, and SHALL accept anonymous callers. `ResolveShareLink(short_code)` SHALL return `InvalidArgument` on an empty code and `NotFound` on an unknown code, and SHALL otherwise atomically increment `click_count` and return the target, `utm`, `og_meta` and the post-increment `click_count`. An error that is not a domain error SHALL reach the caller as `Internal` with message `internal error` and no cause. These behaviours SHALL be implemented in `internal/sharelink` (no grpc import) behind `internal/rpc`, over `internal/store`.

#### Scenario: A created share link resolves back to its target
- **WHEN** `CreateShareLink` is called with target `listing` / `listing_001` and then `ResolveShareLink` is called with the returned code
- **THEN** the resolve response carries target `listing` / `listing_001` and the OG meta defaults, and `click_count` is 1
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/services/new_services.feature` `A created share link resolves back to its target with OG meta`

#### Scenario: Handler round trip returns the stored link
- **WHEN** the `rpc` handler creates a link for `listing` / `abc` and resolves the returned code
- **THEN** the resolved target equals the created target and the code has 7 base62 characters
- **VERIFIED BY**: `team-sharing/internal/rpc` `TestHandlerCreateThenResolve`

#### Scenario: Unknown short code is NotFound
- **WHEN** `ResolveShareLink` is called with a well-formed code that was never created
- **THEN** the call fails with code `NotFound`
- **VERIFIED BY**: `team-sharing/internal/rpc` `TestHandlerResolveUnknownIsNotFound`

#### Scenario: Empty target or empty code is InvalidArgument
- **WHEN** `CreateShareLink` is called with an empty `target_type`, then with an empty `target_id`, and `ResolveShareLink` is called with an empty code
- **THEN** each of the three calls fails with code `InvalidArgument`
- **VERIFIED BY**: `team-sharing/internal/rpc` `TestHandlerCreateInvalidArgument`

#### Scenario: Each resolve increments click_count by exactly one
- **WHEN** a created link is resolved 3 times
- **THEN** the three responses carry `click_count` 1, 2 and 3
- **VERIFIED BY**: `team-sharing/internal/sharelink` `TestResolveIncrementsClickCount`

#### Scenario: A nil UTM map is stored as an empty object
- **WHEN** `CreateShareLink` is called with no UTM and the link is resolved
- **THEN** the resolved `utm` is empty and the call succeeds
- **VERIFIED BY**: `team-sharing/internal/sharelink` `TestCreateWithNilUTM`

#### Scenario: Mint retries on collision and then gives up
- **WHEN** the store reports a code collision on each of 5 consecutive attempts
- **THEN** `CreateShareLink` fails after exactly 5 store attempts with a non-domain error that reaches the caller as `Internal` `internal error`
- **VERIFIED BY**: `team-sharing/internal/sharelink` `TestCreateGivesUpAfterFiveCollisions`

#### Scenario: A non-domain error hides its cause
- **WHEN** the store returns `errors.New("dial tcp 10.0.0.9: refused")` during a resolve
- **THEN** the caller receives code `Internal`, message `internal error`, and the message does not contain `10.0.0.9`
- **VERIFIED BY**: `team-sharing/internal/rpc` `TestMapErrHidesInternalCause`

### Requirement: The creator is the forwarded principal and anonymous callers are accepted

`CreateShareLink` SHALL store the principal id forwarded in `x-principal-id` as `created_by` when the principal is authenticated, and `''` when the call carries no principal or the gateway's anonymous principal (`x-principal-id=anonymous` or `x-principal-type=anonymous`). Identity SHALL be read only from `x-principal-{id,type,scopes}` metadata, through the SDK `auth` interceptor; the legacy `x-user-id` header SHALL NOT identify anyone. `CreateShareLink` and `ResolveShareLink` SHALL require no scope.

#### Scenario: An authenticated creator is recorded
- **WHEN** `CreateShareLink` is called with `x-principal-id=user-1` and `x-principal-type=user`
- **THEN** the stored link has `created_by = user-1`
- **VERIFIED BY**: `team-sharing/internal/rpc` `TestHandlerCreateAttributesPrincipal`

#### Scenario: Anonymous creator is accepted and stored as empty
- **WHEN** `CreateShareLink` is called with `x-principal-id=anonymous`, and again with no metadata at all
- **THEN** both calls succeed and the stored `created_by` is `''` for each
- **VERIFIED BY**: `team-sharing/internal/rpc` `TestCreateAnonymousStoresEmptyCreator`

#### Scenario: The legacy user id header identifies nobody
- **WHEN** `CreateShareLink` is called with only `x-user-id=user-9` in metadata
- **THEN** the call succeeds and the stored `created_by` is `''`
- **VERIFIED BY**: `team-sharing/internal/rpc` `TestLegacyUserIDHeaderIgnored`

#### Scenario: Resolve needs no principal and no scope
- **WHEN** `ResolveShareLink` is called with no metadata for an existing code
- **THEN** the call succeeds and returns the target
- **VERIFIED BY**: `team-sharing/internal/rpc` `TestResolveRequiresNoPrincipal`

### Requirement: The service always uses Postgres and has no in-memory mode

team-sharing SHALL store every share link in Postgres. `config.Load` SHALL return an error naming `DATABASE_URL` when it is unset or empty, in every `ENV` including `local` and `test`, and the service SHALL NOT supply a default DSN. `app.Run` SHALL return an error, without opening the gRPC listener, when the database does not answer a ping within 5 seconds (`postgres.Open`), in every `ENV` including `local`. Non-test code SHALL NOT contain an in-memory share-link repository, a call to `postgres.OpenOrLocal`, or any environment variable that switches the database on or off; `DATABASE_ENABLED` SHALL NOT be read. An in-memory repository SHALL exist only in `_test.go` files as a test fake.

#### Scenario: Missing DATABASE_URL is refused in every ENV
- **WHEN** `config.Load` runs with `GRPC_PORT=50099`, an empty `DATABASE_URL`, and `ENV` set in turn to `local`, `test`, `staging` and `prod`
- **THEN** each call returns an error that names `DATABASE_URL`
- **VERIFIED BY**: `team-sharing/internal/config` `TestLoadRequiresDatabaseURLInEveryEnv`

#### Scenario: No default DSN is compiled in
- **WHEN** `config.Load` runs with `ENV=local`, `GRPC_PORT=50099` and `DATABASE_URL` unset
- **THEN** it returns an error, and no `Settings` field holds a `postgres://` value
- **VERIFIED BY**: `team-sharing/internal/config` `TestLoadHasNoDefaultDSN`

#### Scenario: Unreachable database fails boot locally
- **WHEN** `app.Run` starts with `ENV=local` and `DATABASE_URL` pointing at a closed port
- **THEN** it returns an error within 6 seconds and no TCP listener is open on `GRPC_PORT`
- **VERIFIED BY**: `team-sharing/internal/app` `TestBootFailsWhenDatabaseUnreachableInLocal`

#### Scenario: Unreachable database fails boot outside local
- **WHEN** `app.Run` starts with `ENV=staging` and `DATABASE_URL` pointing at a closed port
- **THEN** it returns an error and no TCP listener is open on `GRPC_PORT`
- **VERIFIED BY**: `team-sharing/internal/app` `TestBootFailsWhenDatabaseUnreachableOutsideLocal`

#### Scenario: The removed enable flag is ignored
- **WHEN** `config.Load` runs with `DATABASE_ENABLED=false`, `ENV=local`, `GRPC_PORT=50099` and a valid `DATABASE_URL`, and again with `DATABASE_ENABLED=false` and no `DATABASE_URL`
- **THEN** the first call succeeds with the URL from `DATABASE_URL`, and the second returns an error naming `DATABASE_URL`
- **VERIFIED BY**: `team-sharing/internal/config` `TestDatabaseEnabledFlagIsIgnored`

#### Scenario: Production source has no in-memory store or local fallback
- **WHEN** every non-test `.go` file under `team-sharing` is scanned for `InMemory`, `OpenOrLocal` and the string `DATABASE_ENABLED`
- **THEN** no file matches
- **VERIFIED BY**: `team-sharing/internal/app` `TestNoInMemoryStoreOrFallbackInProductionSource`

#### Scenario: A running service persists links in Postgres
- **WHEN** a link is created through the service wired to a throwaway Postgres database with `migrations/` applied, and a second service instance on the same database resolves the code
- **THEN** the second instance returns the same target and `click_count` 1
- **VERIFIED BY**: `team-sharing/internal/store` `TestPostgresCreateResolveAcrossInstances`

### Requirement: Settings are the SDK groups and `.env.example` matches them

`internal/config.Settings` SHALL embed `config.Runtime` and `config.GRPC` and hold `Postgres config.Postgres` and `OTel config.OTel`, and `Load` SHALL be `config.Load[Settings]()`. It SHALL read exactly these variables: `ENV` (required, no default), `LOG_LEVEL` (`info`), `LOG_JSON` (`true`), `GRPC_HOST` (`0.0.0.0`), `GRPC_PORT` (required), `SHUTDOWN_GRACE` (`10s`), `SHUTDOWN_DRAIN_DELAY` (`0s`), `GRPC_REFLECTION_ENABLED` (`false`), `DATABASE_URL` (required), `DB_MAX_CONNS` (`10`), `OTEL_ENABLED` (`false`), `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_SERVICE_NAME`. `team-sharing/.env.example` SHALL declare exactly those variables and no others, and SHALL NOT contain `DATABASE_ENABLED` or `SHUTDOWN_GRACE_SECONDS`. gRPC reflection SHALL be off unless `GRPC_REFLECTION_ENABLED=true`, and `config.Load` SHALL refuse `true` when `Runtime.IsStrict()`.

#### Scenario: Valid settings load with the documented defaults
- **WHEN** `config.Load` runs with `ENV=local`, `GRPC_PORT=50099` and a `DATABASE_URL`
- **THEN** `Port` is 50099, `Postgres.URL` equals the given URL, `MaxConns` is 10, `ShutdownGrace` is 10s and `LogLevel` is `info`
- **VERIFIED BY**: `team-sharing/internal/config` `TestConfigLoad`

#### Scenario: Missing ENV or GRPC_PORT is refused
- **WHEN** `config.Load` runs with a valid `DATABASE_URL` and an empty `ENV`, then with `ENV=local` and an empty `GRPC_PORT`
- **THEN** the first error names `ENV` and the second names `GRPC_PORT`
- **VERIFIED BY**: `team-sharing/internal/config` `TestConfigLoadRequiresEnvAndPort`

#### Scenario: A malformed duration is refused
- **WHEN** `config.Load` runs with `SHUTDOWN_GRACE=ten`
- **THEN** it returns an error that names `SHUTDOWN_GRACE`
- **VERIFIED BY**: `team-sharing/internal/config` `TestConfigLoadRejectsBadDuration`

#### Scenario: `.env.example` is in sync with Settings
- **WHEN** `.env.example` and `config.Settings` are compared by `CheckEnvExample`
- **THEN** the two sets of variable names are equal and `DATABASE_ENABLED` and `SHUTDOWN_GRACE_SECONDS` are absent
- **VERIFIED BY**: `team-sharing/internal/config` `TestEnvExampleInSync`

### Requirement: Health reports the database and boot does not migrate

`app.Run` SHALL register a `postgres` check with `healthx` so that the readiness health service (empty service name) report `SERVING` while the database answers and `NOT_SERVING` within one probe interval after it stops answering, and SERVING again after it returns; the `liveness` health service SHALL stay `SERVING` whatever the database state. `app.Run` SHALL NOT apply migrations: it SHALL NOT create `share_links` or `schema_migrations` in an empty database. On shutdown (SIGINT or SIGTERM, modelled by cancelling the context) the service SHALL report NOT_SERVING, stop the gRPC server within `SHUTDOWN_GRACE`, and close the pool.

#### Scenario: Database loss flips health to NOT_SERVING
- **WHEN** the database of a running service is stopped
- **THEN** within `healthx.DefaultInterval` plus 1 second the `grpc.health.v1` status of the empty service name is `NOT_SERVING` and the status of service `liveness` is `SERVING`
- **VERIFIED BY**: `team-sharing/internal/app` `TestHealthReportsDatabaseDown`

#### Scenario: Health recovers when the database returns
- **WHEN** the database of a service reporting NOT_SERVING answers pings again
- **THEN** within `healthx.DefaultInterval` plus 1 second the status is `SERVING`
- **VERIFIED BY**: `team-sharing/internal/app` `TestHealthRecoversWhenDatabaseReturns`

#### Scenario: Boot does not apply migrations
- **WHEN** `app.Run` starts against an empty database with reachable credentials
- **THEN** the table `share_links` does not exist and `schema_migrations` does not exist after boot
- **VERIFIED BY**: `team-sharing/internal/app` `TestBootDoesNotApplyMigrations`

#### Scenario: Shutdown stops the server within the grace period
- **WHEN** the context passed to `app.Run` is cancelled while no call is in flight
- **THEN** `app.Run` returns nil within `SHUTDOWN_GRACE` and the listener port refuses new connections
- **VERIFIED BY**: `team-sharing/internal/app` `TestRunStopsOnContextCancel`

#### Scenario: Migrations apply cleanly to an empty database
- **WHEN** `migrate.Up` applies the embedded `migrations/` to an empty throwaway database, and then a link with a creator is stored
- **THEN** the `share_links` table has the columns `short_code`, `target_type`, `target_id`, `utm`, `og_title`, `og_description`, `og_image_url`, `click_count`, `created_at`, `created_by`
- **VERIFIED BY**: `team-sharing/internal/store` `TestMigrationsApply`

### Requirement: The service consumes the tagged SDK and the shared build files

`team-sharing/go.mod` SHALL contain a `require` for `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at `v1.0.2`, SHALL NOT contain a `replace` for it, and the repository SHALL NOT contain `team-sharing/third_party`, `team-sharing/make` or `team-sharing/.golangci.yml`. `team-sharing/Makefile` SHALL include `../platform-core/make/go-service.mk` and pass `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`. `cmd/server/main.go` SHALL only create the signal context, call `config.Load` and `app.Run`, and exit non-zero on error.

#### Scenario: go.mod requires the tag without replace
- **WHEN** `team-sharing/go.mod` is parsed
- **THEN** it requires the go-sdk module at `v1.0.2` and has no `replace` directive
- **VERIFIED BY**: `team-sharing/internal/app` `TestGoModRequiresTaggedSDKWithoutReplace`

#### Scenario: No vendored or copied build files
- **WHEN** the paths `team-sharing/third_party`, `team-sharing/make` and `team-sharing/.golangci.yml` are looked up
- **THEN** none exists, and `team-sharing/Makefile` contains the include of `../platform-core/make/go-service.mk`
- **VERIFIED BY**: `team-sharing/internal/app` `TestNoVendoredSDKOrCopiedBuildFiles`

#### Scenario: The image builds from the module cache
- **WHEN** `docker build -t team-sharing:local team-sharing` runs with the repository `Dockerfile`
- **THEN** the build exits 0
- **VERIFIED BY**: `docker build -t team-sharing:local team-sharing` (tasks.md 1.8, run by `make -C team-sharing docker`)

### Requirement: Calls are logged and panics are contained

Every gRPC call to team-sharing SHALL pass through the `grpcx` chain: request id, panic recovery, call logging with request id and principal id, and the `auth` interceptor. A panic in a handler SHALL reach the caller as `Internal` with message `internal error` and SHALL NOT stop the process. A call log record SHALL NOT contain the `short_code`, `target_id`, `utm` values or `og_*` text.

#### Scenario: A panicking handler returns Internal and the server keeps serving
- **WHEN** the store panics during one `ResolveShareLink` and a second `ResolveShareLink` for a good code follows
- **THEN** the first call fails with `Internal` `internal error` and the second succeeds
- **VERIFIED BY**: `team-sharing/internal/app` `TestPanicIsContained`

#### Scenario: Call logs carry no link data
- **WHEN** `CreateShareLink` is called for `listing` / `listing-8f3a9c` with an `og_title` of `Secret title`, and the log output of the call is read
- **THEN** one record names the method and the principal id, and no record contains `listing-8f3a9c`, `Secret title` or the returned short code
- **VERIFIED BY**: `team-sharing/internal/app` `TestCallLogsCarryNoLinkData`

#### Scenario: An anonymous call is logged without a principal id
- **WHEN** `ResolveShareLink` is called with `x-principal-id=anonymous` and the log output of the call is read
- **THEN** one record names the method and its code, and the call is not rejected as unauthenticated
- **VERIFIED BY**: `team-sharing/internal/app` `TestAnonymousCallIsLoggedAndServed`

### Requirement: Reflection is off by default and refused in strict ENV

gRPC reflection SHALL be registered only when `GRPC_REFLECTION_ENABLED=true`, and `config.Load` SHALL refuse that value when `Runtime.IsStrict()` with an error naming `GRPC_REFLECTION_ENABLED`.

#### Scenario: Reflection is off by default
- **WHEN** a running service is asked to list services through the reflection API with `GRPC_REFLECTION_ENABLED` unset
- **THEN** the call fails with code `Unimplemented`
- **VERIFIED BY**: `team-sharing/internal/app` `TestReflectionOffByDefault`

#### Scenario: Reflection works when enabled locally
- **WHEN** the service runs with `ENV=local` and `GRPC_REFLECTION_ENABLED=true` and the reflection API lists services
- **THEN** the list contains `platform.sharing.v1.SharingService`
- **VERIFIED BY**: `team-sharing/internal/app` `TestReflectionEnabledLocally`

#### Scenario: Strict ENV refuses reflection
- **WHEN** `config.Load` runs with `ENV=prod`, `GRPC_REFLECTION_ENABLED=true`, `GRPC_PORT=50099` and a `DATABASE_URL`
- **THEN** it returns an error that names `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: `team-sharing/internal/config` `TestLoadRefusesReflectionInStrictEnv`

### Requirement: Boot needs no telemetry collector

With `OTEL_ENABLED=false` the service SHALL boot and serve without any OTLP collector reachable.

#### Scenario: Boot with telemetry off and no collector
- **WHEN** `app.Run` starts with `OTEL_ENABLED=false`, `OTEL_EXPORTER_OTLP_ENDPOINT` pointing at a closed port and a reachable database
- **THEN** it serves a successful `CreateShareLink` call and returns no error from boot
- **VERIFIED BY**: `team-sharing/internal/app` `TestBootWithoutCollectorWhenOTelOff`
