# team-verification

Mock seller/user KYC verification service (Go, gRPC). Bounded context: **verification**. It
owns the `verification_db` database (database-per-service) and the
`platform.verification.v1.VerificationService` gRPC API on port **50064**: users submit a
document *reference*, an admin approves or rejects it, and anyone authorized can read the
resulting status and verified-badge flag.

Status: deployed in the root compose stack. It is a **mock**: only an opaque `doc_ref`
string (storage key) is stored, never real document bytes.

## Contract

Served: `platform.verification.v1.VerificationService` (proto vendored in `proto/platform/verification/v1`).
Identity comes from gateway-forwarded metadata (`x-principal-id`, `x-principal-type`,
`x-principal-scopes`, ADR-0003); the service does no JWT handling. The caller is
`team-gateway` (`UPSTREAM_VERIFICATION_ADDR`, default `team-verification-svc:50064`).

| RPC | Request -> response | Authorization | Errors |
|---|---|---|---|
| `SubmitKyc` | `doc_type`, `doc_ref` -> `id`, `status` (`PENDING`) | USER principal. Owner is the principal id, never the body. | `Unauthenticated` (none/anonymous), `PermissionDenied` (service principal), `InvalidArgument` (blank `doc_type`/`doc_ref`) |
| `GetVerificationStatus` | `user_id` -> `status`, `badge` | Empty or own `user_id`: USER principal only. Any other `user_id`: `admin` scope. | `Unauthenticated`, `PermissionDenied` |
| `ReviewKyc` | `id`, `decision` -> `status` | `admin` scope, and a reviewer cannot review their own submission. | `Unauthenticated`, `PermissionDenied` (missing scope / own submission), `InvalidArgument` (empty id, bad decision), `NotFound`, `FailedPrecondition` (already reviewed) |

Behavior notes:

- `decision` accepts (case-insensitive) `approve|approved|verify|verified` -> `VERIFIED` and
  `reject|rejected|deny|denied` -> `REJECTED`.
- A review is final: only a `PENDING` submission can be updated (guarded in the SQL `UPDATE`).
- `badge` is `status == VERIFIED`. A user with no submission is reported as `PENDING`, no badge.
- Status is that of the user's latest submission (by `created_at`).
- gRPC health (`grpc.health.v1`) and server reflection are always enabled.

Consumes: nothing upstream, only the `common.v1.Principal` type from the vendored proto.

## Events

None. No Kafka or RabbitMQ in this service.

## Data

Database `verification_db` (role `verification_svc`), table `kyc_submissions`:
`id`, `user_id`, `doc_type`, `doc_ref`, `status` (`PENDING|VERIFIED|REJECTED`, CHECK),
`reviewed_at`, `created_at`. A user may have many submissions. Index
`idx_kyc_submissions_user_id (user_id, created_at DESC)` serves the latest-submission lookup.

Migrations: `migrations/001_create_kyc_submissions.{up,down}.sql` (golang-migrate). In compose
they run via the one-shot `team-verification-migrate` job, which the service depends on.
The service itself never migrates.

## Configuration

Only these are read by the code (`internal/config/config.go`):

| Var | Default | Meaning |
|---|---|---|
| `GRPC_PORT` | `50064` | gRPC listen port (same as the Dockerfile and compose) |
| `DATABASE_URL` | `postgres://verification_svc:verification_pass@localhost:5444/verification_db?sslmode=disable` | Postgres DSN |

`.env.example` also lists `ENV`, `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`,
`GRPC_REFLECTION_ENABLED`, `DATABASE_ENABLED`, `DB_MAX_CONNS`. **These are ignored** (see Known
gaps); compose sets `DATABASE_ENABLED` and `ENV` to no effect. There is no env drift gate in
this repo.

## Run locally

Root compose (from the repo root):

```bash
docker compose -f docker-compose.services.yaml up team-verification
```

This starts Postgres, runs `team-verification-migrate`, then `team-verification-svc` on
`:50064`. It is not a `jobs`-profile service.

Standalone (needs a migrated Postgres, e.g. the compose one):

```bash
make proto   # first, see Gotchas
make run     # go run ./cmd/server
```

## Build, test, lint

| Command | Does |
|---|---|
| `make proto` | `buf generate` into `generated/` |
| `make build` | `go build ./...` |
| `make test` | `go test ./...` (unit tests use the in-memory repo, no DB) |
| `make tidy` | `go mod tidy` |

There is no lint target and no CI config in this repo. Run `gofmt -l .` and `go vet ./...`
manually.

## Spec and verification

- `FEATURES.yaml` (repo root) has 2 features, both `automated`: `verification.buyer-cannot-self-approve`
  and `verification.admin-approves-seller`.
- E2E coverage: `platform-e2e/tests/e2e/features/services/verification_access.feature`.
  Verify with `make -C platform-e2e features-check`, and for a change
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.

## Gotchas

- `generated/` is gitignored. Run `make proto` after cloning and after any proto change.
  `buf.gen.yaml` uses remote buf.build plugins, so it needs network access.
- `proto/` is vendored from platform-core. Never edit it here.
- The in-memory repo is only used when the DSN is malformed: `pgxpool.New` is lazy, so with a
  valid DSN and an unreachable DB the service boots and queries fail at runtime. The
  in-memory repo is not persisted.
- Without gateway-forwarded principal metadata every RPC returns `Unauthenticated`.

## Known gaps

- `GRPC_HOST`, `GRPC_REFLECTION_ENABLED`, `LOG_LEVEL`, `LOG_JSON`, `DB_MAX_CONNS`,
  `DATABASE_ENABLED`, `ENV` appear in `.env.example` but are never read. Reflection is always
  on, the listener binds all interfaces, and logging is always JSON.
- Principal metadata is trusted as-is, so the service must only be reachable from the gateway
  (network-level zero trust, ADR-0010).
- No `.github` CI and no lint config.

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md)
- ADRs: [`0003-auth-model`](../platform-core/docs/ADR/0003-auth-model.md),
  [`0010-service-zero-trust`](../platform-core/docs/ADR/0010-service-zero-trust.md),
  [`0001-proto-distribution`](../platform-core/docs/ADR/0001-proto-distribution.md)
