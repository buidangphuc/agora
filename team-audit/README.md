# team-audit

Go gRPC service that owns the **append-only audit trail** bounded context: who (`actor_id`) did what (`action`) to which target (`target_type` + `target_id`), with free-form string metadata. It is a deployed service (compose: `team-audit-svc`, gRPC `:50066`, no host port published) with its own Postgres database `audit_db` (Rule 3, database per service). It has no upstream dependencies and publishes or consumes no events. The gateway exposes both RPCs to the web API (`team-gateway/internal/edge/audit.go`, `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066`).

Go 1.26 (`go.mod`; the Dockerfile builds with `golang:1.26`), module `github.com/buidangphuc/team-audit`.

## Contract

Served from `proto/platform/audit/v1/audit.proto` (vendored; see Gotchas): `platform.audit.v1.AuditService`. The principal comes from gateway-forwarded metadata `x-principal-id`, `x-principal-type`, `x-principal-scopes` (`go-sdk` auth via `grpcx`). The `grpcx` auth interceptor only populates the principal; each handler authorizes as its first statement. `RequirePrincipal` rejects a missing, `anonymous` or ANONYMOUS-type principal with `Unauthenticated`.

| RPC | Authorization and behaviour |
|---|---|
| `WriteAuditEvent` | Service-only: requires a `service` principal holding `audit.write` (`Unauthenticated` with no/anonymous principal, `PermissionDenied` for users incl. admins and scopeless services). The stored `actor_id` is the request's `actor_id` if set, else the service's own id. Empty `action` returns `InvalidArgument`. The response is empty (fire-and-forget); the server assigns `id` (`audit_` + 8 chars of a UUID) and `created_at`. Storage failures return a generic `Internal` ("internal error"); the cause is logged server-side. |
| `QueryAuditLog` | Admin only: authenticated, then scope `admin` (`PermissionDenied` otherwise; a service principal also needs `admin`). Optional filters, combined with AND: `actor_id`, `target_type`, `target_id`, `action` (all exact match; absent or empty means no filter) and a `created_at` range (`created_after` inclusive, `created_before` exclusive; `created_after` later than `created_before` returns `InvalidArgument`, equal bounds match nothing). Newest first (`created_at DESC, id DESC`). The page cursor is a row offset as a decimal string; an empty or malformed cursor means the first page. `page_size` defaults to 20 and is capped at 100 (`internal/audit`). `next_cursor` is set only while more rows remain; `total` is the match count before paging. |

Also served: gRPC health (`""` is SERVING) and server reflection (only when `GRPC_REFLECTION_ENABLED=true`, local use).

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

Indexes: `(actor_id, created_at DESC)`, `(target_type, target_id, created_at DESC)` and `(action, created_at DESC)` (migration `002_audit_action_created_index`). Rows are immutable: the code never issues `UPDATE` or `DELETE`, and there is no such RPC.

The binary embeds the migrations: `server migrate up` (reads `DATABASE_URL`) applies them and exits non-zero on failure or a dirty state. `app.Run` never migrates and fails boot when Postgres is unreachable.

## Configuration

Loaded by the go-sdk config (`internal/config/config.go`): `GRPC_PORT` (required, Dockerfile default `50066`), `GRPC_HOST`, `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY`, `DATABASE_URL` and the `ENV`, `LOG_*` and `OTEL_*` groups. Health is served as `""` (readiness, follows Postgres) and `liveness`; shutdown drains health first.

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

Call it through the gateway (`POST /platform.audit.v1.AuditService/<Method>`), or directly with a gRPC client that sets the `x-principal-*` metadata; reflection is available when `GRPC_REFLECTION_ENABLED=true`.

## Build, test and lint

```bash
make proto      # buf generate (needs buf and network access to buf.build remote plugins)
make build      # go build ./...
make test       # go test ./...
make tidy       # go mod tidy
go vet ./...
gofmt -l .      # should list nothing outside generated/
```

There is no lint target, no `.github/workflows` and no other CI config here; run the commands above as the gate. Store and app tests run against a Postgres testcontainer (`GO_SDK_REQUIRE_DOCKER=1`).

## Spec and verification

- `FEATURES.yaml` lists the audit features; their e2e coverage is in `platform-e2e/tests/e2e/features/security/audit_access.feature` and `security/audit_query_filters.feature`.
- Verify from the repo root: `make -C platform-e2e features-check` and `make -C platform-e2e spec-check CHANGE=<id>` (every scenario of an OpenSpec change is green).
- Changes go through OpenSpec (`openspec/changes/<id>` at the repo root): propose, implement code and e2e together, verify, archive. See the root `README.md` (ASDLC).

## Gotchas

- `generated/` is gitignored (root `.gitignore`) and must be produced with `make proto`. The Dockerfile builds from the local directory, so the image build fails if `generated/` is absent. Never hand-edit it. The first comment line of the `Makefile` says the same.
- `proto/` is vendored from platform-core. Never edit it here; change the contract in platform-core. Use the root `buf.gen.yaml`, which rewrites `go_package` to this module.
- Pagination is an offset, so rows written between pages can shift results.
- Principal metadata is trusted as forwarded; the service is meant to be reachable only through the gateway or from other services.
- `coverage.out` and `coverage.summary` are gitignored test artifacts.

## Known gaps

- None recorded.

## Links

- Rules and recipe: root `AGENTS.md`.
- ADRs in `platform-core/docs/ADR/`: 0001 proto distribution, 0003 / 0006 / 0010 auth and zero trust.
