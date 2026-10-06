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
metadata via the interceptor (`internal/interceptor/principal.go`) and **never from the request body**.
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

`internal/config/config.go` reads only two variables:

| Variable | Default |
|---|---|
| `GRPC_PORT` | `50062` |
| `DATABASE_URL` | `postgres://referral_svc:referral_pass@localhost:5444/referral_db?sslmode=disable` |

Compose sets `DATABASE_URL` to `...@postgres:5432/referral_db?sslmode=disable` (container network);
the `localhost:5444` default is for running on the host. Compose also sets `DATABASE_ENABLED` and
`ENV`, which the code ignores. The remaining `.env.example` entries (`LOG_LEVEL`, `LOG_JSON`,
`GRPC_HOST`, `GRPC_REFLECTION_ENABLED`, `SHUTDOWN_GRACE_SECONDS`, `DB_MAX_CONNS`, `OTEL_*`) are **not
read** by any code. Logging is always JSON via `slog`. There is no `.env.example` drift gate for this repo.

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

| Command | Does |
|---|---|
| `make proto` | `buf generate` into `generated/` (needs `buf` and network for the remote plugins in `buf.gen.yaml`). |
| `make build` / `make test` / `make tidy` | `go build ./...` / `go test ./...` / `go mod tidy`. |

Tests: `internal/handler/referral_test.go`, `internal/service/referral_test.go` and `internal/interceptor/principal_test.go` (anonymous handling), using the in-memory
repository (no DB needed). There is no lint target and no CI workflow in this repo; `proto/buf.yaml`
configures buf lint (STANDARD) and breaking (FILE) but nothing runs them here. The Dockerfile builds with
Go 1.22 (`go.mod` is `go 1.22`).

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
- **In-memory fallback:** `cmd/server/main.go` falls back to the in-memory repository when
  `pgxpool.New` returns an error. `pgxpool.New` does not dial eagerly, so an unreachable DB usually
  does **not** trigger the fallback; the service starts on Postgres and RPCs fail with `Internal`
  until the DB is up. The in-memory mode only happens on an unparseable `DATABASE_URL`, and then loses
  all data on restart.
- No graceful drain beyond `GracefulStop`; `SHUTDOWN_GRACE_SECONDS` is not used.

## Known gaps

- **Trusts `x-principal-id` from any caller that can reach the port.** There is no mTLS or service
  identity (ADR-0010 interim: network isolation only). The service does not verify that the caller is
  the gateway.
- No scope gates (`RequireScopes`); authorization is "authenticated and own data" only.
- `ListReferralRewards` `page.total` is the page length, not the ledger size.
- `HasRedeemed` exists in the repository but is unused; dedup relies on the `referrals.referee_id`
  unique constraint.
- Reward is a fixed constant (`RewardOnReferral = 10000`) in `internal/service/referral.go`, and the
  proto comment example reason `invitee_first_order` differs from the reason actually written
  (`invitee_joined`).
- Redeem is not transactional: the referral insert and reward insert are separate statements, so a
  failure in between leaves a referral without a reward.
- Most `.env.example` variables are inert (see Configuration).

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md) (service table row for team-referral).
- ADRs (`../platform-core/docs/ADR/`): `0001-proto-distribution.md`, `0003-auth-model.md`,
  `0006-rs256-jwks-auth.md`, `0010-service-zero-trust.md`.
