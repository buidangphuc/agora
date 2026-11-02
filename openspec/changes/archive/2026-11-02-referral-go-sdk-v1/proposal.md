## Why

team-referral hand-rolls config, logging, gRPC server, health and shutdown, and in `cmd/server/main.go` it falls back to an in-memory repository ("mock mode") when `pgxpool.New` fails. The owner decided (2026-10-22) that a service which imports the database always uses it, and that services depend on the tagged SDK module, not on copies.

- On main today 0 of 15 Go services import the SDK (`plans/go-sdk-migration/PLAN.md`, Why). team-referral is a Step 1 service: 2 commits on main since the 2026-10-13 migration, and the least coupled one (no Kafka, no outbox, no upstream calls).
- The in-memory fallback is a runtime code path that skips persistence. It also hides a real failure: `pgxpool.New` does not dial, so a bad URL silently serves from memory and loses data on restart (README Gotchas).
- `DATABASE_ENABLED` is set in compose but read by no code, and `.env.example` lists 9 more inert variables.
- In-cluster, `platform-gitops/envs/services/team-referral.yaml` sets no `env:` block at all; the service relies on the code defaults `GRPC_PORT=50062` and the localhost `DATABASE_URL`. The SDK requires `ENV` and `GRPC_PORT` and has no `DATABASE_URL` default, so the gitops values must gain them in the same change.

Source of the requirements: `plans/go-sdk-migration/PLAN.md` (Steps 1, "Per-service recipe", "Kept flags", "Shared lint/make files"), the `go-platform-sdk` spec including the `go-sdk-v1-0-1` requirements and the v1.0.2 changes (no `OpenOrLocal`, reflection off by default, `liveness` health service), ADR-0016 and ADR-0018.

initiative: go-sdk-migration

## What Changes

- Add `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2` to `team-referral/go.mod` (tagged module: no `replace`, no `third_party`).
- Adopt the ADR-0016 layout: `cmd/server/main.go` (signals, settings, exit code) calls `internal/app.Run(ctx, settings)`; `internal/config` (SDK groups `Runtime`, `GRPC`, `Postgres`, `OTel`); `internal/rpc` (transport, error table); `internal/referral` (domain, ex `internal/service`); `internal/store` (Postgres store, ex `internal/repository`); `migrations/embed.go`.
- SDK packages used: `config` (Load, Runtime, GRPC, Postgres, OTel), `logx`, `otelx`, `healthx`, `lifecycle`, `grpcx`, `auth`, `postgres`, `pagination` (`Clamp`), `testkit` and `migrate` (tests). `kafkax`, `consume`, `outbox`, `upstream`, `mockdata`, `auditx` are not used (see Non-goals).
- **BREAKING (config)**: delete the runtime in-memory "mock mode" (`NewInMemoryReferralRepo` fallback in `cmd/server/main.go`). The service always uses Postgres; with an unreachable or unset database it refuses to boot in every `ENV`, including `local`. It uses `postgres.Open` (`OpenOrLocal` is gone in v1.0.2). The in-memory repository moves to `internal/store/fake` and is imported by tests only.
- **BREAKING (config)**: `DATABASE_URL` loses its localhost default and becomes required; `ENV` and `GRPC_PORT` become required (no default). `DATABASE_ENABLED` is deleted from compose and `.env.example`; `GRPC_REFLECTION_ENABLED` is kept (read now). The inert variable `SHUTDOWN_GRACE_SECONDS` is replaced by the SDK names (`SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY`); `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `DB_MAX_CONNS` and `OTEL_*` are now actually read. **BREAKING (ops)**: gRPC reflection is off unless `GRPC_REFLECTION_ENABLED=true` (kept debug flag; refused when true in a strict ENV); `.env.example` and compose set it `true` for local use only.
- Behaviour changes that follow from the SDK, all deliberate: the readiness (empty-name) health service reports NOT_SERVING while Postgres is down and the new `liveness` service stays SERVING; panics are recovered by the SDK chain; shutdown drains and stops within `SHUTDOWN_GRACE` (was `GracefulStop` without a bound); request id and principal id are logged per call and spans are created when `OTEL_ENABLED=true`; a Postgres unique violation that reaches the handler maps to `AlreadyExists` (was `Internal`; only a random-code collision can trigger it) and a cancelled or timed-out request maps to `Canceled` / `DeadlineExceeded` (was `Internal`).
- Behaviour kept (main wins): every RPC, error code and message, the reward of `10000` minor units with reason `invitee_joined`, newest-first listing with the decimal row-offset cursor (the SDK's base64 `EncodeOffset` would change tokens clients hold, so the cursor codec stays local; only `pagination.Clamp` is adopted), and the rule that a malformed cursor restarts at offset 0.
- `Makefile` includes `../platform-core/make/go-service.mk` and passes `LINT_CONFIG=../platform-core/packages/go-sdk/.golangci.yml`; no service-local lint file or make copy.
- Files to delete or edit outside `team-referral/` (all in the write-set of tasks.md):
  - `docker-compose.services.yaml`, service `team-referral`: delete `- DATABASE_ENABLED=true`; keep `ENV=local`, `GRPC_PORT=50062`, `DATABASE_URL=...@postgres:5432/...`. The `team-referral-migrate` job is unchanged.
  - `team-referral/.env.example`: delete `DATABASE_ENABLED`, `SHUTDOWN_GRACE_SECONDS`; keep `GRPC_REFLECTION_ENABLED=true` (local only); add `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY`; keep `ENV`, `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_PORT`, `DATABASE_URL`, `DB_MAX_CONNS`, `OTEL_*` (every key must be a tag of `Settings`; `TestEnvExampleInSync`).
  - `platform-gitops/envs/services/team-referral.yaml` and the mirrored inline block in `platform-gitops/argocd/apps/team-referral.yaml`: nothing to delete (there is no flag line); ADD `probes.livenessService: liveness` (chart opt-in) and `env: {ENV: "local", GRPC_PORT: "50062"}` (baseline), and new overlays `platform-gitops/envs/staging/services/team-referral.yaml` (`ENV: "staging"`) and `platform-gitops/envs/prod/services/team-referral.yaml` (`ENV: "production"`), same shape as `envs/prod/services/team-search.yaml`. `DATABASE_URL` comes from the Vault secret `svc/team-referral` via `secrets.enabled: true`.
  - `team-referral/README.md`: Configuration table = the `Settings` variables; drop the "In-memory fallback" gotcha and the "inert variables" sentences; new layout, SDK tag, `make check`; Contract text unchanged.
- No spec requirement describes the referral flag or fallback today (`openspec/specs/` has no referral-service capability; `deploy-runtime` names only team-search), so nothing is MODIFIED or REMOVED. The delta ADDS capability `referral-service`, which records the current behaviour as testable requirements (characterisation) plus the always-Postgres rule.

## Non-goals

- Kafka and the OrderCompleted reward consumer (backlog Q7/Q37, a separate change). team-referral consumes no Kafka topic today (README "Events: None"; `go.mod` has no Kafka client; reward is written synchronously in `RedeemReferral`). The `order-lifecycle-contract` comment that `OrderCompleted` "is consumed by the referral reward" describes an intended consumer that does not exist in the code. This change does not add it, so `pkg/consume` and `key_fp` logging are not used.
- `pkg/mockdata`: the "mock coin credit" is an accounting ledger, not a swappable outer adapter, so `MOCK_DATA` is not declared.
- Scope gates (`RequireScopes`), transactional redeem, `page.total` as ledger size: known gaps, unchanged.
- Proto changes; generated code stays gitignored and produced by `make proto`.

## Impact

`team-referral/**` (go.mod, go.sum, Makefile, `.env.example`, README, cmd, internal, migrations/embed.go, new FEATURES.yaml), `docker-compose.services.yaml`, `platform-gitops/envs/services/team-referral.yaml`, `platform-gitops/argocd/apps/team-referral.yaml`, `platform-gitops/envs/{staging,prod}/services/team-referral.yaml` (new). Callers (`team-gateway`, `team-frontend`) see no wire change.

## Non-functional requirements

- Latency: target (not gated by a test) unary RPC p99 under 50 ms against a local Postgres; the change adds only SDK interceptors (request id, recovery, logging, auth).
- Availability: single replica as today. Health is SERVING only while Postgres answers a ping; an outage turns it NOT_SERVING within 10 seconds (2 x `healthx.DefaultInterval` of 5 s) instead of serving `Internal` errors. Liveness (`liveness` service) does not depend on the database, so a database outage removes the pod from Service endpoints but does not restart it. Boot fails fast without a database (no degraded in-memory mode).
- Shutdown: on context cancellation (SIGTERM) the server drains for `SHUTDOWN_DRAIN_DELAY` then stops within `SHUTDOWN_GRACE` (default 10s); in-flight RPCs finish or are cut at the bound.
- Limits: page size default 20, max 100; `DB_MAX_CONNS` default 10; image stays distroless static, nonroot, port 50062.
- Compatibility: wire contract and error codes unchanged (exceptions listed in What Changes); database schema and migrations unchanged.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `user_id` (`referral_codes`, `referrals.referrer_id/referee_id`, `referral_rewards`) | personal (pseudonymous account id) | own the code, attribute a referee once, own the reward ledger | legitimate interest (running the referral programme) | life of the account; unchanged by this change |
| `code` | personal when joined to `user_id` | resolve a code to its owner on redeem | legitimate interest | with the owner row |
| `amount`, `reason` | non-sensitive accounting intent (mock coin credit, no money movement) | ledger | legitimate interest | with the owner row |
| request id, principal id in logs | personal (pseudonymous) | traceability | legitimate interest | platform log retention |

No new personal field is collected, stored or logged. The redeemed `code` is never written to logs (the SDK logs method, status, request id, principal id only). A database URL with credentials is never logged: the SDK logs boot errors with the driver error text only, and the README states the value comes from Vault.

## Threat model

- Assets: the referral code to owner mapping, the one-referral-per-user rule, the reward ledger (accounting intent), the database credentials in `DATABASE_URL`.
- Abuse cases:
  - Acting as another user: the caller id is read only from gateway metadata (`x-principal-id`, `x-principal-type`) by `auth.UnaryServerInterceptor`; no RPC reads an identity from the request body.
  - Anonymous reaching user data: `auth.RequirePrincipal` rejects the gateway's anonymous principal (id `anonymous`, or type `anonymous`) with `Unauthenticated`.
  - Self-referral and double redeem for reward farming: rejected by `FailedPrecondition`; the database unique `referee_id` backs the second rule.
  - Silent data loss: the removed in-memory fallback could serve and drop writes after an unparseable URL; now the process exits.
  - Config drift exposing a weaker mode: `DATABASE_ENABLED` and every other `*_ENABLED` dependency flag are not read; removed names in the environment are ignored.
  - Information leak through errors: unmapped errors become `Internal` "internal error" (`grpcx.Status`); driver text stays in logs.
- Authorization rule (unchanged): authenticated, own data only. Any principal that is non-anonymous may call every RPC for itself; there is no admin or cross-user path and no scope gate.
- Residual risk (unchanged, ADR-0010 interim): the service trusts `x-principal-*` from any peer that can reach port 50062; the gitops NetworkPolicy limits ingress to `team-gateway`.
