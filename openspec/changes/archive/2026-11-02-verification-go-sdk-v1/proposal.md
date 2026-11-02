## Why

team-verification is a hand-rolled Go service with the platform defects the go-sdk migration removes:

- `cmd/server/main.go` calls `pgxpool.New`, which connects lazily, so the in-memory repository fallback ("mock mode") never triggers on a dead database and, when it does, the service serves KYC state that vanishes on restart.
- Health is always SERVING, whatever the database does.
- No panic recovery, request ids or tracing; reflection is always registered.
- Auth, config, the gRPC server and the audit client are local copies of what `go-sdk` v1.0.2 now provides.

Decisions already made by the user (recipe, 2026-10-22/23): require the tag `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2` (no `replace`, no `third_party`), no dependency enable flags, no runtime in-memory or noop fallbacks, mock means mock data served by the real service. Source: `plans/go-sdk-migration/PLAN.md` (G3, G4).

initiative: go-sdk-migration

## What Changes

- Layout per ADR-0016: `cmd/server/main.go` calls `internal/app.Run(ctx, settings)`; `internal/config` on SDK `config`; `internal/rpc` (was `handler`), `internal/verification` (was `service`), `internal/store` (was `repository`), `internal/upstream` (audit writer). `internal/grpcserver` and `internal/interceptor` are deleted: their work moves to SDK `grpcx` and `auth`.
- **BREAKING (config)**: `DATABASE_ENABLED` and `UPSTREAM_CALL_TIMEOUT_SECONDS` are gone; `ENV`, `GRPC_PORT` (was default 50064), `DATABASE_URL` (was a local default on localhost:5444) and `UPSTREAM_AUDIT_ADDR` become required in every ENV (no fallback mode without an audit connection); `LOG_LEVEL`, `LOG_JSON` and `GRPC_HOST` were ignored and are now live (defaults keep today's behaviour: `info`, JSON, `GRPC_HOST` empty so the listener binds all interfaces as `:port` did); `ENV` accepts only `local`, `test`, `staging`, `prod`, and a strict ENV is any ENV other than `local` or `test`; `GRPC_REFLECTION_ENABLED` is off by default and refused outside `ENV=local|test`; new `OTEL_*`, `LOG_*`, `SHUTDOWN_*`, `MOCK_DATA` keys come from SDK groups.
- **BREAKING (runtime)**: reflection was always on and is now off by default; the database is mandatory in every `ENV` including `local`: an unreachable database fails boot (`postgres.Open` pings). The in-memory repository fallback is deleted from the runtime; an in-memory store survives only as the test fake `internal/store/storetest`.
- Health: `postgres` checker on the overall (readiness) status; the `liveness` service stays serving while the database is down.
- Audit-first KYC review moves onto `auditx.Gate` and `auditx.Dial` (service principal `service-team-verification`, scope `audit.write`); the client-visible refusal text becomes "audit trail unavailable" (code `Unavailable` unchanged). The audit write is bounded by the SDK's fixed 2 s; `UPSTREAM_CALL_TIMEOUT_SECONDS` is removed. **BREAKING (accepted)**: the old client cut the audit timeout to the remaining inbound deadline minus a 500 ms margin (`boundedCtx`, `deadlineMargin`); `auditx.Gate` applies a plain 2 s `WithTimeout` on the caller context, so that margin is gone. The caller's own deadline still bounds the write, but the 500 ms headroom to return `Unavailable` before the caller's deadline is dropped.
- Errors: `grpcx.Status` with an `ErrorTable` replaces `mapErr`; unique violation (23505) maps to `AlreadyExists`; internal causes stay hidden.
- Mock: team-verification has no external data adapter to mock (it stores references only, never documents). `MOCK_DATA` is declared so the strict-ENV refusal applies, with an empty adapter list: any name is rejected at boot as unknown.
- Makefile includes `../platform-core/make/go-service.mk` with the SDK `.golangci.yml`; `golangci-lint` is pinned as a `go tool`; Dockerfile from `platform-core/templates/go-service/Dockerfile`; `migrations/` gains `embed.go` (embedded SDK `pkg/migrate` files) and the binary gains a `migrate up` subcommand; the service never migrates at boot (it only opens Postgres), compose keeps a `team-verification-migrate` service running the service image with `migrate up`, and gitops gets an Argo PreSync migrate Job (both in change deploy-go-sdk-v1). The service has no Kafka dependency, so Kafka is not a readiness check.

No proto, RPC, schema or event change. Auth, scope, self-review, PENDING-final and audit-before-apply rules of `kyc-audit` are unchanged. A principal whose type is not `user`, `service` or `anonymous` (empty or unrecognised, for example `robot`) stays `Unauthenticated` (type guard kept in `internal/rpc`, scenarios "A principal with no type is refused" and "A principal with an unrecognised type is refused"; the SDK takes the type as a raw string and would let it pass), so no fail-closed authn check is dropped. Client-visible message texts change through the SDK; codes do not, and the gateway passes only codes.

## Non-goals

- Compose and gitops edits (listed in tasks.md, done by change `deploy-go-sdk-v1`).
- Running e2e per service (done once at the end).
- A real KYC document provider or document storage.

## Impact

- Repo: `team-verification` (all Go code, Makefile, Dockerfile, `.env.example`, README, FEATURES.yaml).
- Capabilities: `verification-runtime` (new), `kyc-audit` (two requirements modified, three re-stated only to cite the new `internal/rpc` package paths so no stale path remains after archive).
- Local runs: the `localhost:5444` default `DATABASE_URL` is removed, so a bare `go run` without env now fails; `.env.example` carries a working local `DATABASE_URL` and `TestEnvExampleInSync` also checks that it loads through `Load`.
- Deploy: every environment must set `ENV`, `DATABASE_URL`, `GRPC_PORT` and `UPSTREAM_AUDIT_ADDR`; `probes.livenessService: liveness`; local compose sets `GRPC_REFLECTION_ENABLED=true`; migrations run by `migrate up` (compose service, Argo PreSync Job).

## Non-functional requirements

- Shutdown: on SIGINT/SIGTERM health drains first, then the gRPC server stops within `SHUTDOWN_GRACE` (default 10 s); `app.Run` returns nil.
- Readiness flips to NOT_SERVING within one health interval (`healthx.DefaultInterval`) of the database failing; liveness never depends on the database, so a database outage does not restart pods.
- Boot: an unreachable database fails `Run` within the SDK ping bound of 5 s, before the gRPC port opens.
- Audit write: single attempt, no retry, bounded at 2 s (`auditx`); a call refused before the audit step never touches team-audit.
- Readiness is Postgres only (no Kafka, no upstream check: team-audit down must not make the pod unready, because SubmitKyc and status do not need it).
- Availability: SubmitKyc and GetVerificationStatus depend only on Postgres, never on team-audit.
- Limits: `DB_MAX_CONNS` default 10; no new request size limit is introduced.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `user_id` (principal id) | personal (pseudonymous id) | owner of a KYC submission, audit actor and `metadata.user_id` | legitimate interest: fraud prevention and seller trust | with the submission row (no new data; erasure follows the platform erasure process); platform log retention for logs |
| `doc_type`, `doc_ref` | sensitive (opaque reference to an identity document, never the document) | reviewer looks up the stored document | legitimate interest: seller verification | with the submission row; never written to audit metadata or logs |
| `status`, `reviewed_at`, decision | personal (verification outcome) | badge eligibility and audit | legitimate interest | with the submission row; audit copy per team-audit retention |

No new data is collected. New in logs: request id, principal id and trace id from `logx`; `doc_ref` and `doc_type` are never logged and are never formatted into error strings (SDK rule). No DPO sign-off is required (user rule): basis legitimate interest, platform log retention.

## Threat model

- Assets: KYC status and badge (grants seller trust), document references, the audit trail of reviews.
- Abuse cases:
  - a buyer approves their own KYC (denied: `admin` scope and self-review check, unchanged);
  - an admin approves while team-audit is down to avoid a trace (denied: fail closed `Unavailable`, nothing applied);
  - the service silently serves from memory after a database outage, losing or inventing KYC state (removed: boot fails, readiness NOT_SERVING);
  - reflection or `MOCK_DATA` left on in staging or prod (refused by `Load` before validation);
  - an identity document reference leaking through logs or error text (never logged, sentinel errors);
  - a caller bypassing the gateway with forged `x-principal-*` metadata (unchanged trust model of ADR-0003: the network policy admits only team-gateway).
- Authz rule: `SubmitKyc` requires a user principal; `GetVerificationStatus` another user's status and `ReviewKyc` require the `admin` scope; the reviewer is never the submitter; the actor in the audit event comes from the verified principal, never the request.
