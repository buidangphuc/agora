## ADDED Requirements

### Requirement: team-audit boots on go-sdk with fail-fast storage

team-audit SHALL wire go-sdk v0.1.1 in `internal/app.Run(ctx, settings)` per ADR-0016, and SHALL declare no enable flags for always-on capabilities. It SHALL open the database with `pgx.Open`. Outside `ENV=local`, a database that cannot be reached SHALL fail boot before the port opens. In `ENV=local` it SHALL warn and serve from the in-memory store.

#### Scenario: team-audit fails boot on an unreachable database outside local
- **WHEN** team-audit starts with `ENV=staging` and `DATABASE_URL` pointing at a closed port
- **THEN** `app.Run` returns an error and no listener is opened
- **VERIFIED BY**: `team-audit/internal/app` `TestBootFailsWhenDatabaseUnreachableOutsideLocal`

#### Scenario: team-audit falls back to memory locally
- **WHEN** team-audit starts with `ENV=local` and an unreachable database
- **THEN** its RPCs are served from the in-memory store and health is SERVING
- **VERIFIED BY**: `TestLocalFallsBackToMemoryStore`

#### Scenario: team-audit declares no enable flags
- **WHEN** the declared env keys of team-audit's Settings are listed
- **THEN** none of `DATABASE_ENABLED` and `GRPC_REFLECTION_ENABLED` is declared
- **VERIFIED BY**: `TestNoEnableFlagsDeclared`

### Requirement: team-audit health reflects the database

team-audit SHALL report NOT_SERVING while its Postgres check fails.

#### Scenario: team-audit health fails to NOT_SERVING when the database is lost
- **WHEN** the database used by a running team-audit stops accepting connections
- **THEN** within one probe interval health is NOT_SERVING
- **VERIFIED BY**: `TestHealthReportsDatabaseDown`

### Requirement: team-audit behaviour is unchanged

All existing team-audit tests SHALL move with their packages and pass, with unchanged assertions except auth status message text (codes unchanged).

#### Scenario: team-audit existing suites do not regress and no assertion is weakened
- **WHEN** `make check` runs after the restructure
- **THEN** every pre-existing test passes, and no assertion is removed or loosened
- **VERIFIED BY**: `team-audit` `make check`
