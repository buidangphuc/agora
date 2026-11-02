## Why

`team-notification` hand-rolls the platform layer that go-sdk v1.0.2 now provides: config and `.env.example` check, slog setup, health, shutdown, `pgxpool`, the franz-go consume loop with DLQ and a hand-written dedupe ledger, the service-principal gRPC metadata, and the `x-principal-*` interceptor. It also carries three behaviours the user ruled out on 2026-10-22 ("importing a dependency means using it"):

- `main.go` logs "cannot connect to postgres-notification, running in mock mode" and keeps serving with a nil pool. The inbox RPCs then fail on a nil pool and the alert and preferences RPCs return `Unimplemented` ("alert subscriptions not enabled"). That is a mock service, not mock data.
- `KAFKA_ENABLED` (default false) switches all three consumers off; a failed Kafka client init disables one consumer and the process stays "healthy".
- `ListingConsumer` and the chat and order consumers default to `InMemoryDeduper`, `InMemoryPriceStateStore`, `InMemoryStockStateStore` and `InMemoryLowStockStore` when nothing durable is injected. A restart then re-notifies and loses the price baseline.

Health is a constant SERVING, a panic in a handler kills the process, and the consumer logs the raw Kafka record key (the user id) in errors.

Source: `plans/go-sdk-migration/PLAN.md` (Decisions, Step 2 recipe), ADR-0016, SDK `v1.0.2` (tag exists, module `github.com/buidangphuc/agora/platform-core/packages/go-sdk`). Main's behaviour wins over the old migration on `origin/feat/go-sdk-v1-e2e` (reference only).

initiative: go-sdk-migration

## What Changes

- **Dependency**: `go.mod` requires `github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.2`. No `replace`, no `third_party/`. The service `Makefile` includes `../platform-core/make/go-service.mk` and sets `LINT_CONFIG` to the SDK `.golangci.yml`; `golangci-lint` is pinned as a `tool` directive like the SDK `go.mod`. No copies of either file, no doc comments added (revive doc-comment rules are off).
- **Layout (ADR-0016)**: `cmd/server/main.go` -> `internal/app.Run(ctx, settings)`; `internal/config`; `internal/rpc` (handlers, error table); domain packages `internal/inbox`, `internal/alerts`, `internal/prefs`, `internal/names`; `internal/store` (pgx queries, embedded `migrations/`); `internal/events` (the three Kafka consumers). `cmd/server` keeps its name so the Dockerfile entrypoint and compose are unchanged. `internal/{bootstrap,grpcserver,interceptor,handler,repository,service,consumer,upstream}` are removed.
- **Config (SDK groups)**: `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `LogKeyFP` plus the service's own fields (`LOW_STOCK_THRESHOLD`, topic, DLQ and group names, `UPSTREAM_DOMAIN_ADDR`, `UPSTREAM_IDENTITY_ADDR`). `ENV` and `GRPC_PORT` become required; `DATABASE_URL` and `KAFKA_BROKERS` are required (`KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` default only for a local ENV; `UPSTREAM_*_ADDR` are required in every ENV).
- **BREAKING (accepted) deletions**: `DATABASE_ENABLED`, `KAFKA_ENABLED`, the nil-pool "mock mode", `Unimplemented` for alert and prefs RPCs, the `InMemory*` defaults in production wiring, the "sender name lookup disabled" path, and the per-consumer "disabled on init failure" path. The in-memory repositories and stores move to `_test.go` fakes.
- **BREAKING (accepted), reflection**: the current code registers gRPC reflection unconditionally and the current `.env.example` sets `GRPC_REFLECTION_ENABLED=true` (README: "reflection is always on"). It becomes off by default and is refused in a strict ENV. A repo-wide search found no dev script or runbook that runs `grpcurl` reflection against this service; local users set `GRPC_REFLECTION_ENABLED=true` explicitly.
- **BREAKING (accepted), other removals**: the unused field `config.KafkaBroker` (key `KAFKA_BROKER`, replaced by the SDK `KAFKA_BROKERS`), the `localhost` defaults of `DATABASE_URL` and `KAFKA_BROKERS` (local ENV only keeps an SDK default for `KAFKA_BROKERS`), and the "consumer stopped while the service is running" state (a consumer error now stops the process). `UPSTREAM_DOMAIN_ADDR` and `UPSTREAM_IDENTITY_ADDR` lose their `localhost` defaults and are required in every ENV (no fallback mode).
- **Kept flags**: `OTEL_ENABLED`; `GRPC_REFLECTION_ENABLED` (default false, refused in a strict ENV; see the reflection item above).
- **Mock data**: this service has no mock switch to port. The "mock mode" is deleted, not renamed; no `MOCK_DATA` adapter is introduced.
- **Health**: one `healthx.Health` with the checker `postgres` only (Kafka is not a readiness check); readiness is the overall status, liveness is the `liveness` service (set `probes.livenessService: liveness` in gitops); `Drain` first on shutdown.
- **gRPC**: `grpcx.NewServer` chain (request id, recovery, logging, auth); `auth.RequireUser` for the per-user RPCs; `grpcx.Status` with an error table (23505 -> `AlreadyExists`, `Canceled`, `DeadlineExceeded` come from the SDK table); internal errors are `internal error`.
- **Kafka**: three `consume.Consumer` loops (listing, chat, order) over `kafkax.NewConsumerClient`, DLQ through `kafkax.Producer`, `consume.Permanent` for poison records, key fingerprints (`LOG_KEY_FP_SECRET`) in logs. The `processed_events` ledger becomes `consume.Dedupe.Seen` inside the same transaction as the notification write.
- **Upstream**: `upstream.NewClient(addr, upstream.AsService("service-team-notification", "listing.read", "identity.read"))`; the 2 s lookup bound uses `upstream.Bounded`.
- **Pagination**: `ListNotifications` keeps `page_number` / `page_size` (1-based, default 20); `pagination.Clamp` caps `page_size` at 100 (accepted new bound; the old code had none).
- **Migrations**: the five existing files move to `internal/store/migrations` unchanged (same versions, same `schema_migrations`); the binary gets a `migrate up` subcommand (SDK `pkg/migrate`, embedded migrations); the service never migrates at boot. Compose keeps `team-notification-migrate` running the service image with `migrate up`; gitops gets an Argo PreSync migrate Job (Deploy track, change `deploy-go-sdk-v1`).
- No requirement in `openspec/specs/*` names a removed flag or fallback, so no existing requirement is MODIFIED; one capability `notification-service-runtime` is ADDED. The behaviour in `buyer-notifications` and `notification-order-work` stays and its tests stay green (see D9).

## Impact

Code: `team-notification/**` (all of it). Specs: new `notification-service-runtime`. Deploy (separate change `deploy-go-sdk-v1`): `docker-compose.services.yaml`, `platform-gitops`. No proto change, no new RPC, no new event, no migration change.

## NFR

- Availability: liveness `Check` answers from memory (p99 < 5 ms) and stays SERVING while Postgres or Kafka is down; readiness flips to NOT_SERVING within 10 s (2 x `healthx.DefaultInterval` of 5 s) of a failing Postgres check; a Kafka outage does not change readiness.
- Boot: an unreachable database fails boot in every `ENV` (a failed migration fails `migrate up`) (no local exception; the 5 s `postgres.Open` ping bound is inherited from the SDK and tested there, the boot scenario asserts 10 s); an unset required variable fails `Load` before anything is opened.
- Shutdown: health drains first, then components stop within `SHUTDOWN_GRACE` (default 10 s); a consumer finishes or abandons its current record without committing past it.
- Latency: `ListNotifications`, `GetUnreadCount`, `MarkAsRead` stay single-statement queries (p95 < 100 ms at 10 000 rows per user, unchanged); a chat notification still appears within 30 s of the message (scenario `A chat message becomes a notification within 30 seconds`; the 10,000-row p95 and the liveness p99 have their own scenarios).
- Delivery: at-least-once from Kafka, exactly-once effect per `(consumer, event_id)`; a handler error retries up to 5 times (SDK default) before the DLQ; the offset never advances past an unhandled, un-dead-lettered record.
- Tests: Docker-backed tests must run; a skip is a gate failure (`GO_SDK_REQUIRE_DOCKER=1`).
- Limits: `page_size` <= 100; chat body truncated as today; sender-name lookup bounded to 2 s.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `notifications.user_id`, `alert_subscriptions.user_id`, `notification_prefs.user_id` | pseudonymous user id | scope every row to its owner | legitimate interest (operating the inbox the user asked for) | unchanged by this change; rows live until user erasure |
| `notifications.title`, `body`, `link_url` | personal content (chat message start, shipment tracking number, sender display name) | show the user their own notifications | legitimate interest | unchanged by this change |
| Kafka record key (user, thread or order id) | pseudonymous id | partitioning; appears in error logs | legitimate interest | logs: platform log retention; the raw key is never logged, only the keyed fingerprint `key_fp` |
| `LOG_KEY_FP_SECRET` | secret | keys the fingerprint HMAC | n/a | Vault, rotated by ops; never logged |
| Service principal `service-team-notification` | service identity | upstream name lookups (read scopes only) | n/a | n/a |

No DPO sign-off is required (user rule, 2026-10-22): the basis is legitimate interest and logs follow platform log retention. This change adds no new personal field and removes one leak (raw keys in consumer logs).

## Threat model

- Assets: each user's notifications, alert subscriptions and preferences; the chat text copied into a notification body; the DLQ topics (poison records can hold payloads).
- Abuse 1: user A reads or marks user B's notifications. Control: every per-user RPC resolves the owner from the forwarded principal (`auth.RequireUser`) and scopes SQL by it; a foreign id is `NotFound`; anonymous is `Unauthenticated`; a service principal is `PermissionDenied`.
- Abuse 2: a mislabelled deployment (missing `DATABASE_URL`, Kafka unreachable) serves a hollow service that looks healthy. Control: no flag and no fallback; a missing setting fails `Load`, an unreachable database fails boot, a hollow service cannot boot; Kafka outages are not a readiness signal (consumers retry).
- Abuse 3: redelivered or replayed events spam a user. Control: the ledger insert and the notification insert share one transaction, so a redelivery after a crash inserts neither twice and a failed write rolls the ledger row back.
- Abuse 4: reflection exposes the RPC surface. Control: off by default, `Load` refuses `GRPC_REFLECTION_ENABLED=true` in a strict ENV.
- Abuse 5: a poison record stalls a partition or leaks ids to logs. Control: `consume.Permanent` sends it to the DLQ and commits; logs carry `key_fp` only.
- Abuse 6: the upstream lookup acts with write power. Control: the service principal carries only `listing.read` and `identity.read`.
- Authz rule: every notification RPC requires a user principal and operates only on that user's rows; the consumers write through the store with no caller principal.
