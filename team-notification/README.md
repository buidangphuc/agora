# team-notification

Go gRPC service (port `:50058`) that owns the in-app notification inbox, price-drop and
back-in-stock alert subscriptions, and per-user notification preferences. It also runs three
Kafka consumers that turn domain events into inbox notifications. It is deployed in the root
compose stack. Bounded context: notifications. It owns the `notification_db` database.

It sends nothing outside the inbox: no email, push or SMS exists in the code.

## Contract

Service `platform.notification.v1.NotificationService` (proto vendored in `proto/platform/notification/`).
The service is reached through `team-gateway`, which forwards a resolved Principal as
`x-principal-id`, `x-principal-type` and `x-principal-scopes` metadata (ADR-0003).

**Authorization rule for every RPC** (`callerUserID`, `internal/handler/notification.go`): no principal, an
anonymous principal or an empty id gives `Unauthenticated`; a non-user (service) principal gives
`PermissionDenied`; otherwise the principal id is the user and every query is scoped to it. No RPC calls
`RequireScopes`, so no scope is checked.

| RPC | Behaviour |
|---|---|
| `ListNotifications` | Caller's notifications, newest first. `page_size` defaults to 20, `page_number` to 1 (1-based); returns `total_unread`. |
| `MarkAsRead` | Empty `id` marks all of the caller's notifications read. A non-empty id that is missing or belongs to another user gives `NotFound`. |
| `GetUnreadCount` | Caller's unread count. |
| `SubscribeAlert` | `listing_id` and a non-`UNSPECIFIED` `AlertType` are required. Idempotent per (user, listing, type). |
| `UnsubscribeAlert` | Idempotent; a missing or foreign id is a no-op success. |
| `ListAlertSubscriptions` | Caller's subscriptions. |
| `GetNotificationPrefs` | Stored prefs, or defaults when the user has none (`service.DefaultPrefs`). |
| `UpdateNotificationPrefs` | Upserts `type_enabled` (keyed by `NotificationType` enum name) and `digest_freq`. |

The alert and prefs RPCs return `Unimplemented` when no database pool was created (see Known gaps).
The gRPC server also registers reflection and the standard health service.

**Consumes (outbound gRPC)**, only for chat sender names (`internal/upstream/names.go`). It calls as a
service principal `service-team-notification` with scopes `listing.read,identity.read`:

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

Delivery discipline (`internal/consumer/`): auto-commit is off, and an offset advances only after the record
is applied or dead-lettered. A transient error is retried; an `ErrPermanent` error (bad envelope or payload,
missing ids) goes to the DLQ. Chat and order notifications honour the recipient's `type_enabled` pref (a type
with no stored setting counts as enabled). Listing alerts do not check prefs.

Idempotency: a ledger keyed by (`consumer`, `event_id`) in `processed_events`, recorded after the effect
succeeds. Consumer names: `team-notification.listing`, `team-notification.chat`, `team-notification.order`.

The consumers start only when `KAFKA_ENABLED=true` and a database pool exists. Brokers and the enable flag are
shared by all three; topic, DLQ and group are per consumer.

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
baseline. In-memory variants (`InMemoryDeduper`, `InMemoryPriceStateStore`, `InMemoryStockStateStore`) are the
code defaults and are used in unit tests only; `cmd/server/main.go` always wires the Postgres ones.

## Configuration

Read from the environment. Only the variables below are read by code.

| Variable | Default | Read in |
|---|---|---|
| `GRPC_PORT` | `50058` | `internal/config` |
| `DATABASE_URL` | `postgres://notification_svc:notification_pass@localhost:5440/notification_db?sslmode=disable` | `internal/config` |
| `UPSTREAM_DOMAIN_ADDR` | `localhost:50051` | `internal/config` |
| `UPSTREAM_IDENTITY_ADDR` | `localhost:50053` | `internal/config` |
| `KAFKA_ENABLED` | `false` | `internal/bootstrap/kafka.go` |
| `KAFKA_BROKERS` | `localhost:9092` (comma-separated) | `internal/bootstrap/kafka.go` |
| `LISTING_EVENTS_TOPIC` / `LISTING_EVENTS_DLQ_TOPIC` | `listing.events` / `listing.events.dlq` | same |
| `NOTIFICATION_LISTING_CONSUMER_GROUP` | `team-notification.listing` | same |
| `CHAT_EVENTS_TOPIC` / `CHAT_EVENTS_DLQ_TOPIC` | `chat.events` / `chat.events.dlq` | same |
| `NOTIFICATION_CHAT_CONSUMER_GROUP` | `team-notification.chat` | same |
| `ORDER_EVENTS_TOPIC` / `ORDER_EVENTS_DLQ_TOPIC` | `order.events` / `order.events.dlq` | same |
| `NOTIFICATION_ORDER_CONSUMER_GROUP` | `team-notification.order` | same |

`.env.example` also lists `ENV`, `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_REFLECTION_ENABLED`,
`SHUTDOWN_GRACE_SECONDS`, `DATABASE_ENABLED`, `DB_MAX_CONNS`, `OTEL_*`. The code does not read any of these
(logging is always JSON via `log/slog`; the gRPC listener binds all interfaces; reflection is always on).
`config.go` also reads `KAFKA_BROKER` into a field nothing uses. There is no `.env.example` drift gate in this
repository.

## Run locally

Root compose (shared Postgres, Redpanda, migrate job, service), from the repo root:

```bash
docker compose up -d --build team-notification   # pulls in postgres, redpanda-init and team-notification-migrate
```

The service container sets `KAFKA_ENABLED=true`, `KAFKA_BROKERS=redpanda:9092`, the topic, group and DLQ
names, and `UPSTREAM_DOMAIN_ADDR=team-domain-svc:50051` / `UPSTREAM_IDENTITY_ADDR=team-identity-svc:50053`. It
publishes no host port; the gateway reaches it at `team-notification-svc:50058`. There is no `--profile jobs`
component.

Standalone (needs a reachable Postgres with the migrations applied, and `generated/` present):

```bash
buf generate                                      # see Gotchas
export DATABASE_URL=postgres://notification_svc:notification_pass@localhost:<host-port>/notification_db?sslmode=disable
go run ./cmd/server
```

Set `KAFKA_ENABLED=true` and `KAFKA_BROKERS=<host:port>` to run the consumers (the root compose Redpanda
exposes `19092` on the host per `AGENTS.md`).

## Build, test and lint

```bash
buf generate                  # once, and after any proto change
go build ./...
go vet ./...
gofmt -l .                    # must print nothing
go test -race ./...
TEST_DATABASE_URL=postgres://... go test ./internal/consumer/...   # also runs durable_state_pg_test.go
```

Without `TEST_DATABASE_URL` the Postgres consumer-state test is skipped; apply `migrations/*.up.sql` to that
database first. This repository has no Makefile and no CI config of its own; the commands above are the
local gate. `Dockerfile` builds `./cmd/server` into a distroless image (`golang:1.22`); it expects `generated/`
to exist in the build context.

## Spec and verification

- Feature manifest: `FEATURES.yaml` (11 features: center, alerts, alerts-delivery, unread-badge,
  per-user-isolation, chat-message, chat-preferences, order-shipped, chat-sender-name, state-survives-restart,
  chat-outbox-delivery). Each has a `covered_by` scenario in `platform-e2e`.
- Verify coverage: `make -C platform-e2e features-check`; verify a change:
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes go through OpenSpec (`openspec/changes/<id>` at the repo root), per the root README's ASDLC.
  Related changes: `notify-chat-and-shipment`, `notification-delivery-hardening`.

## Gotchas

- `generated/` is gitignored (root `.gitignore`). Run `buf generate` (uses `buf.gen.yaml`, input `proto/`, remote
  plugins from buf.build, so it needs network access) before building or testing.
- `proto/` is vendored from `platform-core` (ADR-0001). Never edit it here; change the contract in platform-core.
- The consumers do not run without `KAFKA_ENABLED=true` and a database pool.
- The chat consumer ignores the event's `sender_name` and looks the name up itself.

## Known gaps

- **Wrong default Postgres port.** `DATABASE_URL` defaults and `.env.example` use `localhost:5440`, but
  `AGENTS.md` assigns `5441` to postgres-notification (`5440` is postgres-promotion). In root compose the
  service uses `postgres:5432` and is unaffected.
- **No digest delivery.** `digest_freq` is stored and `PrefsService.BundleDigest` / `ListUsersByDigestFreq`
  exist, but nothing calls them: no scheduler runs and no digest notification is ever produced. The code comment
  says wiring the cron is out of scope.
- **`promotion.events` is not consumed.** The topic is created by `redpanda-init`, but no code references it.
  `NOTIFICATION_TYPE_PROMOTION` notifications are never produced by this service.
- **No scope checks.** Handlers only require a user principal; `RequireScopes` exists in
  `internal/interceptor/auth.go` but is not used. The service trusts the gateway-forwarded principal metadata.
- **"Mock mode" is not a real mode.** `main.go` logs "running in mock mode" only when `pgxpool.New` fails (a
  malformed `DATABASE_URL`). An unreachable database does not fail pool creation (connections are lazy). With a
  nil pool the alert, prefs and consumer wiring is skipped and the inbox repository dereferences the nil pool on
  first use.
- **Inert settings.** No OpenTelemetry is wired (no otel import); the variables listed above as unread are
  ignored. `testify` is an indirect dependency only.
- **Listing alerts ignore preferences.** Only chat and order notifications check `type_enabled`.

## Links

- Repo rules and port map: [`../AGENTS.md`](../AGENTS.md)
- ADRs in `../platform-core/docs/ADR/`: `0001-proto-distribution`, `0002-async-broker`, `0003-auth-model`
