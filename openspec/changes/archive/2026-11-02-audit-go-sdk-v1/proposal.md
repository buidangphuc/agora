## Why

team-audit hand-rolls the platform layer that go-sdk v1.0.2 now provides, and it carries a runtime fallback that is unsafe for an audit trail. `cmd/server/main.go` calls `pgxpool.New`, which connects lazily and never fails, so the "cannot connect, running in in-memory mode" branch is dead code at best. At worst a service that lost its database would acknowledge `WriteAuditEvent` and keep the event only in process memory, so a restart would erase the trail that the fail-closed producers (domain, engagement, identity, order, payment, promotion, verification) rely on. Health is always SERVING, there is no panic recovery, no tracing, no migration run in the service (a compose one-shot owns it), and config reads three variables by hand with a hard-coded database default.

Decisions of 2026-10-22/23 (user, recorded in `plans/go-sdk-migration/PLAN.md`): services depend on the tagged module `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`; importing a dependency means using it (no enable flags, no runtime in-memory or noop fallbacks); mock means mock data served by the real service. team-audit has no external adapter to mock, so it takes no `MOCK_DATA` group.

initiative: go-sdk-migration

## What Changes

- **Dependency**: `go.mod` requires `github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.2`; no `replace`, no `third_party`. golangci-lint is pinned as a `tool` like the SDK `go.mod`. `Makefile` includes `../platform-core/make/go-service.mk` and lints with `../platform-core/packages/go-sdk/.golangci.yml` (no copies).
- **Layout (ADR-0016)**: `cmd/server/main.go` -> `internal/app.Run(ctx, settings)`; `internal/config`; `internal/rpc` (replaces `handler`, `interceptor`, `grpcserver`); `internal/audit` (replaces `service`); `internal/store` (replaces `repository`, owns the embedded `migrations`).
- **Settings**: `config.Settings` = SDK `Runtime`, `GRPC`, `Postgres`, `OTel`. `ENV`, `GRPC_PORT` and `DATABASE_URL` are required (no defaults). **BREAKING (accepted)**: `DATABASE_ENABLED`, the in-memory fallback and the hard-coded `DATABASE_URL`/`GRPC_PORT` defaults are removed; `SHUTDOWN_GRACE_SECONDS` (never read) becomes the SDK `SHUTDOWN_GRACE` duration. Kept flags: `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused in a strict ENV).
- **Database**: `postgres.Open` (pings, fails fast); an unreachable database fails boot in every ENV. The in-memory repository survives only as a test fake (`internal/store/storetest`) that the server binary does not link.
- **Migrations**: embedded in the binary and applied only by a `server migrate up` subcommand (SDK `migrate.Up`); the service never migrates at boot. Files are renamed `001_...` -> `0001_...` and `002_...` -> `0002_...` (same versions 1 and 2, so a database already migrated by the old compose job is unchanged). Compose keeps a `team-audit-migrate` service that runs the service image with `migrate up`; gitops gets an Argo PreSync migrate Job (Deploy track, change `deploy-go-sdk-v1`).
- **Health**: `healthx` with a `postgres` checker (readiness = Postgres only; Kafka is not a check in any service); readiness is the overall status, `liveness` is independent of dependencies; gitops sets `probes.livenessService: liveness`.
- **Auth and errors**: the hand-written principal interceptor is replaced by `auth` (`UnaryServerInterceptor`, `RequirePrincipal`, `RequireScopes`, `RequireService`); errors go through one `grpcx.ErrorTable`. **Behaviour changes (accepted)**: a cancelled or timed-out request returns `Canceled` / `DeadlineExceeded` instead of `Internal`; a unique-key violation (`23505`) returns `AlreadyExists`; the `PermissionDenied` message text is the SDK text (`missing scope`, `principal type not allowed`); the code of every authorization outcome is unchanged.
- **Unchanged**: both RPCs and the proto, every filter, ordering `created_at DESC, id DESC`, page size 20 / max 100, the decimal-offset cursor (`pagination.Clamp` only; `EncodeOffset` is base64 and is NOT used), `page.total`, the schema and the `idx_audit_events_action_created` index, the generic `Internal "internal error"` for storage failures.
- **Lifecycle, logs, tracing**: `lifecycle.Run` (drain health, then stop within `SHUTDOWN_GRACE`), `logx` (JSON, service attribute, principal and trace ids), `otelx` (no-op unless `OTEL_ENABLED=true`), panic recovery from `grpcx.NewServer`.
- **Not used (reason in design.md)**: `auditx` (team-audit is the audit store, it does not audit itself), `upstream`, `kafkax`/`consume`, `outbox`, `idempotency`, `flags`, `jwtx`, `resilience`, `mockdata`.
- **Deploy track** (change `deploy-go-sdk-v1`, listed in tasks.md): compose migrate service runs `migrate up`, remove `DATABASE_ENABLED`, set `GRPC_REFLECTION_ENABLED=true` locally, add `ENV` to gitops, set `probes.livenessService`, add the Argo PreSync migrate Job; the shared deploy-runtime requirement on unsafe configuration is MODIFIED only by that change.

## Impact

Code: `team-audit/**` only. Contract: none (proto and gateway untouched). Existing specs stay valid; the Go test names cited by `audit-log-query`, `audit-access-control` and `audit-query-contract` are kept and move packages (map in design.md).

## NFR

- Availability: with Postgres down, readiness (`""`) is NOT_SERVING within one `healthx.DefaultInterval` while `liveness` stays SERVING, so a database outage does not restart pods. On SIGTERM both turn NOT_SERVING first, then in-flight calls finish within `SHUTDOWN_GRACE` (default 10 s).
- Boot: an unreachable database fails `Run` within the SDK 5 s ping bound with a non-zero exit. `migrate up` fails with a non-zero exit on an unreachable database or a dirty state, and `Run` never migrates.
- Latency (unchanged targets, local Postgres): `WriteAuditEvent` p99 under 100 ms; `QueryAuditLog` with any filter combination and page size 100 p99 under 300 ms.
- Limits: page size default 20, max 100; one event per write; no new limits.
- Durability: an acknowledged `WriteAuditEvent` is always committed to Postgres; there is no code path that acknowledges a write without it.

## Data classification

No new personal field is collected. The existing fields, kept as is:

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `audit_events.actor_id` | pseudonymous user or service id | accountability for privileged and money actions | legitimate interest (security, fraud, dispute evidence) | platform log retention; the service never deletes rows |
| `audit_events.target_type`, `target_id` | resource ids (may be user ids) | locate what was acted on | legitimate interest | platform log retention |
| `audit_events.metadata` | free-form strings chosen by the calling service (reasons, ids) | evidence | legitimate interest | platform log retention |

Logs: the service logs error causes, never filter values (`actor_id`, `target_id`, `action`) or metadata; the principal id is the only user id in a log line, added by the SDK logger. No DPO sign-off is required (user rule).

## Threat model

- Assets: the integrity and durability of the audit trail; the confidentiality of who did what to whom.
- Abuse 1, forging the trail: an end user, anonymous caller, admin or scopeless service writes an event. Control: `auth.RequireService(ctx, "audit.write")` as the first statement of `WriteAuditEvent`; users (admins included) get `PermissionDenied`, no principal gets `Unauthenticated`; the gateway does not route the RPC.
- Abuse 2, silent loss: the database is down and the service keeps events in memory. Control: no fallback exists; boot fails, a write on a lost connection returns `Internal` and the fail-closed producers return `Unavailable`.
- Abuse 3, reading the trail: a non-admin queries it. Control: `auth.RequireScopes(ctx, "admin")` first; authorization precedes validation.
- Abuse 4, information leak: storage or driver text in an error, reflection in production. Control: `grpcx.Status` maps unknown errors to `"internal error"`; reflection is off by default and refused when `ENV` is staging or prod.
- Abuse 5, a dependency outage cascades into restarts. Control: separate `liveness` service.
- Abuse 6, schema drift or two schema owners. Control: only `migrate up` (one-shot job) changes the schema; the serving process holds no DDL path at boot.
- Authz rule: `WriteAuditEvent` = service principal holding `audit.write`; `QueryAuditLog` = authenticated principal holding exactly `admin`; health carries no data and needs no principal.
