# team-audit

Go gRPC service that owns the **append-only audit trail** bounded context: who (`actor_id`) did what (`action`) to which target (`target_type` + `target_id`), with free-form string metadata. It is a deployed service (compose: `team-audit-svc`, gRPC `:50066`, no host port published) with its own Postgres database `audit_db` (Rule 3, database per service). It has no upstream dependencies and publishes or consumes no events. The gateway exposes both RPCs to the web API (`team-gateway/internal/edge/audit.go`, `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066`).

Go 1.22 (`go.mod`; the Dockerfile builds with `golang:1.22`), module `github.com/buidangphuc/team-audit`.

## Contract

Served from `proto/platform/audit/v1/audit.proto` (vendored; see Gotchas): `platform.audit.v1.AuditService`. The principal comes from gateway-forwarded metadata `x-principal-id`, `x-principal-type`, `x-principal-scopes` (`internal/interceptor/auth.go`). The interceptor never rejects; each handler authorizes as its first statement. `RequirePrincipal` rejects a missing, `anonymous` or ANONYMOUS-type principal with `Unauthenticated`.

| RPC | Authorization and behaviour |
|---|---|
| `WriteAuditEvent` | Service-only: requires a `service` principal holding `audit.write` (`Unauthenticated` with no/anonymous principal, `PermissionDenied` for users incl. admins and scopeless services). The stored `actor_id` is the request's `actor_id` if set, else the service's own id. Empty `action` returns `InvalidArgument`. The response is empty (fire-and-forget); the server assigns `id` (`audit_` + 8 chars of a UUID) and `created_at`. Storage failures return a generic `Internal` ("internal error"); the cause is logged server-side. |
| `QueryAuditLog` | Admin only: authenticated, then scope `admin` (`PermissionDenied` otherwise; a service principal also needs `admin`). Optional filters `actor_id` and `target_type` (exact match). Newest first (`created_at DESC, id DESC`). The page cursor is a row offset as a decimal string; an empty or malformed cursor means the first page. `page_size` defaults to 20 and is capped at 100 (`internal/service/audit.go`). `next_cursor` is set only while more rows remain; `total` is the match count before paging. |

Also served: gRPC health (`""` is SERVING) and server reflection (always on).

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

Migrations are applied only by the compose one-shot `team-audit-migrate` (golang-migrate, `up`), which the service waits on. The service itself never migrates.

## Configuration

The code reads only these two variables (`internal/config/config.go`); an empty value falls back to the default.

| Variable | Default | Purpose |
|---|---|---|
| `GRPC_PORT` | `50066` | gRPC listen port (also the Dockerfile `ENV` and `EXPOSE`) |
| `DATABASE_URL` | `postgres://audit_svc:audit_pass@localhost:5447/audit_db?sslmode=disable` | Postgres DSN |

`.env.example` also lists `ENV`, `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_REFLECTION_ENABLED`, `SHUTDOWN_GRACE_SECONDS`, `DATABASE_ENABLED`, `DB_MAX_CONNS` and `OTEL_*`. None of them is read: the logger is always JSON on stdout, reflection is always on, the server listens on all interfaces, and shutdown is `GracefulStop` with no timeout. The compose service sets `DATABASE_ENABLED` and `ENV`, which are equally inert. There is no `.env.example` drift gate for this repo.

## Run locally

From the repo root (Postgres and the `platform-core_default` network come from the shared stack):

```bash
docker compose up -d --build team-audit      # runs team-audit-migrate first
```

or the whole stack with `docker compose up -d --build`. Standalone, against a migrated Postgres:

```bash
make proto                                    # once, writes generated/
DATABASE_URL=postgres://... make run          # go run ./cmd/server
```

Call it through the gateway (`POST /platform.audit.v1.AuditService/<Method>`), or directly with a gRPC client that sets the `x-principal-*` metadata; reflection is on.

## Build, test and lint

```bash
make proto      # buf generate (needs buf and network access to buf.build remote plugins)
make build      # go build ./...
make test       # go test ./...
make tidy       # go mod tidy
go vet ./...
gofmt -l .      # should list nothing outside generated/
```

There is no lint target, no `.github/workflows` and no other CI config here; run the commands above as the gate. Tests use the in-memory repository, so they need no database.

## Spec and verification

- There is no `FEATURES.yaml` in this repo. E2E coverage is the scenario "A written audit event is returned by QueryAuditLog" in `platform-e2e/tests/e2e/features/services/new_services.feature`.
- Verify from the repo root: `make -C platform-e2e features-check` and `make -C platform-e2e spec-check CHANGE=<id>` (every scenario of an OpenSpec change is green).
- Changes go through OpenSpec (`openspec/changes/<id>` at the repo root): propose, implement code and e2e together, verify, archive. See the root `README.md` (ASDLC).

## Gotchas

- `generated/` is gitignored (root `.gitignore`) and must be produced with `make proto`. The Dockerfile builds from the local directory, so the image build fails if `generated/` is absent. Never hand-edit it. The first comment line of the `Makefile` says the same.
- `proto/` is vendored from platform-core. Never edit it here; change the contract in platform-core. Use the root `buf.gen.yaml`, which rewrites `go_package` to this module.
- In-memory fallback: if `pgxpool.New` returns an error, `main.go` logs a warning and uses the in-memory repository, so events are lost on restart. `pgxpool.New` connects lazily, so a bad or unreachable DB usually does not trigger the fallback; the service starts with the Postgres repo and RPCs fail with `Internal`.
- Pagination is an offset, so rows written between pages can shift results.
- Principal metadata is trusted as forwarded; the service is meant to be reachable only through the gateway or from other services.
- `coverage.out` and `coverage.summary` are gitignored test artifacts.

## Known gaps

- `QueryAuditLog` supports only `actor_id` and `target_type` filters; there is no `target_id` or time-range filter even though an index on `(target_type, target_id, created_at)` exists.
- `.env.example` documents many variables the code ignores (see Configuration).
- No FEATURES.yaml, so `features-check` has no manifest for this service.
- No tracing: `OTEL_*` is declared but nothing configures a tracer.

## Links

- Rules and recipe: root `AGENTS.md`.
- ADRs in `platform-core/docs/ADR/`: 0001 proto distribution, 0003 / 0006 / 0010 auth and zero trust.
