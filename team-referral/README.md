# team-referral

Go gRPC service for the **referral program** bounded context: per-user referral codes, redemption of
someone else's code (each user can be referred at most once), and a **mock coin-credit** reward ledger.
It owns the `referral_db` Postgres database; no other service writes these tables. Rewards are
accounting intent only: no wallet or money movement happens here. Deployed in the local compose stack
on gRPC port **50062**; reached from clients through `team-gateway` (`UPSTREAM_REFERRAL_ADDR`,
default `team-referral-svc:50062`).

## Contract

Service `platform.referral.v1.ReferralService` (`proto/platform/referral/v1/referral.proto`, vendored).
Also registers gRPC health (`""` = SERVING) and gRPC reflection (always on).

Authorization rule (same for every RPC): the handler resolves the caller from the `x-principal-id` gRPC
metadata via the go-sdk auth interceptor (`auth.RequirePrincipal` in `internal/rpc`) and **never from the request body**.
A missing or empty `x-principal-id` returns `Unauthenticated`, and so does the gateway's anonymous principal (`x-principal-id` equal to `anonymous` or `x-principal-type` equal to `anonymous`). Every RPC acts only on the caller's own
data; there is no admin or cross-user path. There are no `RequireScopes` / scope checks (see Known gaps).

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
(golang-migrate `up` against the shared `postgres` container). The service itself never migrates.

## Configuration

Settings are loaded by `internal/config` through go-sdk `pkg/config` (go-sdk v1.0.2). `.env.example` lists exactly the settings; `make check-env` verifies it.

| Variable | Default | Notes |
|---|---|---|
| `ENV` | `local` | Required. Deployment environment. |
| `LOG_LEVEL` | `info` | Log level. |
| `LOG_JSON` | `true` | JSON logs. |
| `GRPC_HOST` | `0.0.0.0` | gRPC bind host. |
| `GRPC_PORT` | `50062` | Required. gRPC port. |
| `GRPC_REFLECTION_ENABLED` | `true` | gRPC reflection. |
| `SHUTDOWN_GRACE` | `10s` | Graceful shutdown grace period. |
| `SHUTDOWN_DRAIN_DELAY` | `0s` | Delay before draining on shutdown. |
| `DATABASE_URL` | `postgres://referral_svc:referral_pass@localhost:5444/referral_db?sslmode=disable` | Required. Compose uses `@postgres:5432`. |
| `DB_MAX_CONNS` | `10` | Postgres pool size. |
| `OTEL_ENABLED` | `false` | Enable OpenTelemetry. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | OTLP endpoint. |
| `OTEL_SERVICE_NAME` | `team-referral` | OTel service name. |

## Run locally

Root stack (from the repo root):

```
docker compose up -d --build            # whole stack incl. postgres, team-referral-migrate, team-referral
docker compose up -d --build team-referral   # this service + its migrate job and Postgres
```

`team-referral` has no published host port in compose; reach it via `team-gateway`. There is no
`--profile jobs` component for this service.

Standalone: `make run` (`go run ./cmd/server`) needs generated code (see Gotchas) and a Postgres
reachable at `DATABASE_URL` with the migration applied.

## Build, test, lint

Layout follows ADR-0016 on go-sdk `v1.0.2`: `cmd/server` (entry), `internal/app` (wiring), `internal/config` (settings), `internal/rpc` (gRPC handlers), `internal/referral` (domain logic), `internal/store` (Postgres store, `internal/store/fake` for tests).

| Command | Does |
|---|---|
| `make proto` | `buf generate` into `generated/` (needs `buf` and network for the remote plugins in `buf.gen.yaml`). |
| `make build` / `make test` / `make tidy` | `go build ./...` / `go test ./...` / `go mod tidy`. |
| `make check` | vet + lint + test + check-env. |

The Dockerfile builds with Go 1.26 (`go.mod` is `go 1.26.0`).

## Spec and verification

- No `FEATURES.yaml` in this repo. The only e2e-facing coverage entry is `account.referral` in
  `team-frontend/FEATURES.yaml` (services `team-frontend`, `team-referral`).
- E2E coverage is verified in `platform-e2e`: `make -C platform-e2e features-check` (CI gate) and
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Behavior changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC.
  No OpenSpec change specific to this service exists today.

## Gotchas

- `generated/` is gitignored (root `.gitignore`: `**/generated/`). A fresh clone does not compile
  (imports `.../team-referral/generated/...`) and the Dockerfile `go build` also fails without it. Run
  `make proto` first.
- `proto/` is vendored from platform-core (ADR-0001). Never edit it here; change the contract in
  platform-core and re-vendor.

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
