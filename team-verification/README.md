# team-verification

Mock seller/user KYC verification service (Go, gRPC). Bounded context: **verification**. It
owns the `verification_db` database (database-per-service) and the
`platform.verification.v1.VerificationService` gRPC API on port **50064**: users submit a
document *reference*, an admin approves or rejects it, and anyone authorized can read the
resulting status and verified-badge flag.

Status: deployed in the root compose stack (`team-verification-svc`, no host port published). It is
a **mock**: only an opaque `doc_ref` string (storage key) is stored, never real document bytes.

Go 1.26 (`go.mod`; the Dockerfile builds with `golang:1.26`), module
`github.com/buidangphuc/team-verification`. The platform layer (config, logging, tracing, auth,
health, lifecycle, Postgres, gRPC server chain) is
[go-sdk](../platform-core/packages/go-sdk/README.md) v0.1.1, vendored in `third_party/go-sdk` (see
`third_party/go-sdk/VERSION`) by `platform-core/tools/sync-go-sdk.sh`; never edit it by hand. Layout
follows [ADR-0016](../platform-core/docs/ADR/0016-go-service-convention-and-sdk.md):

| Path | Role |
|---|---|
| `cmd/server/main.go` | Signal context, `config.Load`, `app.Run`, exit code. |
| `internal/app` | Composition root: logger, telemetry, health, store, gRPC server, lifecycle. |
| `internal/config` | `Settings` (SDK groups) and the `.env.example` drift test. |
| `internal/rpc` | gRPC handlers, the error table, and the caller checks (`auth.go`). |
| `internal/verification` | Domain: the KYC lifecycle (submit, review, status, badge). |
| `internal/store` | Postgres and in-memory KYC stores. |
| `migrations/` | SQL migrations plus `embed.go` (package `migrations`, for tests). |

## Contract

Served: `platform.verification.v1.VerificationService` (proto vendored in `proto/platform/verification/v1`)
on `:50064`, plus the standard gRPC health service and gRPC reflection (always on). Health is SERVING
while the database answers a ping, NOT_SERVING when it does not (checked every 5s) and during shutdown
drain. A panic in a handler is recovered and returned as `Internal`; every call gets a request id and a
log line. Identity comes from gateway-forwarded metadata (`x-principal-id`, `x-principal-type`,
`x-principal-scopes`, ADR-0003); the service does no JWT handling. The caller is `team-gateway`
(`UPSTREAM_VERIFICATION_ADDR`, default `team-verification-svc:50064`).

| RPC | Request -> response | Authorization | Errors |
|---|---|---|---|
| `SubmitKyc` | `doc_type`, `doc_ref` -> `id`, `status` (`PENDING`) | USER principal. Owner is the principal id, never the body. | `Unauthenticated` (none/anonymous/unknown type), `PermissionDenied` (service principal), `InvalidArgument` (blank `doc_type`/`doc_ref`) |
| `GetVerificationStatus` | `user_id` -> `status`, `badge` | Empty or own `user_id`: USER principal only. Any other `user_id`: `admin` scope. | `Unauthenticated`, `PermissionDenied` |
| `ReviewKyc` | `id`, `decision` -> `status` | `admin` scope, and a reviewer cannot review their own submission. | `Unauthenticated`, `PermissionDenied` (missing scope / own submission), `InvalidArgument` (empty id, bad decision), `NotFound`, `FailedPrecondition` (already reviewed) |

Behavior notes:

- A caller is authenticated only when its principal type is `user` or `service`. A missing, `anonymous`
  or unknown type is `Unauthenticated`. The scope check uses the SDK's `auth.RequireScopes`, which
  answers `PermissionDenied` with the message `missing scope`.
- `decision` accepts (case-insensitive) `approve|approved|verify|verified` -> `VERIFIED` and
  `reject|rejected|deny|denied` -> `REJECTED`.
- A review is final: only a `PENDING` submission can be updated (guarded in the SQL `UPDATE`).
- `badge` is `status == VERIFIED`. A user with no submission is reported as `PENDING`, no badge.
- Status is that of the user's latest submission (by `created_at`).

Consumes: nothing upstream, only the principal metadata the gateway forwards.

## Events

None. No Kafka or RabbitMQ in this service.

## Data

Database `verification_db` (role `verification_svc`), table `kyc_submissions`:
`id`, `user_id`, `doc_type`, `doc_ref`, `status` (`PENDING|VERIFIED|REJECTED`, CHECK),
`reviewed_at`, `created_at`. A user may have many submissions. Index
`idx_kyc_submissions_user_id (user_id, created_at DESC)` serves the latest-submission lookup.

Migrations: `migrations/001_create_kyc_submissions.{up,down}.sql` (golang-migrate). The service never
applies them at boot; in compose the one-shot `team-verification-migrate` job applies them before the
service starts. The root `migrations` package embeds the files for the store test.

## Configuration

`internal/config` binds the SDK groups from env; `.env.example` lists every key and
`TestEnvExampleInSync` fails when the two drift. `GRPC_PORT` is required (no default; the Dockerfile
and compose set 50064).

| Var | Default | Meaning |
|---|---|---|
| `ENV` | `local` | `local` or `test` allow the in-memory fallback; anything else requires a reachable database. |
| `LOG_LEVEL`, `LOG_JSON` | `info`, `true` | Logging. |
| `GRPC_HOST`, `GRPC_PORT` | `0.0.0.0`, required | Listen address. |
| `SHUTDOWN_GRACE_SECONDS`, `SHUTDOWN_DRAIN_DELAY_SECONDS` | `10`, `0` | Shutdown drains health first, then stops within the grace period. |
| `DATABASE_URL`, `DB_MAX_CONNS` | required, `10` | Postgres. A missing `DATABASE_URL` fails at load. |
| `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME` | `false`, none, none | Telemetry, off by default. |

There are no enable flags: `DATABASE_ENABLED` and `GRPC_REFLECTION_ENABLED` no longer exist.

Database behaviour: the pool is opened with a ping. Outside `ENV=local` an unreachable database fails
boot (non-nil error, port never opened). With `ENV=local` the service logs a warning and serves from the
in-memory store.

## Run locally

Root compose (from the repo root):

```bash
docker compose -f docker-compose.services.yaml up --build team-verification
```

This starts Postgres, runs `team-verification-migrate`, then `team-verification-svc` on `:50064`. It is
not a `jobs`-profile service.

Standalone: start a Postgres with `verification_db`/`verification_svc`, apply `migrations/*.up.sql`, copy
`.env.example` to `.env`, export the vars (the code does not load `.env` itself), then `make run`.
`.env.example` points at port `5444`, while compose uses the in-network `postgres:5432`. Without a
database, run with `ENV=local GRPC_PORT=50064 DATABASE_URL=postgres://x@127.0.0.1:1/x make run`; the
unreachable URL triggers the in-memory fallback.

## Build, test and lint

Targets come from `make/go-service.mk` (synced; `make help` lists them).

| Command | Does |
|---|---|
| `make proto` | `buf generate` into `generated/` (remote buf.build plugins, needs network) |
| `make build` | `go build ./...` |
| `make test` | `go test -race ./...`; `DOCKER_HOST` is taken from the docker context |
| `make lint` | golangci-lint with the synced `.golangci.yml` |
| `make check` | vet, lint, tests (Docker tests must run, not skip), `.env.example` drift check |
| `make docker` | build the image `team-verification:dev` |

Run `make proto` first on a fresh checkout and again after a proto re-sync, because `generated/` is
gitignored. Tests that start Postgres use testcontainers, so Docker must be running; `make check` fails
instead of skipping when it is not.

## Spec and verification

- `FEATURES.yaml` has the 2 `automated` features `verification.buyer-cannot-self-approve` and
  `verification.admin-approves-seller`, plus the runtime scenarios of change
  `migrate-team-verification-to-go-sdk` as `not-testable` entries that name their unit tests.
- E2E coverage: `platform-e2e/tests/e2e/features/services/verification_access.feature`.
  Verify with `make -C platform-e2e features-check`, and for a change
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.

## Gotchas

- `generated/` is gitignored. Run `make proto` after cloning and after any proto change.
- `proto/` is vendored from platform-core. Never edit it here.
- In-memory fallback: only with `ENV=local` and an unreachable database. Submissions are then lost on
  restart, and health stays SERVING because there is no database to check.
- Without gateway-forwarded principal metadata every RPC returns `Unauthenticated`.

## Known gaps

- Principal metadata is trusted as-is, so the service must only be reachable from the gateway
  (network-level zero trust, ADR-0010).
- No `.github` CI in this repo.

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md)
- ADRs: [`0003-auth-model`](../platform-core/docs/ADR/0003-auth-model.md),
  [`0010-service-zero-trust`](../platform-core/docs/ADR/0010-service-zero-trust.md),
  [`0001-proto-distribution`](../platform-core/docs/ADR/0001-proto-distribution.md),
  [`0016-go-service-convention-and-sdk`](../platform-core/docs/ADR/0016-go-service-convention-and-sdk.md)
