# team-referral

Go gRPC service for the **referral program** bounded context: per-user referral codes, redemption of
someone else's code (each user can be referred at most once), and an inviter reward ledger in xu.
Invitees are attributed asynchronously from `identity.events`; the inviter reward is `PENDING` when the
invitee's first order completes and is released by a sweeper after that order's return window ends with no
return. It owns the `referral_db` Postgres database; no other service writes these tables. Rewards are
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
| `GetMyReferral` | Caller's code (minted on first read), `invited_count`, `pending_xu`, `released_xu` and `rewards_total` (equal to `released_xu`), all xu. | `Unauthenticated` |
| `RedeemReferral` | Caller redeems `code` owned by another user: attributes the caller as referee (status `ATTRIBUTED`, `ReferralAttributed` in the same transaction) and credits nothing. Only an account younger than `REFERRAL_REDEEM_MAX_ACCOUNT_AGE` with a stored registration and no completed order may redeem. | `InvalidArgument` empty code; `NotFound` unknown code; `FailedPrecondition` (Vietnamese message) own code, caller already referred, ineligible account or code owner at the lifetime cap; `Unauthenticated` |
| `ListReferralRewards` | Caller's own `PENDING` and `RELEASED` rewards (`VOID` omitted) with `status`, `amount` in xu and `release_at`, newest first. Cursor is a decimal row offset; default page size 20, max 100. `page.total` is the length of the returned page, not the full count. | `Unauthenticated` |

Consumes: no upstream RPCs. Postgres and Kafka.

## Events

Consumed (envelope `platform.events.v1.EventEnvelope`, durable consumer groups, idempotent through
`referral_processed_events`, malformed payloads go to the dead-letter topic):

| Topic | Envelope type | Effect |
|---|---|---|
| `identity.events` | `platform.identity.v1.UserRegistered` | Stores the registration time; a non-empty `referral_code` creates an `ATTRIBUTED` referral or a rejection. DLQ `referral.identity.dlq`. |
| `order.events` | `platform.order.v1.OrderCompleted` | Stores the buyer's completed-order fact; the first completed order of an `ATTRIBUTED` referral creates a `PENDING` reward with `release_at = return_window_ends_at`, or marks the referral `CAPPED`. DLQ `referral.order.dlq`. |
| `order.events` | `platform.order.v1.ReturnRequested` | Voids a `PENDING` reward of that order and returns the referral to `ATTRIBUTED`. |

Produced through the `referral_outbox_events` outbox relayer on `referral.events`, keyed by referee id,
with a stable event id:

| Envelope type | When |
|---|---|
| `platform.referral.v1.ReferralAttributed` | A referral row is created (signup or manual redeem). |
| `platform.referral.v1.ReferralRejected` | A signup code is unknown, the user's own, or its owner is at the lifetime cap. |
| `platform.referral.v1.ReferralRewardReleased` | The sweeper releases a due reward under the daily and lifetime caps. |

A sweeper runs every `REFERRAL_RELEASE_SWEEP_INTERVAL`, releases due `PENDING` rewards oldest first
(daily cap counted per calendar day in `REFERRAL_CAP_TIMEZONE`) and purges processed-event rows older than 7 days.

## Data

Database `referral_db` (role `referral_svc`), migrations `migrations/001_create_referrals.up.sql` and `migrations/002_referral_attribution.up.sql`:

| Table | Purpose |
|---|---|
| `referral_codes` | One code per user (`user_id` PK, `code` unique). |
| `referrals` | `referrer_id`, `referee_id` (unique, so a user is referred once), `code`, `status` (`ATTRIBUTED`, `QUALIFIED`, `REWARDED`, `CAPPED`), `attributed_at`; index on `referrer_id`. |
| `referral_rewards` | Reward ledger: `id`, `user_id`, `amount` (BIGINT xu), `reason`, `status` (`PENDING`, `RELEASED`, `VOID`), `release_at`, `released_at`, `order_id` (unique when set), `referee_id`; indexes `(user_id, created_at DESC)`, `(status, release_at)`, `(user_id, released_at)`. |
| `referral_accounts` | `user_id` PK, `registered_at` from `UserRegistered`. |
| `referral_buyer_orders` | `buyer_id` PK, `first_completed_at` from `OrderCompleted`. |
| `referral_processed_events` | `event_id` PK, `processed_at`; consumer idempotency, purged after 7 days. |
| `referral_outbox_events` | go-sdk outbox rows published to `referral.events`. |

Migrations are applied by the `team-referral-migrate` job in the root `docker-compose.services.yaml`
(golang-migrate `up` against the shared `postgres` container). The service itself never migrates.

## Configuration

Settings are loaded by `internal/config` through go-sdk `pkg/config` (go-sdk at a tagged release, currently `v1.0.3`). `.env.example` lists exactly the settings; `make check-env` verifies it.

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
| `KAFKA_BROKERS` | `localhost:9092` | Broker list; required in staging and production. |
| `OUTBOX_POLL_INTERVAL` | `1s` | Outbox relayer poll interval. |
| `OUTBOX_BATCH_SIZE` | `100` | Outbox rows per relay pass. |
| `OUTBOX_CLAIM_LOCK` | `60s` | Outbox row lease. |
| `REFERRAL_REWARD_XU` | `10000` | Reward per qualified invitee, in xu; must be positive. |
| `REFERRAL_DAILY_CAP` | `5` | Rewards released per inviter per calendar day; must be positive. |
| `REFERRAL_LIFETIME_CAP` | `50` | Rewards per inviter over a lifetime; must be positive. |
| `REFERRAL_REDEEM_MAX_ACCOUNT_AGE` | `168h` | Oldest account that may redeem manually. |
| `REFERRAL_RELEASE_SWEEP_INTERVAL` | `1m` | Sweeper interval. |
| `REFERRAL_CAP_TIMEZONE` | `Asia/Ho_Chi_Minh` | Timezone of the daily cap day. |
| `IDENTITY_EVENTS_TOPIC` | `identity.events` | Consumed topic for `UserRegistered`. |
| `ORDER_EVENTS_TOPIC` | `order.events` | Consumed topic for `OrderCompleted` and `ReturnRequested`. |
| `REFERRAL_EVENTS_TOPIC` | `referral.events` | Produced topic. |
| `REFERRAL_IDENTITY_CONSUMER_GROUP` | `team-referral-identity` | Consumer group for `identity.events`. |
| `REFERRAL_ORDER_CONSUMER_GROUP` | `team-referral-order` | Consumer group for `order.events`. |
| `REFERRAL_IDENTITY_DLQ_TOPIC` | `referral.identity.dlq` | Dead-letter topic for `identity.events`. |
| `REFERRAL_ORDER_DLQ_TOPIC` | `referral.order.dlq` | Dead-letter topic for `order.events`. |

## Run locally

Root stack (from the repo root):

```
docker compose up -d --build            # whole stack incl. postgres, team-referral-migrate, team-referral
docker compose up -d --build team-referral   # this service + its migrate job and Postgres
```

`team-referral` has no published host port in compose; reach it via `team-gateway`. There is no
`--profile jobs` component for this service.

Standalone: `make run` (`go run ./cmd/server`) needs generated code (see Gotchas) and a Postgres
reachable at `DATABASE_URL` with the migrations applied and a Kafka broker at `KAFKA_BROKERS`.

## Build, test, lint

Layout follows ADR-0016 on go-sdk at a tagged release (`vX.Y.Z`, no `replace`; currently `v1.0.3`): `cmd/server` (entry), `internal/app` (wiring), `internal/config` (settings), `internal/rpc` (gRPC handlers), `internal/referral` (domain logic and sweeper), `internal/store` (Postgres store, `internal/store/fake` for tests), `internal/consumer` (Kafka consumers), `internal/events` (outbox envelopes).

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

## Links

- Root rules: [`../AGENTS.md`](../AGENTS.md) (service table row for team-referral).
- ADRs (`../platform-core/docs/ADR/`): `0001-proto-distribution.md`, `0003-auth-model.md`,
  `0006-rs256-jwks-auth.md`, `0010-service-zero-trust.md`.
