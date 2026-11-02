## Context

team-audit is a gRPC service (`:50066`) with two RPCs and one table. It consumes and publishes nothing and calls no upstream. Main today: 14 commits since the first migration, all behaviour of Q3 (`audit-query-filters`, service-only writes) is on main and wins over `origin/feat/go-sdk-v1-e2e` (reference only; that branch still has `OpenOrLocal`, an in-memory fallback, no filters and no migrations 002).

## D1 Package map (ADR-0016)

| Today | After | Notes |
|---|---|---|
| `cmd/server/main.go` | `cmd/server/main.go` | signal context, `config.Load`, `app.Run`, non-zero exit on error; binary stays `server` (Dockerfile path unchanged) |
| (inline in main) | `internal/app/app.go` | `const service = "team-audit"`; composition root |
| `internal/config` | `internal/config` | `Settings`, `Load`, `.env.example` drift test |
| `internal/handler`, `internal/interceptor`, `internal/grpcserver` | `internal/rpc` | handlers, `ErrorTable`, `Register`; no interceptor file: `grpcx.NewServer` installs `auth.UnaryServerInterceptor` |
| `internal/service` | `internal/audit` | `Service`, `Event`, `Filter`, sentinels `ErrEmptyAction`, `ErrInvalidTimeRange`; imports no grpc and no generated code |
| `internal/repository` | `internal/store` | `Postgres` repository, embedded `migrations/`; in-memory repository becomes `internal/store/storetest` |
| `migrations/` | `internal/store/migrations/` | renamed to four digits, embedded with `//go:embed` |

The domain type `audit.Event` (id, actor, action, target type, target id, metadata, created at) is mapped to `auditv1.AuditEvent` in `rpc`. Rejected: keeping the proto message in the domain (violates "domain never imports generated transport code", and the old `service` package did exactly that).

## D2 Test relocation keeps test names

Existing specs cite Go tests by package and name. The tests move with their code and keep their names, so every `VERIFIED BY` in `audit-log-query`, `audit-access-control` and `audit-query-contract` still resolves by function name:

| Old cite | New package |
|---|---|
| `internal/handler` (`TestQueryAuditLog*`, `TestWriteAuditEvent*`, `TestInternalErrorIsGenericAndNotLeaked`) | `internal/rpc` |
| `internal/service` (`TestQuery*`, `TestWrite*`) | `internal/audit` |
| `internal/repository` (`TestRepositoryFilter*`, `TestMigration002ActionIndex`) | `internal/store` |
| `internal/interceptor` (`TestPrincipalResolution`, `TestRequireService`) | deleted; replaced by `TestAuthorizationBoundary` (SDK `auth` is tested in the SDK) |
| `internal/grpcserver` (`TestWireAuthorization`) | `internal/rpc` |
| `internal/config` (`TestLoadDefaults`, `TestLoadEnvOverride`) | rewritten (defaults no longer exist): `TestLoadRequiresGRPCPort`, `TestLoadReadsEnvironment` |

Updating the package text in `openspec/specs/*` is done when this change is archived, not by this change.

## D3 Migrations run from the binary as `migrate up`, never at boot

The binary has two modes: `server` serves; `server migrate up` opens Postgres (`postgres.Open`), runs `migrate.Up(ctx, url, store.Migrations())` through `app.Migrate(ctx, settings)` and exits (0 on success or nothing pending, non-zero on any error including a dirty state). `app.Run` never migrates. `main` dispatches on `os.Args`; the subcommand loads the same `config.Settings` as the server (the job carries the same environment). Compose keeps `team-audit-migrate` running the service image with `migrate up`; gitops runs the same command as an Argo PreSync Job (both in `deploy-go-sdk-v1`). golang-migrate holds an advisory lock, so concurrent jobs migrate once.

File names: golang-migrate parses the leading digits as the version, so `0001_create_audit_events` is version 1, as `001_create_audit_events` was. A database whose `schema_migrations` row says version 2 (written by the old compose job) therefore needs nothing. Rejected: migrating inside `Run` (a rolling update would run DDL from serving pods) and three-digit names (the ADR convention is four digits).

## D4 Storage errors and the id collision

Event ids are `audit_` plus 8 hex characters of a UUID (about 4.3 billion values). A collision on the primary key now surfaces as `AlreadyExists` through the `23505` row of the `grpcx` table instead of `Internal`. The id format is not changed here (out of scope); the probability is negligible at audit volumes and the producers fail closed on any error.

## D5 Authorization through the SDK `auth` package

`WriteAuditEvent` calls `auth.RequireService(ctx, "audit.write")`; `QueryAuditLog` calls `auth.RequireScopes(ctx, "admin")`; each is the first statement of its handler. SDK `RequireService` checks the principal type before the scope, which is the order the old `RequireService` used, so the codes are identical: `Unauthenticated` (no or anonymous principal), `PermissionDenied` (user principal, or service without the scope). The SDK has no scope that implies another, so bare `admin` does not open `WriteAuditEvent` (same as today). Only the message text differs (accepted change, see Clarifications).

## D6 Cursor stays a decimal offset

`pagination.EncodeOffset` is base64 and `DecodeOffset` rejects malformed tokens. team-audit's cursor is a plain decimal string and a malformed cursor means the first page (gateway and the audit viewer UI send the value back unchanged). The cursor code stays in `rpc` (`strconv`); only `pagination.Clamp(size, 20, 100)` is taken from the SDK. Rejected: switching to the SDK token (changes the wire format of `next_cursor`).

## D7 Mock data: none

The service has one adapter, its own Postgres, and "mock mode" meant skipping it. That is the runtime in-memory fallback the user removed, not mock data served by the real code path. There is no external provider to replace with a fixture, so no `MOCK_DATA` group and no `pkg/mockdata` import. e2e keeps using the real service with its real database.

## D8 SDK packages not used

`auditx` (this service is the store; a gate that writes to itself before writing makes no sense), `upstream`, `kafkax`, `consume`, `outbox`, `idempotency` (no events; writes are append-only and a retry by a producer is a new event by design), `flags`, `jwtx` (the gateway verifies JWTs), `resilience` (no outbound calls), `mockdata` (D7). `UPSTREAM_AUDIT_ADDR` is therefore not a setting of this service.

## D9 In-memory repository as a test fake

`internal/store/storetest.Memory` implements the same interface and serves `audit` and `rpc` tests and the parity test against Postgres. A test (`TestServerBinaryExcludesInMemoryStore`) runs `go list -deps ./cmd/server` and fails if `storetest` is linked, so the fake cannot return as a runtime fallback.

## D10 Tests that need Postgres

They use `testkit.Postgres(t)` and run in `make check` with `GO_SDK_REQUIRE_DOCKER=1`; they fail, not skip, without Docker. `TEST_DATABASE_URL` and the hand-rolled schema helper (`pgtest_audit_test.go`) are removed. The unreachable-database boot test needs no Docker (closed local port). A skipped Docker-backed test is a gate failure, never a pass (human, Round 2).

## Clarifications

### Round 1 — 2026-10-23
- Target `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`: require the tag, no `replace`, no `third_party`; reference `../platform-core/make/go-service.mk` and the SDK `.golangci.yml` without copies; add no doc comments (revive doc-comment rules are off); pin golangci-lint as a go tool like the SDK `go.mod`. (human, recipe)
- ADR-0016 layout: `cmd/<bin>/main.go` -> `internal/app.Run(ctx, settings)`; `internal/config`; `internal/rpc`; domain packages; `internal/store`; no `internal/events` (no Kafka). (human, recipe)
- Delete every dependency enable flag (`DATABASE_ENABLED` here) and every runtime in-memory or noop fallback (`NewInMemoryAuditRepo` in `main`); in-memory implementations stay only as test fakes. Kept flags: `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused in a strict ENV). (human, recipe)
- Mock is mock data served by the real service, never a mock service; switches move onto `pkg/mockdata` (`MOCK_DATA=<adapters>`, refused in a strict ENV before validation). team-audit has no such switch to move and no adapter to mock (D7). (human, recipe)
- Use the SDK for everything it covers: config, logx, otelx, healthx (readiness = overall, liveness independent of dependencies, `probes.livenessService: liveness`), lifecycle, grpcx (error table incl. `23505` -> `AlreadyExists`, `Canceled`/`DeadlineExceeded`), auth, postgres.Open (no `OpenOrLocal`), migrate, pagination (keep the existing cursor format). These behaviour changes are accepted. (human, recipe)
- Keep every current behavioural requirement and every Q1-Q11/C4/privacy-hardening behaviour green; main's behaviour wins over the old migration on `origin/feat/go-sdk-v1-e2e`. MODIFY only requirements that describe removed flags, fallbacks or env vars; ADD new capability requirements where no spec describes the service. No existing spec of `audit-*` names `DATABASE_ENABLED`, the in-memory fallback or the migrate job, so this change only ADDS `audit-service-runtime`. (human, recipe)
- No DPO sign-off needed: basis legitimate interest, platform log retention. (human, recipe)
- e2e is not run per service now (once at the end); scenarios may still name e2e features. `docker-compose.services.yaml` and `platform-gitops` edits are not code tasks of this change; they are listed under "Deploy track" in tasks.md. (human, recipe)

### Round 2 — 2026-10-23
- Migrations: the binary has a `migrate up` subcommand (SDK `pkg/migrate`, embedded migrations); the service never migrates at boot; compose keeps a `team-audit-migrate` service running the service image with `migrate up`; gitops gets an Argo PreSync migrate Job (in `deploy-go-sdk-v1`). (human)
- Kafka is not a readiness check in any service; readiness = Postgres here (team-audit has no Kafka). (human)
- The shared deploy-runtime requirement "Services refuse unsafe configuration in staging and production" and any other shared deploy-runtime requirement are MODIFIED only by `deploy-go-sdk-v1`; this change has no MODIFIED copy and lists the service bullet under the Deploy track. (human)
- Local compose sets `GRPC_REFLECTION_ENABLED=true` for team-audit (Deploy track line). (human)
- Docker-backed tests must run: a skip is a gate failure (`GO_SDK_REQUIRE_DOCKER=1`). (human)
- Raw gitops manifests, `LOG_KEY_FP_SECRET`, upstream-address and outbox decisions do not apply to team-audit (no raw manifest, no log key, no upstream, no outbox). (human)
- Accepted: the SDK 2 s outbound bound and error-text changes already listed; no new accepted behaviour change for team-audit. (human)
