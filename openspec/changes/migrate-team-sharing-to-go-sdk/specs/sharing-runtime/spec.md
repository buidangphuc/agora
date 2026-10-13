## ADDED Requirements

### Requirement: team-sharing boots on go-sdk with fail-fast storage

team-sharing SHALL wire go-sdk v0.1.1 in `internal/app.Run(ctx, settings)` per ADR-0016. It SHALL always open the database with `pgx.Open`, which pings. Migrations SHALL stay in `migrations/` at the repo root, applied by the deploy migrate job, not at boot. Outside `ENV=local`, a database that cannot be reached SHALL fail boot with a non-nil error before the port opens. In `ENV=local` it SHALL log a warning and serve from the in-memory store.

#### Scenario: Unreachable database fails boot outside local
- **WHEN** the service starts with `ENV=staging` and `DATABASE_URL` pointing at a closed port
- **THEN** `app.Run` returns an error and no gRPC listener is opened
- **VERIFIED BY**: `team-sharing/internal/app` `TestBootFailsWhenDatabaseUnreachableOutsideLocal`

#### Scenario: Local falls back to the in-memory store
- **WHEN** the service starts with `ENV=local` and an unreachable database
- **THEN** it serves `CreateShareLink` and `ResolveShareLink` from memory
- **VERIFIED BY**: `TestLocalFallsBackToMemoryStore`

### Requirement: Health reflects the database

When the database is in use, team-sharing SHALL register a `postgres` health check and report NOT_SERVING while the check fails.

#### Scenario: Database loss flips health
- **WHEN** the database used by a running service is stopped
- **THEN** within one probe interval the gRPC health status is NOT_SERVING
- **VERIFIED BY**: `TestHealthReportsDatabaseDown` (testkit Postgres)

#### Scenario: Without a database no postgres check is registered
- **WHEN** the service runs from the in-memory store (`ENV=local`, database unreachable)
- **THEN** health is SERVING and does not report NOT_SERVING for a database it does not use
- **VERIFIED BY**: `TestLocalMemoryStoreHealthIsServing`

### Requirement: Share-link behaviour is unchanged

The migration SHALL NOT change the RPC behaviour. The current handler and domain tests SHALL be kept, moved to the new packages and pass.

The behaviour that must not change:
- Anonymous callers may create links, and `created_by` is empty for them.
- An authenticated creator is stamped as `created_by`.
- Resolving increments `click_count`.
- An empty `target_type`, `target_id` or `short_code` is `InvalidArgument`.
- An unknown code is `NotFound`.
- Internal causes are hidden.

#### Scenario: Existing behaviour tests stay green after the move
- **WHEN** the suite runs after the restructure
- **THEN** these tests pass unchanged in assertions: `TestHandlerCreateThenResolve`, `TestHandlerResolveUnknownIsNotFound`, `TestHandlerCreateInvalidArgument`, `TestHandlerCreateAttributesPrincipal`, `TestMapErrHidesInternalCause`, `TestCreateResolveRoundtrip`, `TestResolveUnknownReturnsNotFound`, `TestResolveIncrementsClickCount`, `TestCreateAndResolveValidation`, `TestCreateWithNilUTM` and `TestCreateShareLinkRecordsCreator`
- **VERIFIED BY**: `team-sharing` `make check`

#### Scenario: Anonymous creator is not rejected
- **WHEN** `CreateShareLink` is called with `x-principal-id=anonymous`
- **THEN** it succeeds and the stored `created_by` is empty
- **VERIFIED BY**: `TestCreateAnonymousStoresEmptyCreator`

### Requirement: Migrations live at the repo root

ADR-0016 and `examples/hello` SHALL place a service's migrations in `<repo>/migrations/` (golang-migrate `NNNN_name.up/down.sql`), applied by a migrate job. A root package `migrations` SHALL embed them (`//go:embed *.sql`) for `migrate.Up` in tests and in an optional `migrate up` subcommand. A service SHALL NOT apply migrations at boot unless its ADR says so.

#### Scenario: Example migrations are embedded from the root
- **WHEN** the hello example tests run on testkit Postgres
- **THEN** `migrate.Up` applies `examples/hello/migrations/*.up.sql` through the root `migrations` package
- **VERIFIED BY**: `platform-core/packages/go-sdk/examples/hello/internal/store` `TestMigrationsApply`

#### Scenario: Boot does not apply migrations
- **WHEN** team-sharing boots against an empty database
- **THEN** no `share_links` table is created and `schema_migrations` does not exist
- **VERIFIED BY**: `team-sharing/internal/app` `TestBootDoesNotApplyMigrations`
