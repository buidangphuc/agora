# team-chat

Go gRPC service for the **Chat** bounded context: 1:1 buyer-to-seller conversation threads tied
to a listing, messages (text, listing card, quick reply), per-party unread counters,
participant-scoped message search, and seller quick-reply templates. It also produces the
`chat.events` Kafka stream (one `ChatMessage` event per stored message) through a transactional
outbox. It owns the `chat_db` database and serves `platform.chat.v1.ChatService` on `:50057`.

Status: deployed service, wired in the root `docker-compose.services.yaml`.

## 1. Contract

Proto: `proto/platform/chat/v1/chat.proto` (vendored from `platform-core/packages/proto`).
Every RPC calls `interceptor.RequirePrincipal`, which rejects a missing, empty or
`anonymous`/ANONYMOUS principal with `Unauthenticated`. The principal comes from the gateway's
`x-principal-id`, `x-principal-type` and `x-principal-scopes` metadata. The only server
interceptor is `AuthUnaryInterceptor`, which parses that metadata; no scopes are checked.

| RPC | Authorization rule |
|---|---|
| `GetOrCreateThread` | Any authenticated user; the caller becomes `buyer_id`. `seller_id` is required and must differ from the caller (`InvalidArgument`). Unique on `(buyer_id, seller_id, listing_id)`. The thread title is the hard-coded string "Sản phẩm" with an empty image. |
| `ListThreads` | Any authenticated user; returns only threads where the caller is buyer or seller. |
| `GetThreadMessages` | Caller must be the thread's buyer or seller (`PermissionDenied` otherwise, `NotFound` if no thread). |
| `SendMessage` | Same participant check. Persists the message, updates the thread and enqueues the event. Rich kinds (`MESSAGE_TYPE_LISTING_CARD`, `MESSAGE_TYPE_QUICK_REPLY`) use this same RPC. |
| `MarkThreadRead` | Same participant check. Resets only the caller's unread counter. |
| `SearchMessages` | Any authenticated user; `ILIKE` search limited to threads the caller participates in. A blank query returns an empty result. |
| `ListQuickReplies` | Any authenticated user; `seller_id` may be any seller, and an empty one defaults to the caller. Returns four built-in Vietnamese defaults if the seller has no rows. |
| `StreamChat` | Not implemented here (`Unimplemented` from the embedded server). The gateway serves it by forwarding to `team-ai`. |

Paging: the service clamps `page_size` to 1..100 (default 20 for `ListThreads`, 50 for messages
and search); `next_cursor` is the next page number as a string.

Consumes: nothing. There are no upstream RPCs or data dependencies (see Known gaps for
`UPSTREAM_LISTING_ADDR`).

## 2. Events

| Direction | Topic | Type | Key | Notes |
|---|---|---|---|---|
| Produces | `KAFKA_CHAT_TOPIC` (default `chat.events`) | `platform.chat.v1.ChatMessage` inside `platform.events.v1.EventEnvelope` | `thread_id` | One per stored message. The payload also carries `recipient_id` (the other participant) and `seller_id`, populated only on events. |
| Consumes | none | | | |

How it is published (ADR-0002, outbox pattern as in ADR-0009):

1. `SaveMessage` inserts the message, updates the thread and inserts a `chat_outbox_events`
   row in **one transaction** (`events.BuildMessageOutboxRow`). The RPC never touches Kafka.
2. `events.Relayer` polls every `OUTBOX_POLL_INTERVAL`, claims pending rows with
   `FOR UPDATE SKIP LOCKED` under an `OUTBOX_CLAIM_LOCK_SECONDS` lease, produces the stored
   envelope bytes to Kafka (producer linger 0, synchronous ack) and marks them `published`.
3. On failure the row is retried with exponential backoff (1s doubling, capped at 5m). After
   `OUTBOX_MAX_ATTEMPTS` it is parked with `status='failed'` and never claimed again until an
   operator resets it, for example
   `UPDATE chat_outbox_events SET status='pending', attempts=0, available_at=now(), locked_until=NULL WHERE status='failed'`.

Delivery is **at-least-once**. `event_id` is deterministic (`events.ChatMessageEventID`, a
UUIDv5 of the message id), so consumers dedupe on it. Per-thread order holds because the key
is `thread_id` and claimed rows come back oldest-first.

The relayer starts only when Postgres is on, a Kafka producer exists (`KAFKA_ENABLED=true`
and the brokers dialed) and `OUTBOX_ENABLED=true`. Otherwise `main.go` logs a warning, rows
keep being written and they pile up unpublished.

Known consumer: `team-notification` (group `team-notification.chat`, dead-letter topic
`chat.events.dlq`), which creates a CHAT notification for the recipient.

## 3. Data

Database `chat_db`, user `chat_svc` (the shared `postgres` instance in compose). Migrations are
in `migrations/` (each with a `.down.sql`) and are **not** applied by the service binary.

| Table | Migration | Purpose |
|---|---|---|
| `chat_threads` | 0001 | Thread with listing context, last message, `unread_count_buyer`, `unread_count_seller`. Unique `uq_chat_threads_parties`. |
| `chat_messages` | 0001, 0002 | Messages (FK to thread, cascade delete). 0002 adds `message_type`, `listing_id`, `payload`. |
| `quick_replies` | 0002 | Seller canned replies, ordered by `sort_order`. Nothing in this service writes it. |
| `chat_outbox_events` | 0003 | Outbox: `event_id` PK, `aggregate_id` (thread id), `payload` (envelope bytes), `status` (`pending`/`published`/`failed`), `attempts`, `available_at`, `locked_until`, `error`. |

Apply them with the `team-chat-migrate` compose service (golang-migrate `up`), or run
golang-migrate yourself against `chat_db`. The Docker image copies `migrations/` but the
service does not run them.

## 4. Configuration

Read by `internal/config/config.go`; `.env.example` mirrors it. An unset or empty variable
falls back to the default. There is no automated drift gate between `.env.example` and the code.

| Variable | Default | Meaning |
|---|---|---|
| `SERVER_HOST` | `0.0.0.0` | gRPC bind host |
| `SERVER_PORT` | `50057` | gRPC port |
| `SERVER_SHUTDOWN_GRACE_SECONDS` | `5.0` | Graceful-stop timeout |
| `SERVER_REFLECTION_ENABLED` | `true` | gRPC reflection |
| `RUNTIME_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `RUNTIME_LOG_JSON` | `false` | JSON log output |
| `POSTGRES_HOST` | `postgres-chat` | Compose overrides it to `postgres` |
| `POSTGRES_PORT` | `5432` | |
| `POSTGRES_DB` | `chat_db` | |
| `POSTGRES_USER` | `chat_svc` | |
| `POSTGRES_PASSWORD` | `chat_pass` | |
| `POSTGRES_SSLMODE` | `disable` | |
| `KAFKA_ENABLED` | `false` | Enables the producer and with it the relayer |
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated seeds. `.env.example` uses `localhost:19092` (redpanda host port); compose uses `redpanda:9092`. |
| `KAFKA_CHAT_TOPIC` | `chat.events` | |
| `OUTBOX_ENABLED` | `true` | Run the relayer |
| `OUTBOX_POLL_INTERVAL` | `1s` | Go duration; invalid or non-positive falls back to 1s |
| `OUTBOX_BATCH_SIZE` | `100` | Rows claimed per sweep |
| `OUTBOX_CLAIM_LOCK_SECONDS` | `60` | Lease on claimed rows |
| `OUTBOX_MAX_ATTEMPTS` | `10` | Attempts before a row is parked `failed` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Tracing is on only when set |
| `OTEL_SERVICE_NAME` | `team-chat` | |
| `UPSTREAM_LISTING_ADDR` | `localhost:50051` | Parsed but unused (see Known gaps) |

The database is always enabled (pool max 20); startup fails if Postgres is unreachable. If
Kafka is enabled but the dial fails, the service only logs a warning and runs without a relayer.

## 5. Run locally

Root compose, from the repo root. `team-chat` depends on `team-chat-migrate` and
`redpanda-init` (which creates `chat.events` and `chat.events.dlq`), so the shared `postgres`
and `redpanda` services must be available from the compose files:

```bash
docker compose -f docker-compose.services.yaml up -d --build team-chat
```

Compose sets `KAFKA_ENABLED=true`, `KAFKA_BROKERS=redpanda:9092`, the `OUTBOX_*` values and
`POSTGRES_HOST=postgres`. It also sets `DATABASE_ENABLED`, `DATABASE_URL`, `GRPC_PORT` and `ENV`,
which the code does not read.

Standalone: apply the migrations to a reachable `chat_db`, copy `.env.example` to `.env` and
export it (the binary does not load `.env` itself), then `go run ./cmd/server`. Go and buf are
not installed on the host per the root `AGENTS.md`, so use a Go 1.22 container.

## 6. Build, test and lint

There is no Makefile and no repo-local CI config here.

| Task | Command |
|---|---|
| Build | `go build ./cmd/server` (or `docker build .`, Go 1.22 alpine) |
| Test | `go test -race ./...` |
| Postgres tests | `internal/repository/*_pg_test.go` skip unless `TEST_DATABASE_URL` points at a disposable Postgres |
| Regenerate stubs | `buf generate` (`buf.gen.yaml`, input `proto/`, Go package prefix `github.com/buidangphuc/team-chat/generated`) |

Other tests use in-memory repositories (`InMemoryChatRepository`, `InMemoryOutboxRepository`)
and an ephemeral gRPC server. `coverage.out` feeds the root `sonar-project.properties`. No
lint command is defined here.

## 7. Spec and verification

- `FEATURES.yaml` at the repo root declares `chat.buyer-seller-thread` and `chat.thread-list`,
  both `automated`, covered by `chat/chat_thread.feature` and `buyer/consumer_pages.feature`
  in `platform-e2e`.
- Gates: `make -C platform-e2e features-check` (manifest and coverage) and
  `make -C platform-e2e spec-check CHANGE=<id>` (every scenario of an OpenSpec change has a
  green e2e).
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC and
  `AGENTS.md` section 9b. The notification side of `chat.events` is specified in
  `openspec/changes/notify-chat-and-shipment`.

## 8. Gotchas

- `generated/` is gitignored output of `buf generate`; regenerate it after any proto change
  and never hand-edit it.
- `proto/` is vendored from `platform-core/packages/proto`. Never edit it here; change it in
  platform-core (non-breaking) and re-vendor.
- `recipient_id` and `seller_id` exist only on the event payload; RPC responses leave them empty.
- No relayer means no events. With `KAFKA_ENABLED=false` (the default) or `OUTBOX_ENABLED=false`,
  `chat_outbox_events` accumulates `pending` rows; enabling Kafka later publishes the backlog.
- Search is `ILIKE '%query%'` with no supporting index.
- Authorization is only the handler and service participant checks; there are no scope checks
  and no stream interceptor.

## 9. Known gaps

- `UPSTREAM_LISTING_ADDR` is parsed and used nowhere. The thread's listing title is the
  hard-coded "Sản phẩm" and the image is empty; no listing lookup happens.
- Nothing in this repo writes `quick_replies`, so `ListQuickReplies` returns the built-in
  defaults unless rows are inserted by hand.
- `ListQuickReplies` lets any authenticated user read any seller's templates (intended so
  buyers can fetch them). `SendMessage` does not validate `payload` or `listing_id` for rich
  message types.
- Live chat push is not wired end to end. `team-gateway` has an SSE broker with a
  `chat:{thread}` room, but it does not consume `chat.events` and nothing publishes into that
  room; only `team-notification` consumes the topic.
- Parked (`failed`) outbox rows have no tooling or alert; recovery is manual SQL.
- `StreamChat` is an unimplemented seed RPC in this service.

## 10. Links

- Root `AGENTS.md`: the rules (database per service, contract as source of truth, Kafka for events).
- `platform-core/docs/ADR/0001-proto-distribution.md`, `0002-async-broker.md`,
  `0003-auth-model.md`, `0009-payment-order-event-integration.md` (outbox pattern).
