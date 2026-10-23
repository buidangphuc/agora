## Why

team-chat still hand-rolls config, logging, tracing, health, shutdown, the Postgres pool, the Kafka producer and the outbox relayer, and carries dependency enable flags and runtime fallbacks that the user ruled out on 2026-10-22 ("if a service imports a dependency, it uses it"). The SDK `go-sdk@v1.0.2` covers all of it. This change ports team-chat onto the tagged SDK on current main (which includes C4 q6 disputes and q11 checkout), keeping every current behaviour, the way sharing and referral are being ported.

Source: `plans/go-sdk-migration/PLAN.md` (Decisions, Per-service recipe, Kept flags, Shared lint/make files). The old migration on `origin/feat/go-sdk-v1-e2e` is a reference only; main wins.

Current gaps in team-chat that this change closes:
- `KAFKA_ENABLED` (default false), `OUTBOX_ENABLED` and `Database.Enabled` gate the producer, the relayer and the pool; with Kafka off the service logs "rows are recorded but not published" and runs on, so chat notifications silently stop.
- `OUTBOX_MAX_ATTEMPTS` parks a row after 10 failed relays, which loses the notification for a Kafka outage longer than the backoff schedule; the spec "Chat message events are written through an outbox" says a stored message SHALL eventually produce its event.
- gRPC reflection is on by default (`SERVER_REFLECTION_ENABLED=true`) with no strict-ENV refusal, and the service has no `ENV` at all.
- Liveness and readiness are one status; `InMemoryChatRepository` and `InMemoryOutboxRepository` live in the production packages.
- The Kafka producer connects once at boot and a failure only logs a warning.

initiative: go-sdk-migration

## What Changes

- Module: `github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.2` as a `require` (no `replace`, no `third_party`); `Makefile` includes `../platform-core/make/go-service.mk` and lints with `../platform-core/packages/go-sdk/.golangci.yml`; golangci-lint is pinned as a `tool` in `go.mod` like the SDK; no copies of either file and no doc comments added.
- ADR-0016 layout: `cmd/server/main.go` calls `internal/app.Run(ctx, settings)`; `internal/config` (SDK groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox`), `internal/rpc` (handler, `Register`, error mapping), `internal/chat` (domain service), `internal/store` (Postgres repository and outbox table name), `internal/events` (outbox row builder). `internal/bootstrap`, `internal/grpcserver`, `internal/interceptor`, `internal/observability` are deleted.
- **BREAKING (env)** The service reads the SDK variable names: `ENV` (required), `GRPC_HOST`/`GRPC_PORT` (required), `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY`, `LOG_LEVEL`, `LOG_JSON`, `DATABASE_URL` (required), `DB_MAX_CONNS`, `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `KAFKA_BROKERS`, `KAFKA_CHAT_TOPIC`, `OUTBOX_POLL_INTERVAL`, `OUTBOX_BATCH_SIZE`, `OUTBOX_CLAIM_LOCK`, `GRPC_REFLECTION_ENABLED`. Removed: `SERVER_*`, `RUNTIME_LOG_*`, `POSTGRES_*`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `OUTBOX_CLAIM_LOCK_SECONDS`, `OUTBOX_MAX_ATTEMPTS`, `UPSTREAM_LISTING_ADDR` (read by `config` but used by no code).
- **BREAKING (behaviour, accepted)** Postgres, the Kafka producer and the outbox relayer are always on. An unreachable database or an invalid Kafka client fails boot in every `ENV`. The relayer has no attempt limit: a transient failure retries with capped backoff and only a permanent record error parks a row.
- `chat_outbox_events` moves to the SDK v1 outbox schema with migrations `0004`..`0011` generated from `outbox.UpgradeSQL("chat_outbox_events")` and `0012` that requeues rows parked as `failed` (columns `topic`, `traceparent`, `seq`, status and aggregate indexes; the old claim index dropped). Per-thread ordering uses `seq`, so a retried message cannot move behind a newer message of its thread.
- Migrations: the binary gets a `migrate up` subcommand (`internal/app.Migrate`, SDK `migrate.Up`, embedded `migrations.Files`); the service never migrates at boot. Compose keeps `team-chat-migrate` running the service image with `migrate up`; gitops gets an Argo PreSync migrate Job (both in deploy-go-sdk-v1, Deploy track).
- Outbox retention: the service runs `outbox.Store.Purge(ctx, 7*24h)` on a schedule (every hour), so published rows older than 7 days are deleted; the first rollout scales old pods to 0 before new pods start (Recreate once, never two relayers).
- Health: `postgres.Checker` is the only readiness check; Kafka is not a readiness check in team-chat or in any other service because the outbox absorbs a broker outage. The `liveness` health service is independent of dependencies.
- Reflection: off unless `GRPC_REFLECTION_ENABLED=true`, refused by `Load` in a strict `ENV`.
- Auth, request id, recovery, logging and error mapping come from `auth` and `grpcx`; the caller-visible codes and messages of every RPC are unchanged except the three items in design D6.
- ADDED capability `chat-service` documents the existing RPC contract, authorization and configuration rules, which no spec describes today. MODIFY: none of the existing requirements describes a removed flag or fallback (checked: `buyer-notifications` "Chat message events are written through an outbox", `deploy-runtime` "gRPC services are health-probed"); the outbox requirement stays true and is exercised by the new scenarios.
- Compose and gitops edits are NOT in the code tasks; they are listed in tasks.md "Deploy track (done by change deploy-go-sdk-v1)".

## Impact

Code: `team-chat/**` only (go.mod, go.sum, Makefile, Dockerfile, README.md, `.env.example`, `cmd`, `internal`, `migrations`, `FEATURES.yaml`). Contracts: `chat.v1.ChatService` and `chat.events` (`platform.events.v1.EventEnvelope` carrying `platform.chat.v1.ChatMessage`, key = thread id, event_id deterministic per message) are unchanged; team-notification and team-gateway need no change. Dependencies: SDK tag `v1.0.2` must exist (it does).

## NFR

- Availability: readiness reflects Postgres only; a Kafka outage keeps `SendMessage` serving and delays events; liveness answers from memory and does not fail on a dependency outage.
- Latency: SDK interceptor chain adds no I/O; `SendMessage` still writes message, thread update and outbox row in one transaction with no added database round trip.
- Delivery: at-least-once to `chat.events`; per-thread order by `seq`; no attempt limit, so a stored message is never dropped by retries. Consumers dedupe on the deterministic `event_id`.
- Limits: page sizes unchanged (threads default 20, messages and search default 50, maximum 100 for both).
- Migration: `migrate up` finishes in under 5 s on 10,000 outbox rows; the outbox purge deletes in the background and never blocks `SendMessage`.
- Tests: Docker-backed (Postgres, Redpanda) tests must run; a skip is a gate failure.
- Boot: an unreachable database fails boot within the `postgres.Open` 5 s ping bound; shutdown drains health, waits `SHUTDOWN_DRAIN_DELAY`, stops gRPC, then the relayer, then closes the producer and the pool.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `chat_messages.content`, `payload` | personal (free text a user wrote, may hold anything) | deliver the buyer-seller conversation | contract (the user asks to send it) | unchanged by this change: stored with the thread; no new copy is made |
| `sender_id`, `buyer_id`, `seller_id`, `recipient_id`, `sender_name` | personal (user ids, display name) | thread membership, authorization, notification target | contract; notification target is legitimate interest | thread lifetime; the outbox row carries them in the envelope (see the next row) |
| Outbox `payload` (EventEnvelope with ChatMessage, principal) | personal, copy of the above | relay to `chat.events` | legitimate interest | published rows are deleted 7 days after publishing by the scheduled `Store.Purge` (before: kept forever); rows parked as `failed` by the old relayer are requeued by migration `0012` |
| Logs (request id, trace id, user id as principal id, method) | pseudonymous | operations and abuse analysis | legitimate interest | platform log retention |

Message `content`, `payload`, `sender_name` and the Kafka key (thread id) SHALL NOT be written to logs; error text from the driver SHALL NOT reach callers. No DPO sign-off is needed (user rule): basis legitimate interest, platform log retention.

## Threat model

- Assets: private conversations between a buyer and a seller; the `chat.events` stream (recipient and sender ids); the outbox table.
- Abuse 1, thread snooping: a user reads, searches, sends to or marks read a thread they are not part of. Control: every RPC requires a non-anonymous principal and checks buyer or seller membership; search is scoped to the caller's threads. Authz rule: the principal id (never a request field) is the acting user.
- Abuse 2, anonymous caller: the gateway's anonymous principal (id `anonymous`, type ANONYMOUS) must not pass. Control: `auth.RequirePrincipal` rejects it with `Unauthenticated`.
- Abuse 3, reflection left on: discloses the API surface. Control: default off and refused in staging and prod before any other validation.
- Abuse 4, data leak through errors or logs: SQL text, user ids or message text in a gRPC error or log. Control: `grpcx.Status` maps unknown errors to `Internal` "internal error" and logs the cause server-side only; handlers never log content.
- Abuse 5, silent loss of events: broker outage plus attempt limit parks a message event forever. Control: no attempt limit, outbox row in the message transaction, parked rows block only their own thread and are visible in `Stats`.
- Abuse 6, mock or fallback running in production: control: no in-memory runtime path exists in the binary; test fakes live in `_test.go` files only.
