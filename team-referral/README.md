# team-referral

Go gRPC service for the **referral program** bounded context: per-user referral codes, redemption of
someone else's code (each user can be referred at most once), and a **mock coin-credit** reward ledger.
It owns the `referral_db` Postgres database; no other service writes these tables. Rewards are
accounting intent only: no wallet or money movement happens here. Deployed in the local compose stack
on gRPC port **50062**; reached from clients through `team-gateway` (`UPSTREAM_REFERRAL_ADDR`,
default `team-referral-svc:50062`).

Go 1.26 (`go.mod`; the Dockerfile builds with `golang:1.26`), module `github.com/buidangphuc/team-referral`. The platform layer (config, logging, tracing, auth, health, lifecycle, Postgres, gRPC server chain) is [go-sdk](../platform-core/packages/go-sdk/README.md) v0.1.1, vendored in `third_party/go-sdk` (see `third_party/go-sdk/VERSION`) by `platform-core/tools/sync-go-sdk.sh`; never edit it by hand. Layout follows [ADR-0016](../platform-core/docs/ADR/0016-go-service-convention-and-sdk.md):

| Path | Role |
|---|---|
| `cmd/server/main.go` | Signal context, `config.Load`, `app.Run`, exit code. |
| `internal/app` | Composition root: logger, telemetry, health, store, gRPC server, lifecycle. |
| `internal/config` | `Settings` (SDK groups) and the `.env.example` drift test. |
| `internal/rpc` | gRPC handlers and the error table; requires an authenticated caller on every RPC. |
| `internal/referral` | Domain: code minting, redemption rules, reward ledger, pagination. |
| `internal/store` | Postgres and in-memory referral stores. |
| `migrations/` | SQL migrations plus `embed.go` (package `migrations`, for tests). |

## Contract

Service `platform.referral.v1.ReferralService` (`proto/platform/referral/v1/referral.proto`, vendored).
Also serves the standard gRPC health service and gRPC reflection (always on). Health is SERVING while the
database answers a ping, NOT_SERVING when it does not (checked every 5s) and during shutdown drain. A panic in
a handler is recovered and returned as `Internal`; every call gets a request id and a log line.

Authorization rule (same for every RPC): the handler resolves the caller from the `x-principal-id` gRPC
metadata through the SDK `auth` interceptor and `auth.RequirePrincipal`, and **never from the request body**.
A missing or empty `x-principal-id` returns `Unauthenticated`, and so does the gateway's anonymous principal
(`x-principal-id` equal to `anonymous` or `x-principal-type` equal to `anonymous`). Every RPC acts only on the
caller's own data; there is no admin or cross-user path. There are no `RequireScopes` / scope checks (see Known gaps).

| RPC | Behavior | Errors |
|---|---|---|
| `CreateReferralCode` | Returns the caller's code, minting one (`REF` + 8 chars of a UUID) if none exists. Idempotent. | `Unauthenticated` |
| `GetMyReferral` | Caller's code (minted on first read), `invited_count`, `rewards_total` (minor units). | `Unauthenticated` |
| `RedeemReferral` | Caller redeems `code` owned by another user: records the referral, appends a reward of `10000` minor units, reason `invitee_joined`, to the **referrer's** ledger. | `InvalidArgument` empty code; `NotFound` unknown code; `FailedPrecondition` own code or caller already redeemed; `Unauthenticated` |
| `ListReferralRewards` | Caller's own rewards, newest first. Cursor is a decimal row offset; default page size 20, max 100. `page.total` is the length of the returned page, not the full count. | `Unauthenticated` |

Consumes: no upstream RPCs. Only Postgres.

## Events

None. The service neither produces nor consumes Kafka or RabbitMQ messages.

## Data

Database `referral_db` (role `referral_svc`), migration `migrations/001_create_referrals.up.sql`:

| Table | Purpose |
|---|---|
| `referral_codes` | One code per user (`user_id` PK, `code` unique). |
| `referrals` | `referrer_id`, `referee_id` (unique, so a user is referred once), `code`; index on `referrer_id`. |
| `referral_rewards` | Append-only mock ledger: `id`, `user_id`, `amount` (BIGINT minor units), `reason`; index `(user_id, created_at DESC)`. |

Migrations are applied by the `team-referral-migrate` job in the root `docker-compose.services.yaml`
(golang-migrate `up` against the shared `postgres` container). The service itself never migrates. The root `migrations` package embeds the files for the store test.

## Configuration

`internal/config` binds the SDK groups from env; `.env.example` lists every key and `TestEnvExampleInSync` fails when the two drift. `GRPC_PORT` is required (no default; the Dockerfile and compose set 50062).

| Var | Default | Meaning |
|---|---|---|
| `ENV` | `local` | `local` or `test` allow the in-memory fallback; anything else requires a reachable database. |
| `LOG_LEVEL`, `LOG_JSON` | `info`, `true` | Logging. |
| `GRPC_HOST`, `GRPC_PORT` | `0.0.0.0`, required | Listen address. |
| `SHUTDOWN_GRACE_SECONDS`, `SHUTDOWN_DRAIN_DELAY_SECONDS` | `10`, `0` | Shutdown drains health first, then stops within the grace period. |
| `DATABASE_URL`, `DB_MAX_CONNS` | required, `10` | Postgres. A missing `DATABASE_URL` fails at load. |
| `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME` | `false`, none, none | Telemetry, off by default. |

There are no enable flags: `DATABASE_ENABLED` and `GRPC_REFLECTION_ENABLED` no longer exist. Compose sets
`DATABASE_URL` to `...@postgres:5432/referral_db?sslmode=disable` (container network); the `localhost:5444`
URL in `.env.example` is for running on the host.

Database behaviour: the pool is opened with a ping. Outside `ENV=local` an unreachable database fails boot
(non-nil error, port never opened). With `ENV=local` the service logs a warning and serves from the
in-memory store.

## Run locally

Root stack (from the repo root):

```
docker compose up -d --build            # whole stack incl. postgres, team-referral-migrate, team-referral
docker compose up -d --build team-referral   # this service + its migrate job and Postgres
```

`team-referral` has no published host port in compose; reach it via `team-gateway`. There is no
`--profile jobs` component for this service.

Standalone: start a Postgres with `referral_db`/`referral_svc`, apply `migrations/*.up.sql`, copy
`.env.example` to `.env`, export the vars (the code does not load `.env` itself), then `make run`. Without a
database, run with `ENV=local GRPC_PORT=50062 DATABASE_URL=postgres://x@127.0.0.1:1/x make run`; the
unreachable URL triggers the in-memory fallback.

## Build, test, lint

Targets come from `make/go-service.mk` (synced; `make help` lists them).

| Command | Does |
|---|---|
| `make proto` | `buf generate` into `generated/` (remote buf.build plugins, needs network) |
| `make build` | `go build ./...` |
| `make test` | `go test -race ./...`; `DOCKER_HOST` is taken from the docker context |
| `make lint` | golangci-lint with the synced `.golangci.yml` |
| `make check` | vet, lint, tests (Docker tests must run, not skip), `.env.example` drift check |
| `make docker` | build the image `team-referral:dev` |

Run `make proto` first on a fresh checkout and again after a proto re-sync, because `generated/` is
gitignored. Tests that start Postgres use testcontainers, so Docker must be running; `make check` fails
instead of skipping when it is not. The domain and handler tests use the in-memory store; the boot, health
and store tests use a real Postgres. There is no CI workflow in this repo.

## Spec and verification

- `FEATURES.yaml` lists the runtime scenarios of change `migrate-team-referral-to-go-sdk` as `not-testable`
  entries that name their unit tests. The only e2e-facing coverage entry for the referral journey is
  `account.referral` in `team-frontend/FEATURES.yaml` (services `team-frontend`, `team-referral`).
- E2E coverage is verified in `platform-e2e`: `make -C platform-e2e features-check` (CI gate) and
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Behavior changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.

## Gotchas

- `generated/` is gitignored (root `.gitignore`: `**/generated/`). A fresh clone does not compile
  (imports `.../team-referral/generated/...`) and the Dockerfile `go build` also fails without it. Run
  `make proto` first.
- `proto/` is vendored from platform-core (ADR-0001). Never edit it here; change the contract in
  platform-core and re-vendor.
- In-memory fallback: only with `ENV=local` and an unreachable database. Codes, referrals and rewards are then
  lost on restart, and health stays SERVING because there is no database to check.
- The service does not apply migrations at boot; the `team-referral-migrate` job (or you, standalone) does.

## Known gaps

- **Trusts `x-principal-id` from any caller that can reach the port.** There is no mTLS or service
  identity (ADR-0010 interim: network isolation only). The service does not verify that the caller is
  the gateway.
- No scope gates (`RequireScopes`); authorization is "authenticated and own data" only.
- `ListReferralRewards` `page.total` is the page length, not the ledger size.
- `HasRedeemed` exists in the repository but is unused; dedup relies on the `referrals.referee_id`
  unique constraint.
- Reward is a fixed constant (`RewardOnReferral = 10000`) in `internal/referral/referral.go`, and the
  proto comment example reason `invitee_first_order` differs from the reason actually written
  (`invitee_joined`).
- Redeem is not transactional: the referral insert and reward insert are separate statements, so a
  failure in between leaves a referral without a reward.

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md) (service table row for team-referral).
- ADRs (`../platform-core/docs/ADR/`): `0001-proto-distribution.md`, `0003-auth-model.md`,
  `0006-rs256-jwks-auth.md`, `0010-service-zero-trust.md`.
