# team-audit

Go gRPC service that owns the **append-only audit trail** bounded context: who (`actor_id`) did what (`action`) to which target (`target_type` + `target_id`), with free-form string metadata. It is a deployed service (compose: `team-audit-svc`, gRPC `:50066`, no host port published) with its own Postgres database `audit_db` (Rule 3, database per service). It has no upstream dependencies and publishes or consumes no events. The gateway exposes both RPCs to the web API (`team-gateway/internal/edge/audit.go`, `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066`).

Go 1.26 (`go.mod`; the Dockerfile builds with `golang:1.26`), module `github.com/buidangphuc/team-audit`. The platform layer (config, logging, tracing, auth, health, lifecycle, Postgres, gRPC server chain) is [go-sdk](../platform-core/packages/go-sdk/README.md) v1.0.0, vendored in `third_party/go-sdk` (see `third_party/go-sdk/VERSION`) by `platform-core/tools/sync-go-sdk.sh`; never edit it by hand. Layout follows [ADR-0016](../platform-core/docs/ADR/0016-go-service-convention-and-sdk.md):

| Path | Role |
|---|---|
| `cmd/server/main.go` | Signal context, `config.Load`, `app.Run`, exit code. |
| `internal/app` | Composition root: logger, telemetry, health, store, gRPC server, lifecycle. |
| `internal/config` | `Settings` (SDK groups) and the `.env.example` drift test. |
| `internal/rpc` | gRPC handlers, the error table, cursor decoding and the local auth helpers (`auth.go`). |
| `internal/audit` | Domain: validation (`ErrEmptyAction`) and page-size clamping. |
| `internal/store` | Postgres and in-memory audit stores (`AuditRepository`). |
| `migrations/` | SQL migrations plus `embed.go` (package `migrations`, for tests). |

## Contract

Served: `platform.audit.v1.AuditService` (`proto/platform/audit/v1/audit.proto`) on `:50066`, plus the standard gRPC health service and gRPC reflection (always on). Health is SERVING while the database answers a ping, NOT_SERVING when it does not (checked every 5s) and during shutdown drain. A panic in a handler is recovered and returned as `Internal`; every call gets a request id and a log line. The principal comes from gateway-forwarded metadata `x-principal-id`, `x-principal-type`, `x-principal-scopes`, resolved by the SDK auth interceptor, which never rejects: each handler authorizes as its first statement (`internal/rpc/auth.go`). A missing, `anonymous` or ANONYMOUS-type principal is `Unauthenticated`.

| RPC | Authorization and behaviour |
|---|---|
| `WriteAuditEvent` | Service-only: requires a `service` principal holding `audit.write` (`Unauthenticated` with no/anonymous principal, `PermissionDenied` for users incl. admins and scopeless services). The stored `actor_id` is the request's `actor_id` if set, else the service's own id. Empty `action` returns `InvalidArgument`. The response is empty (fire-and-forget); the server assigns `id` (`audit_` + 8 chars of a UUID) and `created_at`. Storage failures return a generic `Internal` ("internal error"); the cause is logged server-side. |
| `QueryAuditLog` | Admin only: authenticated, then scope `admin` (`PermissionDenied` otherwise; a service principal also needs `admin`). Optional filters `actor_id` and `target_type` (exact match). Newest first (`created_at DESC, id DESC`). The page cursor is a row offset as a decimal string; an empty or malformed cursor means the first page. `page_size` defaults to 20 and is capped at 100 (`internal/audit/audit.go`). `next_cursor` is set only while more rows remain; `total` is the match count before paging. |

Auth uses the SDK's `auth.RequirePrincipal` and `auth.RequireScopes` (exact scope match, no admin bypass). The SDK has no service-type check, so `requireService` in `internal/rpc/auth.go` adds one: after `RequirePrincipal`, a non-`service` principal is `PermissionDenied`. The status codes are the ones team-audit had before the migration; only the message text changed (`missing scope` instead of `insufficient_scope: missing "<scope>"`).

Consumes: nothing (no upstream RPCs).

## Events

None. There is no Kafka or RabbitMQ code in this repo.

## Data

Database `audit_db` (role `audit_svc`) on the stack's shared Postgres. One table, `audit_events`, from `migrations/001_create_audit_events.up.sql`:

| Column | Type |
|---|---|
| `id` | `VARCHAR(64)` primary key |
| `actor_id` | `VARCHAR(64)` not null, default `''` |
| `action` | `VARCHAR(128)` not null |
| `target_type`, `target_id` | `VARCHAR(64)` not null, default `''` |
| `metadata` | `JSONB` not null, default `{}` |
| `created_at` | `TIMESTAMPTZ` not null, default `NOW()` (the service writes `NOW()`) |

Indexes: `(actor_id, created_at DESC)` and `(target_type, target_id, created_at DESC)`. Rows are immutable: the code never issues `UPDATE` or `DELETE`, and there is no such RPC.

Migrations are applied only by the compose one-shot `team-audit-migrate` (golang-migrate, `up`), which the service waits on. The service itself never migrates. The root `migrations` package embeds the files for the store test.

## Configuration

`internal/config` binds the SDK groups from env; `.env.example` lists every key and `TestEnvExampleInSync` fails when the two drift. `GRPC_PORT` is required (no default; the Dockerfile and compose set 50066).

| Var | Default | Meaning |
|---|---|---|
| `ENV` | required, no default | `local` or `test` allow the in-memory fallback; anything else requires a reachable database. |
| `LOG_LEVEL`, `LOG_JSON` | `info`, `true` | Logging. |
| `GRPC_HOST`, `GRPC_PORT` | `0.0.0.0`, required | Listen address. |
| `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY` | `10s`, `0s` | Shutdown drains health first, then stops within the grace period. |
| `DATABASE_URL`, `DB_MAX_CONNS` | required, `10` | Postgres. A missing `DATABASE_URL` fails at load. |
| `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME` | `false`, none, none | Telemetry, off by default. |

There are no enable flags: the database and reflection are always on (`DATABASE_ENABLED` and `GRPC_REFLECTION_ENABLED` are gone). Database behaviour: the pool is opened with a ping. Outside `ENV=local` an unreachable database fails boot (non-nil error, port never opened). With `ENV=local` the service logs a warning and serves from the in-memory store.

## Run locally

Root compose (from the repo root, `docker-compose.services.yaml`):

```
docker compose -f docker-compose.services.yaml up --build team-audit
```

This brings up `postgres`, `team-audit-migrate` and `team-audit`. No port is published; reach it through `team-gateway` (`:8080`) (`POST /platform.audit.v1.AuditService/<Method>`) or a gRPC client that sets the `x-principal-*` metadata.

Standalone: start a Postgres with `audit_db`/`audit_svc`, apply `migrations/*.up.sql`, copy `.env.example` to `.env`, export the vars (the code does not load `.env` itself), then `make run`. Note `.env.example` points at port `5447`, while compose uses the in-network `postgres:5432`. Without a database, run with `ENV=local GRPC_PORT=50066 DATABASE_URL=postgres://x@127.0.0.1:1/x make run`; the unreachable URL triggers the in-memory fallback.

## Build, test and lint

Targets come from `make/go-service.mk` (synced; `make help` lists them).

| Command | Does |
|---|---|
| `make proto` | `buf generate` into `generated/` (remote buf.build plugins, needs network) |
| `make build` | `go build ./...` |
| `make test` | `go test -race ./...`; `DOCKER_HOST` is taken from the docker context |
| `make lint` | golangci-lint with the synced `.golangci.yml` |
| `make check` | vet, lint, tests (Docker tests must run, not skip), `.env.example` drift check |
| `make docker` | build the image `team-audit:dev` |

Run `make proto` first on a fresh checkout and again after a proto re-sync, because `generated/` is gitignored. The app and store tests start Postgres with testcontainers, so Docker must be running; `make check` fails instead of skipping when it is not. The other suites use the in-memory store.

## Spec and verification

- `FEATURES.yaml` has the automated gateway scenario "A refused edge write leaves no audit event" and the runtime scenarios of change `migrate-team-audit-to-go-sdk` as `not-testable` entries that name their unit tests. The written-event scenario is "A written audit event is returned by QueryAuditLog" in `platform-e2e/tests/e2e/features/services/new_services.feature`.
- Verify from the repo root: `make -C platform-e2e features-check` and `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>` at the repo root): propose, implement code and e2e together, verify, archive. See the root `README.md` (ASDLC).

## Gotchas

- `generated/` is gitignored (root `.gitignore`) and must be produced with `make proto`. The Dockerfile builds from the local directory, so the image build fails if `generated/` is absent. Never hand-edit it.
- `proto/` is vendored from platform-core. Never edit it here; change the contract in platform-core, then re-sync.
- In-memory fallback: only with `ENV=local` and an unreachable database. Events are then lost on restart, and health stays SERVING because there is no database to check.
- Pagination is an offset, so rows written between pages can shift results.
- Principal metadata is trusted as forwarded; the service is meant to be reachable only through the gateway or from other services.
- `coverage.out` and `coverage.summary` are gitignored test artifacts.

## Known gaps

- `QueryAuditLog` supports only `actor_id` and `target_type` filters; there is no `target_id` or time-range filter even though an index on `(target_type, target_id, created_at)` exists.

## Links

- Rules and recipe: root `AGENTS.md`.
- ADRs in `platform-core/docs/ADR/`: 0001 proto distribution, 0003 / 0006 / 0010 auth and zero trust, 0016 Go service convention and SDK.
