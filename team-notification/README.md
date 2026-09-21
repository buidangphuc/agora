# team-notification — In-App Notification Center & Alert Dispatch Engine (Go)

The `team-notification` microservice powers the central notification inbox, price-drop and back-in-stock alert subscriptions, asynchronous event ingestion from Kafka event streams, user notification preference management, and scheduled digest delivery across the Agora marketplace.

The service strictly complies with **Database-per-service (Rule 3)**, owning its dedicated `notification_db` database on PostgreSQL (port `:5441`), and exposes gRPC service `platform.notification.v1.NotificationService` on port `:50058`.

---

## 1. Service Overview & Responsibilities

`team-notification` acts as the unified communication and alerting hub for buyers and sellers. Key responsibilities include:

1. **In-App Notification Inbox & Counter:**
   - Stores and serves user notifications categorized by `NotificationType` (Orders, Promotions, System, Chat, Price Drop, Back in Stock).
   - Serves high-performance unread badge counters (`GetUnreadCount`) for frontend notification bell indicators.
   - Provides granular and bulk read operations (`MarkAsRead`) supporting single-item or mark-all transitions.
2. **Asynchronous Event Routing & Consumer Pipeline:**
   - Consumes domain events from Kafka topics (`listing.events`, `order.events`, `chat.events`, `promotion.events`).
   - Self-diffing `ListingConsumer` evaluates `ListingChanged` snapshots against local state to detect price drops and 0-to-positive stock transitions without requiring upstream delta events.
   - Idempotent processing backed by an event deduplication ledger (`Deduper`) and Dead Letter Queue (`DLQ`) routing for poison messages.
3. **Price-Drop & Back-in-Stock Alert Subscriptions:**
   - Allows users to subscribe to specific listing alerts (`SubscribeAlert`, `UnsubscribeAlert`, `ListAlertSubscriptions`).
   - Automatically fans out in-app alerts to subscribed users when matching catalog state transitions occur.
4. **User Notification Preferences & Digest Scheduling:**
   - Manages granular per-type opt-in preferences (`notification_prefs.prefs` JSONB map) to control channel permissions.
   - Supports configurable digest delivery cadences (`DIGEST_FREQUENCY_OFF`, `DIGEST_FREQUENCY_DAILY`, `DIGEST_FREQUENCY_WEEKLY`) for notification bundling.

---

## 2. Technology Stack & Key Libraries

- **Language & Runtime:** Go 1.22 (Standard Library + pinned dependencies)
- **RPC Framework:** gRPC Go (`google.golang.org/grpc`), Protocol Buffers v2 (`google.golang.org/protobuf`), Buf CLI managed generation.
- **Database & Storage:** PostgreSQL 16 via `github.com/jackc/pgx/v5` (`pgxpool` connection pooling).
- **Kafka & Event Streaming:** `github.com/twmb/franz-go` (`kgo.Client`), consumer groups with manual offset commit and DLQ support.
- **Security & Authorization:** Tokenless downstream architecture; extracts forwarded `Principal` metadata (`x-principal-id`, `x-principal-scopes`) from Gateway requests.
- **Observability:** OpenTelemetry Go SDK (`go.opentelemetry.io/otel`), structured logging via `log/slog`.
- **Testing:** Native Go `testing`, `testify`, table-driven unit tests, in-memory repositories and consumer fakes.

---

## 3. Detailed Architecture Diagram

```mermaid
flowchart TD
    subgraph Clients["Clients Layer"]
        UserBrowser["User Browser / Mobile App"]
    end

    subgraph Edge["Edge Layer"]
        Gateway["team-gateway (Connect Edge :8080)"]
    end

    subgraph NotificationService["team-notification Service (:50058)"]
        GRPCServer["gRPC Server (Auth & Tracing Interceptors)"]
        
        subgraph Handlers["RPC Handlers"]
            NotifHandler["NotificationHandler (List, Read, Unread)"]
            AlertsHandler["AlertsHandler (Subscribe, Unsubscribe)"]
            PrefsHandler["NotificationPrefsHandler (Get & Update Prefs)"]
        end

        subgraph Services["Domain Services"]
            AlertsSvc["AlertsService"]
            PrefsSvc["NotificationPrefsService"]
        end

        subgraph Ingestion["Event Ingestion Pipeline"]
            ListingConsumer["ListingConsumer (Self-Diff Engine)"]
            DedupeLedger["Dedupe Ledger (Idempotency Guard)"]
            PriceStockStore["Price & Stock Last-Seen State Store"]
        end

        subgraph Repositories["Data Access Layer"]
            NotifRepo["PostgresNotificationRepo"]
            AlertsRepo["PostgresAlertSubscriptionRepo"]
            PrefsRepo["PostgresNotificationPrefsRepo"]
        end
    end

    subgraph EventBroker["Kafka / Redpanda Broker"]
        ListingEvents[("listing.events")]
        OrderEvents[("order.events")]
        ChatEvents[("chat.events")]
        DLQTopic[("listing.events.dlq")]
    end

    subgraph Storage["Persistence Layer"]
        PostgresDB[("PostgreSQL: notification_db (:5441)")]
    end

    UserBrowser -->|HTTP / Connect RPC| Gateway
    Gateway -->|gRPC + x-principal metadata| GRPCServer
    GRPCServer --> NotifHandler
    GRPCServer --> AlertsHandler
    GRPCServer --> PrefsHandler

    NotifHandler --> NotifRepo
    AlertsHandler --> AlertsSvc
    AlertsSvc --> AlertsRepo
    PrefsHandler --> PrefsSvc
    PrefsSvc --> PrefsRepo

    ListingEvents -->|Fetch Record| ListingConsumer
    ListingConsumer --> DedupeLedger
    ListingConsumer --> PriceStockStore
    ListingConsumer -->|Query Subscribers| AlertsRepo
    ListingConsumer -->|Fanout Notifications| NotifRepo
    ListingConsumer -->|Poison Records| DLQTopic

    NotifRepo --> PostgresDB
    AlertsRepo --> PostgresDB
    PrefsRepo --> PostgresDB
```

---

## 4. Internal Package Structure & Data Schemas

### 4.1. Internal Package Structure

```
team-notification/
├── cmd/
│   └── server/
│       └── main.go                 # Service entrypoint, gRPC listener & Kafka consumer loops
├── generated/                      # Proto generated code from platform-core
│   └── platform/
│       ├── common/v1/              # Common proto definitions (Principal, PageRequest)
│       ├── events/v1/              # EventEnvelope proto stubs
│       ├── listing/v1/             # ListingChanged proto stubs
│       └── notification/v1/        # NotificationService proto & gRPC stubs
├── internal/
│   ├── bootstrap/
│   │   └── kafka.go                # Kafka consumer group & DLQ producer bootstrap
│   ├── config/
│   │   ├── config.go               # Struct configuration loaded from ENV variables
│   │   └── config_test.go          # Config loading unit tests
│   ├── consumer/
│   │   ├── listing.go              # ListingChanged self-diff consumer, dedupe ledger & DLQ
│   │   └── listing_test.go         # Price-drop and back-in-stock consumer test suites
│   ├── grpcserver/
│   │   └── server.go               # gRPC server configuration with interceptors
│   ├── handler/
│   │   ├── alerts.go               # Price-drop / back-in-stock subscription RPC handlers
│   │   ├── notification.go         # Notification list, mark-read and unread count handlers
│   │   ├── notification_prefs.go   # User notification preference RPC handlers
│   │   └── notification_test.go    # Unit tests for notification handlers
│   ├── repository/
│   │   ├── alerts.go               # Alert subscription repository interface & Postgres queries
│   │   ├── notification.go         # Notification inbox repository interface & Postgres queries
│   │   └── notification_prefs.go   # Preferences repository interface & Postgres JSONB queries
│   └── service/
│       ├── alerts.go               # Alert subscription business logic
│       ├── notification_prefs.go   # User preference validation and defaults logic
│       └── notification_prefs_test.go # Preference service unit tests
└── migrations/
    ├── 001_create_notifications.up.sql   # notifications table & indexes
    ├── 002_alert_subscriptions.up.sql    # alert_subscriptions table & unique index
    └── 003_notification_prefs.up.sql     # notification_prefs table & digest index
```

### 4.2. Database Entity-Relationship Diagram

```mermaid
erDiagram
    NOTIFICATIONS {
        varchar id PK
        varchar user_id
        varchar title
        text body
        int type
        varchar link_url
        boolean is_read
        timestamptz created_at
    }

    ALERT_SUBSCRIPTIONS {
        varchar id PK
        varchar user_id
        varchar listing_id
        int type
        timestamptz created_at
    }

    NOTIFICATION_PREFS {
        varchar user_id PK
        jsonb prefs
        int digest_freq
        timestamptz updated_at
    }

    NOTIFICATIONS }o--|| NOTIFICATION_PREFS : "filtered by preferences of"
    ALERT_SUBSCRIPTIONS }o--|| NOTIFICATIONS : "triggers creation of"
```

---

## 5. Core Workflows

### 5.1. Notification Event Routing & Self-Diffing Alert Ingestion

1. **Ingestion & Envelope Decoding:** `ListingConsumer` fetches records from `listing.events`, decodes the `EventEnvelope`, and validates `type == "platform.listing.v1.ListingChanged"`.
2. **Idempotency Guard:** Checks the `Deduper` ledger for `event_id`. Duplicate deliveries are acknowledged as no-ops.
3. **State Self-Diffing:**
   - `ListingChanged` contains a full product snapshot, not a delta.
   - **Price Drop:** Compares `current_price` against `PriceStateStore.Get(listingID)`. If `current_price < previous_price`, it queries all `ALERT_TYPE_PRICE_DROP` subscribers for that listing and creates notification records.
   - **Back in Stock:** Compares `current_stock` against `StockStateStore.Get(listingID)`. On a transition from `0` to `> 0`, it queries all `ALERT_TYPE_BACK_IN_STOCK` subscribers and emits notifications.
4. **State Advancement & Offset Commit:** Last-seen price and stock are updated only after notifications succeed, ensuring transient database errors trigger retry on redelivery.
5. **DLQ Routing:** Unrecoverable / malformed records are diverted to `listing.events.dlq` before advancing offsets.

```mermaid
sequenceDiagram
    autonumber
    participant Kafka as Kafka (listing.events)
    participant Consumer as ListingConsumer
    participant Dedupe as Dedupe Ledger
    participant State as Price & Stock State
    participant Repo as Alerts & Notif Repo
    participant DB as Postgres (notification_db)

    Kafka->>Consumer: Fetch record (ListingChanged snapshot)
    Consumer->>Dedupe: IsProcessed(event_id)
    Dedupe-->>Consumer: false (New event)

    Consumer->>State: Get last-seen price & stock
    State-->>Consumer: prevPrice: 200000, prevStock: 0
    Note over Consumer: Snapshot price: 150000 (Drop!)<br/>Snapshot stock: 10 (Restocked!)

    Consumer->>Repo: ListByListingAndType(listing_id, PRICE_DROP)
    Repo->>DB: SELECT * FROM alert_subscriptions WHERE listing_id=$1 AND type=PRICE_DROP
    DB-->>Repo: [Sub{user_id: "u1"}, Sub{user_id: "u2"}]
    
    Consumer->>Repo: CreateNotification(user: "u1", "Giảm giá!", ...)
    Repo->>DB: INSERT INTO notifications
    DB-->>Repo: OK

    Consumer->>State: Set(listing_id, price: 150000, stock: 10)
    Consumer->>Dedupe: MarkProcessed(event_id)
    Consumer->>Kafka: Commit offset
```

### 5.2. User Notification Preferences & Filtering

- Users manage notification channel preferences via `UpdateNotificationPrefs`.
- Stored as a flexible `JSONB` map (e.g. `{"NOTIFICATION_TYPE_ORDER": true, "NOTIFICATION_TYPE_PROMOTION": false}`).
- When sending non-critical marketing or promotional alerts, the dispatcher inspects user preferences; disabled types are suppressed.

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Gateway as team-gateway
    participant Notif as team-notification
    participant DB as Postgres (notification_db)

    User->>Gateway: POST /UpdateNotificationPrefs(prefs: {type_enabled: {...}, digest_freq: DAILY})
    Gateway->>Notif: gRPC UpdateNotificationPrefs
    Notif->>DB: INSERT INTO notification_prefs (user_id, prefs, digest_freq) VALUES ($1, $2, $3) ON CONFLICT (user_id) DO UPDATE SET prefs=$2, digest_freq=$3, updated_at=NOW()
    DB-->>Notif: OK
    Notif-->>Gateway: UpdateNotificationPrefsResponse
    Gateway-->>User: 200 OK
```

### 5.3. Digest Scheduling & In-App Notification Feed

- **Real-Time Delivery:** Urgent notifications (e.g., order confirmations, dispute updates, direct chat alerts) are immediately written to the inbox for real-time unread badge counts.
- **Digest Bundling:** Users opting into `DIGEST_FREQUENCY_DAILY` or `DIGEST_FREQUENCY_WEEKLY` have promotional or summary notifications batched and delivered on cadence.
- **Feed Retrieval & Badge:** Frontend requests `ListNotifications(page_size, page_number)` and `GetUnreadCount()`.

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Gateway as team-gateway
    participant Notif as team-notification
    participant DB as Postgres (notification_db)

    User->>Gateway: GET /GetUnreadCount
    Gateway->>Notif: gRPC GetUnreadCount
    Notif->>DB: SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND is_read=FALSE
    DB-->>Notif: count = 3
    Notif-->>User: GetUnreadCountResponse(unread_count: 3)

    User->>Gateway: POST /MarkAsRead(id: "noti_123")
    Gateway->>Notif: gRPC MarkAsRead
    Notif->>DB: UPDATE notifications SET is_read=TRUE WHERE id=$1 AND user_id=$2
    DB-->>Notif: OK
    Notif-->>User: MarkAsReadResponse(success: true)
```

---

## 6. gRPC API Specification (`NotificationService`)

Defined in package `platform.notification.v1`.

### 6.1. Notification Inbox
| RPC Method | Request Payload | Response Payload | Description |
|---|---|---|---|
| `ListNotifications` | `page_size`, `page_number` | `notifications` (repeated), `total_unread` | Retrieves paginated notification history and unread count. |
| `MarkAsRead` | `id` (empty to mark all) | `success` (bool) | Marks a single notification or all notifications of user as read. |
| `GetUnreadCount` | `{}` | `unread_count` (int32) | Fetches current unread notification count for badge rendering. |

### 6.2. Alert Subscriptions
| RPC Method | Request Payload | Response Payload | Description |
|---|---|---|---|
| `SubscribeAlert` | `listing_id`, `type` (AlertType) | `subscription` (AlertSubscription) | Subscribes user to price-drop or back-in-stock alerts. |
| `UnsubscribeAlert`| `subscription_id` | `{}` | Cancels an active alert subscription. |
| `ListAlertSubscriptions` | `{}` | `subscriptions` (repeated) | Lists all active alert subscriptions for the caller. |

### 6.3. User Preferences
| RPC Method | Request Payload | Response Payload | Description |
|---|---|---|---|
| `GetNotificationPrefs` | `{}` | `prefs` (NotificationPrefs) | Fetches user's per-type opt-ins and digest frequency. |
| `UpdateNotificationPrefs` | `prefs` (NotificationPrefs) | `prefs` (NotificationPrefs) | Updates user notification preferences and digest schedule. |

### 6.4. Enums Reference

```protobuf
enum NotificationType {
  NOTIFICATION_TYPE_UNSPECIFIED   = 0;
  NOTIFICATION_TYPE_ORDER         = 1;
  NOTIFICATION_TYPE_PROMOTION     = 2;
  NOTIFICATION_TYPE_SYSTEM        = 3;
  NOTIFICATION_TYPE_CHAT          = 4;
  NOTIFICATION_TYPE_PRICE_DROP    = 5;
  NOTIFICATION_TYPE_BACK_IN_STOCK = 6;
}

enum AlertType {
  ALERT_TYPE_UNSPECIFIED   = 0;
  ALERT_TYPE_PRICE_DROP    = 1;
  ALERT_TYPE_BACK_IN_STOCK = 2;
}

enum DigestFrequency {
  DIGEST_FREQUENCY_OFF    = 0;
  DIGEST_FREQUENCY_DAILY  = 1;
  DIGEST_FREQUENCY_WEEKLY = 2;
}
```

---

## 7. Environment Variables

| Variable | Default | Description |
|---|---|---|
| `GRPC_PORT` | `50058` | gRPC server listening port |
| `DATABASE_URL` | `postgres://notification_svc:notification_pass@localhost:5441/notification_db?sslmode=disable` | PostgreSQL connection string |
| `KAFKA_BROKER` | `localhost:19092` | Kafka / Redpanda bootstrap broker address |
| `KAFKA_GROUP_ID`| `team-notification-service` | Consumer group ID for event subscription |
| `KAFKA_LISTING_TOPIC`| `listing.events` | Kafka topic for catalog changes |
| `KAFKA_DLQ_TOPIC` | `listing.events.dlq` | Dead Letter Queue topic for poison records |

---

## 8. Local Setup & Testing

```bash
# 1. Start Postgres & Kafka infrastructure
docker compose -p platform-core up -d postgres-notification redpanda

# 2. Run test suite
go test -v -race ./...

# 3. Start notification service locally
go run cmd/server/main.go
```
