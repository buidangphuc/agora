# team-chat

Go gRPC service for the **Chat** bounded context: 1:1 buyer-to-seller conversation threads tied
to a listing, messages (text, listing card, quick reply), per-party unread counters,
participant-scoped message search, and seller quick-reply templates. It also produces the
`chat.events` Kafka stream (one `ChatMessage` event per stored message) through a transactional
outbox. It owns the `chat_db` database and serves `platform.chat.v1.ChatService` on `:50057`.

Status: deployed service, wired in the root `docker-compose.services.yaml`.

Go 1.26, module `github.com/buidangphuc/team-chat`. The platform layer (config, logging,
tracing, auth, health, lifecycle, Postgres, the gRPC server chain, the outbox relayer and the Kafka
producer) is [go-sdk](../platform-core/packages/go-sdk/README.md) v0.2.1, vendored in
`third_party/go-sdk` (see `third_party/go-sdk/VERSION`) by `platform-core/tools/sync-go-sdk.sh`;
never edit it by hand. Layout follows
[ADR-0016](../platform-core/docs/ADR/0016-go-service-convention-and-sdk.md):

| Path | Role |
|---|---|
| `cmd/server/main.go` | Signal context, `config.Load`, `app.Run`, exit code. |
| `internal/app` | Composition root: logger, telemetry, health, database, publisher, server, outbox relayer. |
| `internal/config` | `Settings` (SDK groups) and the `.env.example` drift test. |
| `internal/rpc` | gRPC handlers; maps domain errors to statuses. |
| `internal/chat` | Domain: thread participation, message validation, paging bounds. |
| `internal/store` | Postgres and in-memory chat stores; writes the outbox row in the message transaction. |
| `internal/events` | Builds the `chat.events` envelope. |
| `migrations/` | SQL migrations plus `embed.go` (package `migrations`, for tests). |

## 1. Contract

Proto: `proto/platform/chat/v1/chat.proto` (vendored from `platform-core/packages/proto`).
Every RPC calls `auth.RequirePrincipal` (go-sdk), which rejects a missing, empty or
`anonymous` principal with `Unauthenticated`. The principal comes from the gateway's
`x-principal-id`, `x-principal-type` and `x-principal-scopes` metadata, parsed by the SDK auth
interceptor; no scopes are checked. A panic in a handler is recovered and returned as `Internal`,
and every call gets a request id and a log line. Health is SERVING while the database answers a
ping and NOT_SERVING when it does not (checked every 5s) and during shutdown drain. gRPC reflection is always on.

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
2. The SDK `outbox.Relayer` (a lifecycle component that stops after the gRPC server) polls every
   `OUTBOX_POLL_INTERVAL`, claims pending rows with `FOR UPDATE SKIP LOCKED` under an
   `OUTBOX_CLAIM_LOCK` lease, produces the stored envelope bytes to Kafka through
   `kafkax` (synchronous ack) and marks them `published`.
3. On failure the row is retried with capped exponential backoff, forever: there is no attempt
   limit. After `AlertAfter` (10) failed attempts the relayer logs at ERROR on every further
   attempt. Only an error that is permanent for the record (message too large, corrupt or
   invalid record) parks the row with `status='failed'`. A parked row blocks the later rows of
   its thread until `Store.RequeueParked` / `Store.Requeue` puts it back or it is deleted.

Delivery is **at-least-once**. `event_id` is deterministic (`events.ChatMessageEventID`, a
UUIDv5 of the message id), so consumers dedupe on it. Per-thread order holds because the key
is `thread_id` and rows of a thread are claimed in `seq` (creation) order; a row is not claimed while an older unpublished row of the same thread exists.

The relayer and the Kafka producer are always on. Creating the producer does not dial the
brokers, so an unreachable broker delays publishing (rows stay `pending` and are retried) but
does not stop boot.

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
| `chat_outbox_events` | 0003 | Outbox: `event_id` PK, `aggregate_id` (thread id), `payload` (envelope bytes), `status` (`pending`/`published`/`failed`), `attempts`, `available_at`, `locked_until`, `error`. 0004-0011 upgrade it to the go-sdk v1 shape (`topic`, `traceparent`, `seq`, v1 indexes). |

Apply them with the `team-chat-migrate` compose service (golang-migrate `up`), or run
golang-migrate yourself against `chat_db`. The Docker image copies `migrations/` but the
service does not run them.

### Deploying the go-sdk v1 outbox (migrations 0004-0011)

Migrations 0004-0011 are forward-only (their `.down.sql` files are no-ops) and implement
`outbox.UpgradeSQL` for `chat_outbox_events`. A v0 relayer ignores `seq`, so before applying them
on a database that already holds rows:

1. Stop the v0 `team-chat` relayers (scale the deployment to 0), or let them drain the pending rows.
2. Deal with rows a v0 relayer parked. v0 parked a row as `failed` after `OUTBOX_MAX_ATTEMPTS`
   attempts; v1 never sends a `failed` row but keeps it as the head of its thread, so it blocks
   every later message event of that thread. List them with
   `SELECT event_id, aggregate_id, attempts, error FROM chat_outbox_events WHERE status = 'failed'`,
   fix the cause, then `DELETE` the rows you will not send or requeue the rest with
   `UPDATE chat_outbox_events SET status='pending', attempts=0, available_at=now(), locked_until=NULL, error=NULL WHERE status='failed'`.
   After the upgrade `outbox.Store.RequeueParked` does the same for rows v1 parks later.
3. Purge published rows you do not need (`DELETE FROM chat_outbox_events WHERE status = 'published'`);
   every backfill batch sorts the rows still unnumbered, so fewer rows means a faster upgrade.
4. Apply the migrations, then deploy v1. 0005 numbers existing rows by `(created_at, event_id)`
   in batches of 10000, 0006 switches in one transaction (inserts wait on its table lock for a
   moment), 0007 makes `seq` NOT NULL, and 0008-0011 each hold one `CREATE/DROP INDEX
   CONCURRENTLY` statement in a file of their own, because it cannot run inside a transaction.
   If an index migration fails, drop the invalid index and rerun the migration.

## 4. Configuration

`internal/config` binds the SDK groups from env; `.env.example` lists every key and
`TestEnvExampleInSync` fails when the two drift. `GRPC_PORT` is required (no default; the
Dockerfile and compose set 50057). `ENV` is required too (no default). Any other empty variable falls back to its default. A malformed value
(for example a bad `OUTBOX_POLL_INTERVAL`) fails the load.

| Variable | Default | Meaning |
|---|---|---|
| `ENV` | required | `local`, `test`, `staging` or `prod`. Strict environments (`staging`, `prod`) also require `KAFKA_BROKERS`. |
| `LOG_LEVEL`, `LOG_JSON` | `info`, `true` | Logging. |
| `GRPC_HOST`, `GRPC_PORT` | `0.0.0.0`, required | Listen address. |
| `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY` | `10s`, `0s` | Shutdown drains health first, then stops within the grace period. |
| `DATABASE_URL`, `DB_MAX_CONNS` | required, `10` | Postgres. A missing `DATABASE_URL` fails at load. |
| `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME` | `false`, none, none | Telemetry, off by default. |
| `KAFKA_BROKERS` | `localhost:9092` in local/test, required otherwise | Comma-separated seeds. `.env.example` uses `localhost:19092` (redpanda host port); compose uses `redpanda:9092`. |
| `KAFKA_CHAT_TOPIC` | `chat.events` | Topic the relayer publishes to. |
| `OUTBOX_POLL_INTERVAL` | `1s` | Go duration. |
| `OUTBOX_BATCH_SIZE` | `100` | Rows claimed per sweep |
| `OUTBOX_CLAIM_LOCK` | `60s` | Lease on claimed rows (Go duration) |
| `UPSTREAM_LISTING_ADDR` | `localhost:50051` | Parsed but unused (see Known gaps) |

Postgres, Kafka and the outbox relayer are always on: there are no enable flags. The database
must be reachable at boot in every `ENV` (there is no in-memory fallback); an unreachable one
fails boot before any port opens.

## 5. Run locally

Root compose, from the repo root. `team-chat` depends on `team-chat-migrate` and
`redpanda-init` (which creates `chat.events` and `chat.events.dlq`), so the shared `postgres`
and `redpanda` services must be available from the compose files:

```bash
docker compose -f docker-compose.services.yaml up -d --build team-chat
```

Compose sets `DATABASE_URL`, `KAFKA_BROKERS=redpanda:9092`, `KAFKA_CHAT_TOPIC`, the `OUTBOX_*`
values, `GRPC_PORT` and `ENV`.

Standalone: apply the migrations to a reachable `chat_db`, copy `.env.example` to `.env` and
export it (the binary does not load `.env` itself), then `make run`.

## 6. Build, test and lint

The `Makefile` includes `make/go-service.mk` (synced from platform-core; do not edit).

| Task | Command |
|---|---|
| Build | `make build` (or `make docker`, which builds the image `team-chat:dev`) |
| Test | `make test` (race detector; sets `DOCKER_HOST` from the docker context) |
| Everything CI runs | `make check` (vet, golangci-lint, tests with Docker required, `.env.example` drift) |
| Regenerate stubs | `make proto` (`buf generate`; `buf.gen.yaml`, input `proto/`, Go package prefix `github.com/buidangphuc/team-chat/generated`) |

The Postgres tests (`internal/store`, `internal/events`, `internal/app`) start a testkit Postgres
container, so they need Docker; `make check` fails instead of skipping without it. Other tests use
the in-memory repository (`InMemoryChatRepository`, `InMemoryOutbox`) and an ephemeral gRPC server.
`coverage.out` feeds the root `sonar-project.properties`.

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
- Events are published only while Kafka is reachable. During an outage `chat_outbox_events`
  accumulates `pending` rows (retried with backoff); they are published once it recovers.
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
- Parked (`failed`) outbox rows have no alert yet; recover with `outbox.Store.RequeueParked` (v1) or manual SQL.
- `StreamChat` is an unimplemented seed RPC in this service.

## 10. Links

- Root `AGENTS.md`: the rules (database per service, contract as source of truth, Kafka for events).
- `platform-core/docs/ADR/0001-proto-distribution.md`, `0002-async-broker.md`,
  `0003-auth-model.md`, `0009-payment-order-event-integration.md` (outbox pattern).
