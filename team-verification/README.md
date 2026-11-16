# team-verification

Seller/user KYC verification service (Go, gRPC), built on `platform-core/packages/go-sdk` at a tagged release, currently `v1.0.3` (ADR-0016). Bounded context: **verification**. It
owns the `verification_db` database (database-per-service) and the
`platform.verification.v1.VerificationService` gRPC API on port **50064**: users submit a
document *reference*, an admin approves or rejects it, and anyone authorized can read the
resulting status and verified-badge flag.

Status: deployed in the root compose stack. Only an opaque `doc_ref` string (storage key) is
stored, never real document bytes. The service always uses Postgres; there is no memory mode.

Layout: `cmd/server` (config load, `migrate up` subcommand) -> `internal/app` (wiring: logx, otelx, healthx, mockdata, postgres, auditx, grpcx, lifecycle) -> `internal/rpc` -> `internal/verification` -> `internal/store`; `internal/upstream` adapts team-audit to `auditx.Writer`.

## Contract

Served: `platform.verification.v1.VerificationService` (proto vendored in `proto/platform/verification/v1`).
Identity comes from gateway-forwarded metadata (`x-principal-id`, `x-principal-type`,
`x-principal-scopes`, ADR-0003); the service does no JWT handling. The caller is
`team-gateway` (`UPSTREAM_VERIFICATION_ADDR`, default `team-verification-svc:50064`).

| RPC | Request -> response | Authorization | Errors |
|---|---|---|---|
| `SubmitKyc` | `doc_type`, `doc_ref`, `additional_doc_refs` (1..3 keys from `CreateKycUploadUrl`) -> `id`, `status` (`PENDING`) | USER principal. Owner is the principal id, never the body. Each key must be the caller's unused grant with an uploaded object matching its type and size; earlier submissions get `superseded_at`. | `Unauthenticated` (none/anonymous), `PermissionDenied` (service principal), `InvalidArgument` (blank `doc_type`/`doc_ref`, more than 3 docs, unknown/foreign/used key), `FailedPrecondition` (expired grant, object missing or mismatching) |
| `GetVerificationStatus` | `user_id` -> `status`, `badge`, `reject_reason_code` | Empty or own `user_id`: USER principal only. Any other `user_id`: `admin` scope, or a service principal with `verification.read` (no `reject_reason_code`). | `Unauthenticated`, `PermissionDenied` |
| `CreateKycUploadUrl` | `content_type`, `size_bytes` -> presigned PUT `upload_url`, `doc_ref` | USER principal; at most 3 open grants | `Unauthenticated`, `PermissionDenied`, `InvalidArgument`, `ResourceExhausted` |
| `ListKycSubmissions` | `status`, cursor, page size -> submissions (oldest first) | `admin` scope | `PermissionDenied`, `InvalidArgument` |
| `GetKycDocumentUrl` | `submission_id`, `index` -> presigned GET (5 min) | `admin` scope; audited before the URL is issued | `PermissionDenied`, `NotFound`, `InvalidArgument`, `Unavailable` |
| `GetPublicBadges` | 1..50 `user_ids` -> badges | Any caller, including anonymous | `InvalidArgument` |
| `ReviewKyc` | `id`, `decision`, `reason_code` -> `status` | `admin` scope, and a reviewer cannot review their own submission. | `Unauthenticated`, `PermissionDenied` (missing scope / own submission), `InvalidArgument` (empty id, bad decision, `reason_code` missing on reject or set on approve), `NotFound`, `FailedPrecondition` (already reviewed) |

Behavior notes:

- `decision` accepts (case-insensitive) `approve|approved|verify|verified` -> `VERIFIED` and
  `reject|rejected|deny|denied` -> `REJECTED`.
- A review is final: only a `PENDING` submission can be updated (guarded in the SQL `UPDATE`).
- `badge` is `status == VERIFIED`. A user with no submission is reported as `NOT_SUBMITTED`, no badge.
- `reason_code` is lower_snake_case, at most 64 characters, required on reject and empty on approve.
- Upload grants: `image/jpeg`, `image/png` or `application/pdf`, 1..5242880 bytes, valid 15 minutes.
- Status is that of the user's latest submission (by `created_at`).
- gRPC health (`grpc.health.v1`) is always enabled: the overall status is readiness (follows the database), the `liveness` service does not depend on the database. Server reflection is off unless `GRPC_REFLECTION_ENABLED=true` in a local or test `ENV`.

Consumes: team-audit (`WriteAuditEvent`, service principal `service-team-verification` with scope
`audit.write`, single attempt bounded to 2 s by `auditx.Gate`).

`ReviewKyc` writes a `kyc.review` audit event (target `kyc_submission`, metadata `decision` and
`user_id`) after validation and before the decision is applied. It fails closed: any audit error,
returns `Unavailable` ("audit trail unavailable") and leaves the submission `PENDING`. `UPSTREAM_AUDIT_ADDR` is required in every ENV: boot fails without it.
Submit and status do not depend on team-audit.

## Events

Consumes `identity.events` (group `team-verification-identity`): `UserErased` deletes the user's
KYC objects first, then their submissions, documents and upload grants in one transaction. A
storage outage returns a retryable error; a malformed event is permanent. Dead letters go to
`identity.events.team-verification.dlq`. Other event types are ignored. Publishes nothing.

A retention sweeper runs hourly: documents of a `REJECTED` submission 90 days after review, and of
a superseded submission 90 days after `superseded_at`, are deleted from storage first and then marked
`deleted_at`; a failed object delete is retried on the next run.

## Data

Database `verification_db` (role `verification_svc`), table `kyc_submissions`:
`id`, `user_id`, `doc_type`, `doc_ref`, `status` (`PENDING|VERIFIED|REJECTED`, CHECK),
`reviewed_at`, `created_at`, `reason_code`, `superseded_at`; plus `kyc_upload_grants` and
`kyc_documents` (migration `002_kyc_documents`). A user may have many submissions. Index
`idx_kyc_submissions_user_id (user_id, created_at DESC)` serves the latest-submission lookup.

Migrations: `migrations/001_create_kyc_submissions.{up,down}.sql`, `migrations/002_kyc_documents.{up,down}.sql` (golang-migrate). In compose
they run via the one-shot `team-verification-migrate` job, which the service depends on.
The binary embeds the migrations: `server migrate up` (reads `DATABASE_URL`) applies them and exits. `app.Run` never migrates.

## Configuration

`internal/config.Settings` is composed of the go-sdk config groups (`Runtime`, `GRPC`, `Postgres`, `OTel`, `MockData`) plus `UPSTREAM_AUDIT_ADDR`, loaded with `sdkconfig.Load`. `.env.example` lists these variables.

| Var | Default | Meaning |
|---|---|---|
| `ENV` | none | Required. `local`, `test`, `staging` or `prod`; strict environments refuse unsafe options |
| `LOG_LEVEL` | `info` | Log level |
| `LOG_JSON` | `true` | JSON log output |
| `GRPC_HOST` | `0.0.0.0` | gRPC bind host |
| `GRPC_PORT` | none | Required. gRPC listen port (`50064` in the Dockerfile and compose) |
| `GRPC_REFLECTION_ENABLED` | `false` | gRPC reflection; only allowed when `ENV` is local or test |
| `SHUTDOWN_GRACE` | `10s` | Graceful shutdown timeout |
| `SHUTDOWN_DRAIN_DELAY` | `0s` | Delay between NOT_SERVING and stop |
| `DATABASE_URL` | none | Required. Postgres DSN; an unreachable database fails boot |
| `DB_MAX_CONNS` | `10` | Postgres pool size |
| `UPSTREAM_AUDIT_ADDR` | none | Required. team-audit gRPC address |
| `MOCK_DATA` | empty | Mock-data adapters; the service has none, so any value fails boot (refused in strict ENV) |
| `STORAGE_ENDPOINT`, `STORAGE_BUCKET`, `STORAGE_ACCESS_KEY`, `STORAGE_SECRET_KEY` | none | Required. MinIO bucket holding KYC documents |
| `STORAGE_REGION`, `STORAGE_USE_SSL` | `us-east-1`, `false` | Storage region and TLS |
| `STORAGE_PUBLIC_ENDPOINT` | empty (= `STORAGE_ENDPOINT`) | Host the presigned upload and download URLs are signed for, reachable from the browser |
| `KAFKA_BROKERS` | none | Kafka brokers (go-sdk `Kafka` group) |
| `KAFKA_IDENTITY_TOPIC` | `identity.events` | UserErased source topic |
| `KAFKA_IDENTITY_DLQ_TOPIC` | `identity.events.team-verification.dlq` | Dead-letter topic |
| `KAFKA_IDENTITY_GROUP` | `team-verification-identity` | Consumer group |
| `OTEL_ENABLED` | `false` | Enable OpenTelemetry export |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | OTLP endpoint |
| `OTEL_SERVICE_NAME` | `team-verification` | OTel service name |

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
go run ./cmd/server migrate up   # needs DATABASE_URL
make run     # go run ./cmd/server
```

## Build, test, lint

| Command | Does |
|---|---|
| `make proto` | `buf generate` into `generated/` |
| `make build` | `go build ./...` |
| `make check` | Local gate from `platform-core/make/go-service.mk` (format, lint, vet, build, tests) |
| `make test` | `go test ./...` (store and app tests use Docker Postgres via testkit) |
| `make tidy` | `go mod tidy` |

Lint config comes from the go-sdk `.golangci.yml`.

## Spec and verification

- `FEATURES.yaml` (repo root) has 28 features: 6 `automated` (the self-approval, approval, rejection and audit
  scenarios) and 22 `not-testable` unit-level entries.
- E2E coverage: `platform-e2e/tests/e2e/features/services/verification_access.feature` and
  `services/verification_kyc_audit.feature`.
  Verify with `make -C platform-e2e features-check`, and for a change
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.

## Gotchas

- `generated/` is gitignored. Run `make proto` after cloning and after any proto change.
  `buf.gen.yaml` uses remote buf.build plugins, so it needs network access.
- `proto/` is vendored from platform-core. Never edit it here.
- Without gateway-forwarded principal metadata every RPC returns `Unauthenticated`.

## Known gaps

- Principal metadata is trusted as-is, so the service must only be reachable from the gateway
  (network-level zero trust, ADR-0010).
- No `.github` CI in this repo.
- `docker-compose.services.yaml` sets `IDENTITY_EVENTS_TOPIC` for team-verification, but the
  service reads `KAFKA_IDENTITY_TOPIC` (`internal/config/config.go`); it works only because the
  default is `identity.events`.

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md)
- ADRs: [`0003-auth-model`](../platform-core/docs/ADR/0003-auth-model.md),
  [`0010-service-zero-trust`](../platform-core/docs/ADR/0010-service-zero-trust.md),
  [`0001-proto-distribution`](../platform-core/docs/ADR/0001-proto-distribution.md)
