# team-notification

Go gRPC service (port `:50058`) that owns the in-app notification inbox, price-drop and
back-in-stock alert subscriptions, and per-user notification preferences. It also runs three
Kafka consumers that turn domain events into inbox notifications. It is deployed in the root
compose stack. Bounded context: notifications. It owns the `notification_db` database.

It sends nothing outside the inbox: no email, push or SMS exists in the code.

The service is built on go-sdk v0.2.1 (ADR-0016, vendored in `third_party/go-sdk`). Layout:

| Path | Role |
|---|---|
| `cmd/server` | Process: signals, settings, exit code. |
| `internal/app` | Composition root: builds and runs everything (`app.Run`). |
| `internal/config` | `Settings`, built on SDK `config`. |
| `internal/rpc` | gRPC transport: decodes requests, reads the caller, maps errors. |
| `internal/notification` | Use cases: alert subscriptions and preferences. |
| `internal/store` | Postgres repositories (with in-memory twins for tests) and the consumer state. |
| `internal/events` | The listing, chat and order consumers and their SDK consume wiring. |
| `internal/upstream` | Outbound gRPC to team-domain and team-identity (sender names). |

## Contract

Service `platform.notification.v1.NotificationService` (proto vendored in `proto/platform/notification/`).
The service is reached through `team-gateway`, which forwards a resolved Principal as
`x-principal-id`, `x-principal-type` and `x-principal-scopes` metadata (ADR-0003); the SDK auth
interceptor reads it.

**Authorization rule for every RPC** (`callerUserID`, `internal/rpc/notification.go`): no principal, an
anonymous principal or an empty id gives `Unauthenticated`; a non-user (service) principal gives
`PermissionDenied`; otherwise the principal id is the user and every query is scoped to it. No RPC checks
a scope.

| RPC | Behaviour |
|---|---|
| `ListNotifications` | Caller's notifications, newest first. `page_size` defaults to 20, `page_number` to 1 (1-based); returns `total_unread`. |
| `MarkAsRead` | Empty `id` marks all of the caller's notifications read. A non-empty id that is missing or belongs to another user gives `NotFound`. |
| `GetUnreadCount` | Caller's unread count. |
| `SubscribeAlert` | `listing_id` and a non-`UNSPECIFIED` `AlertType` are required. Idempotent per (user, listing, type). |
| `UnsubscribeAlert` | Idempotent; a missing or foreign id is a no-op success. |
| `ListAlertSubscriptions` | Caller's subscriptions. |
| `GetNotificationPrefs` | Stored prefs, or defaults when the user has none (`notification.DefaultPrefs`). |
| `UpdateNotificationPrefs` | Upserts `type_enabled` (keyed by `NotificationType` enum name) and `digest_freq`. |

Errors that are not a known validation or not-found case become `Internal` with the message
`internal error`; the cause is logged (`grpcx.Status`). The gRPC server also registers reflection and the
standard health service; health follows a Postgres ping and drains before shutdown.

**Consumes (outbound gRPC)**, only for chat sender names (`internal/upstream/names.go`). It calls through
SDK `upstream.Dial` with a per-call service principal `service-team-notification` and scopes
`listing.read,identity.read`:

| Upstream | RPC | Env |
|---|---|---|
| team-domain | `ListingService.BatchGetStorefronts` (shop display name, when the sender is the thread's seller) | `UPSTREAM_DOMAIN_ADDR` |
| team-identity | `PublicProfileService.GetPublicProfiles` (user display name) | `UPSTREAM_IDENTITY_ADDR` |

Each lookup has a 2 s timeout; any failure falls back to the label `Người dùng` and the notification is
still created. Connections are lazy.

## Events

Consumed only (it produces no events except dead-letter records). Each record value is a marshalled
`platform.events.v1.EventEnvelope`; `event_id` is required.

| Topic | Envelope `type` | Effect | DLQ |
|---|---|---|---|
| `listing.events` | `platform.listing.v1.ListingChanged` | Self-diffs the snapshot against the stored last-seen price and stock. A price decrease notifies `PRICE_DROP` subscribers; a stock move from 0 to positive notifies `BACK_IN_STOCK` subscribers. A first-ever snapshot (no prior) never fires. | `listing.events.dlq` |
| `chat.events` | `platform.chat.v1.ChatMessage` | One `CHAT` notification to `recipient_id` (never the sender; skipped if `recipient_id` is empty). Link `/chat/{thread_id}`. | `chat.events.dlq` |
| `order.events` | `platform.order.v1.OrderShipped` | One `ORDER` notification to `buyer_id` with carrier and tracking code. Link `/account/orders/{order_id}`. Other order event types are ignored. | `order.events.dlq` |

Delivery discipline: each consumer is an SDK `consume.Consumer` (`events.NewRunner`) and a lifecycle
component that stops after the gRPC server. Auto-commit is off, and an offset advances only after the
record is applied or dead-lettered. A transient error is retried (5 attempts, backoff from 100 ms); an
`ErrPermanent` error (bad envelope or payload, missing ids) goes to the DLQ at once. When a record can be
neither handled nor dead-lettered, the consumer stops with an error without committing, the service
exits, and the record is redelivered after restart. Chat and order notifications honour the recipient's
`type_enabled` pref (a type with no stored setting counts as enabled). Listing alerts do not check prefs.
A dead-lettered record keeps its key and value and carries the SDK's `dlq-*` headers (cause, source topic,
partition, offset and group).

Idempotency: a ledger keyed by (`consumer`, `event_id`) in `processed_events` (SDK `consume.Dedupe` on the
existing table: `Processed` reads, `Seen` records), recorded after the effect succeeds. Consumer names: `team-notification.listing`,
`team-notification.chat`, `team-notification.order`.

Kafka has no enable flag: the three consumers always run. Brokers are shared (`KAFKA_BROKERS`); topic, DLQ
and group are per consumer.

## Data

Database `notification_db` on the shared Postgres (user `notification_svc`). Migrations in `migrations/`
(golang-migrate format, `*.up.sql` and `*.down.sql`). The service does not run them; the
`team-notification-migrate` job in `docker-compose.services.yaml` applies them (`up`) before the service starts.

| Migration | Table | Notes |
|---|---|---|
| 001 | `notifications` | `id`, `user_id`, `title`, `body`, `type` (int), `link_url`, `is_read`, `created_at`; index `(user_id, created_at DESC)`. |
| 002 | `alert_subscriptions` | Unique `(user_id, listing_id, type)`; index `(listing_id, type)` used by the listing consumer. |
| 003 | `notification_prefs` | One row per user: `prefs` JSONB, `digest_freq` int, `updated_at`; index on `digest_freq`. |
| 004 | `processed_events`, `listing_last_seen` | Durable consumer state: dedupe ledger `(consumer, event_id)`, and last `price` (BIGINT) and `stock` (INT) per listing, each nullable (null means unknown prior). |

Consumer state is durable, so a restart neither re-notifies a redelivered event nor loses the price and stock
baseline. In-memory variants (`InMemoryDeduper`, `InMemoryPriceStateStore`, `InMemoryStockStateStore`) are used
in unit tests only; `internal/app` always wires the Postgres ones.

The pool is opened with a ping. Outside `ENV=local` or `test` an unreachable database fails boot (non-nil error, port
never opened). With `ENV=local` (or `test`) the service logs a warning and starts on a lazily connecting pool: calls that
need the database fail with `Internal` until it is up, and health reports `NOT_SERVING`.

## Configuration

Read from the environment into `config.Settings`; `.env.example` lists every key and a test keeps the two in
sync (`make check-env`). `GRPC_PORT` and `DATABASE_URL` are required and have no defaults.

| Variable | Default | Notes |
|---|---|---|
| `ENV` | required | `local` or `test` allow starting without the database; `staging`, `stage`, `prod`, `production` also require `KAFKA_BROKERS`. |
| `LOG_LEVEL`, `LOG_JSON` | `info`, `true` | |
| `GRPC_HOST`, `GRPC_PORT` | `0.0.0.0`, required | Compose and the Dockerfile set `50058`. |
| `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY` | `10s`, `0s` | Go durations. |
| `DATABASE_URL`, `DB_MAX_CONNS` | required, `10` | |
| `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME` | `false`, none, none | |
| `KAFKA_BROKERS` | `localhost:9092` locally, required otherwise | Comma-separated; the only broker setting. |
| `LISTING_EVENTS_TOPIC` / `LISTING_EVENTS_DLQ_TOPIC` | `listing.events` / `listing.events.dlq` | |
| `NOTIFICATION_LISTING_CONSUMER_GROUP` | `team-notification.listing` | |
| `CHAT_EVENTS_TOPIC` / `CHAT_EVENTS_DLQ_TOPIC` | `chat.events` / `chat.events.dlq` | |
| `NOTIFICATION_CHAT_CONSUMER_GROUP` | `team-notification.chat` | |
| `ORDER_EVENTS_TOPIC` / `ORDER_EVENTS_DLQ_TOPIC` | `order.events` / `order.events.dlq` | |
| `NOTIFICATION_ORDER_CONSUMER_GROUP` | `team-notification.order` | |
| `UPSTREAM_DOMAIN_ADDR`, `UPSTREAM_IDENTITY_ADDR` | `localhost:50051`, `localhost:50053` | |

There are no enable flags: the database, Kafka and gRPC reflection are always on.

## Run locally

Root compose (shared Postgres, Redpanda, migrate job, service), from the repo root:

```bash
docker compose -f docker-compose.services.yaml up -d --build team-notification   # pulls in postgres, redpanda-init and team-notification-migrate
```

The service container sets `KAFKA_BROKERS=redpanda:9092`, the topic, group and DLQ names, and
`UPSTREAM_DOMAIN_ADDR=team-domain-svc:50051` / `UPSTREAM_IDENTITY_ADDR=team-identity-svc:50053`. It
publishes no host port; the gateway reaches it at `team-notification-svc:50058`.

Standalone (needs a reachable Postgres with the migrations applied, a broker, and `generated/` present):

```bash
make proto                                        # buf generate; see Gotchas
export GRPC_PORT=50058
export DATABASE_URL=postgres://notification_svc:notification_pass@localhost:<host-port>/notification_db?sslmode=disable
export KAFKA_BROKERS=<host:port>                  # the root compose Redpanda exposes 19092 on the host
make run
```

## Build, test and lint

```bash
make proto    # once, and after any proto change
make check    # vet + lint + test (Docker) + check-env
make docker   # builds the image (generates stubs first)
```

`make check` needs Docker: the app, consumer and consumer-state tests start Postgres and Redpanda with the SDK
`testkit` (`GO_SDK_REQUIRE_DOCKER=1` makes a missing Docker a failure, not a skip).

## Spec and verification

- Feature manifest: `FEATURES.yaml` (15 features: center, alerts, alerts-delivery, unread-badge,
  per-user-isolation, chat-message, chat-preferences, order-shipped, chat-sender-name, state-survives-restart,
  chat-outbox-delivery, and four `notification-runtime.*` entries). Each user-facing one has a `covered_by`
  scenario in `platform-e2e`; the runtime ones are `not-testable` and name their unit tests.
- Verify coverage: `make -C platform-e2e features-check`; verify a change:
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>` at the repo root), per the root README's ASDLC.
  Related changes: `notify-chat-and-shipment`, `notification-delivery-hardening`,
  `migrate-team-notification-to-go-sdk`.

## Gotchas

- `generated/` is gitignored (root `.gitignore`). Run `make proto` (uses `buf.gen.yaml`, input `proto/`, remote
  plugins from buf.build, so it needs network access) before building or testing.
- `proto/` is vendored from `platform-core` (ADR-0001). Never edit it here; change the contract in platform-core.
- `third_party/go-sdk` is a vendored copy: update it with `platform-core/tools/sync-go-sdk.sh`, never by hand.
- The chat consumer ignores the event's `sender_name` and looks the name up itself.

## Known gaps

- **Wrong default Postgres port.** `.env.example` uses `localhost:5440`, but `AGENTS.md` assigns `5441` to
  postgres-notification (`5440` is postgres-promotion). In root compose the service uses `postgres:5432` and is
  unaffected.
- **No digest delivery.** `digest_freq` is stored and `PrefsService.BundleDigest` / `ListUsersByDigestFreq`
  exist, but nothing calls them: no scheduler runs and no digest notification is ever produced. The code comment
  says wiring the cron is out of scope.
- **`promotion.events` is not consumed.** The topic is created by `redpanda-init`, but no code references it.
  `NOTIFICATION_TYPE_PROMOTION` notifications are never produced by this service.
- **The dedupe mark is not in the effect's transaction.** The ledger is written after the effect (as before),
  so a crash between the two can notify twice; the SDK pattern is to mark inside the effect's transaction.
- **Listing alerts ignore preferences.** Only chat and order notifications check `type_enabled`.

## Links

- Repo rules and port map: [`../AGENTS.md`](../AGENTS.md)
- ADRs in `../platform-core/docs/ADR/`: `0001-proto-distribution`, `0002-async-broker`, `0003-auth-model`,
  `0016-go-service-convention-and-sdk`
