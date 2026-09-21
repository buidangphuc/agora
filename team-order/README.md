# team-order — Order, Shopping Cart & Distributed Saga Microservice

`team-order` is the core transaction processing and order fulfillment microservice in the Agora polyrepo e-commerce platform. It coordinates shopping cart persistence, multi-vendor order checkout via a **Distributed Purchase Saga Orchestrator**, asynchronous payment settlement, carrier logistics tracking, and post-purchase Return Merchandise Authorization (RMA).

Operating on Go 1.22, `team-order` provides a high-performance gRPC API (:50055), exclusively manages its own PostgreSQL database (`order_db`), and enforces platform architectural standards: **Database-per-service (Rule 3)**, **Saga Orchestration with Compensating Transactions**, and **Transactional Outbox Event Sourcing (ADR-0002 / ADR-0013)**.

---

## 1. Service Overview & Business Context

```
┌─────────────────┐       gRPC       ┌─────────────────┐  Reserve/Release  ┌─────────────────┐
│  team-gateway   │ ───────────────> │   team-order    │ ────────────────> │   team-domain   │
│     (:8080)     │  (Principal FW)  │    (:50055)     │   Stock (:50051)  │    (:50051)     │
└─────────────────┘                  └───────┬─┬───────┘                   └─────────────────┘
                                             │ │  Address Lookup           ┌─────────────────┐
                                             │ └─────────────────────────> │  team-identity  │
                                             │         (:50053)            │    (:50053)     │
                                             │                             └─────────────────┘
                                             │    Voucher Reserve/Commit   ┌─────────────────┐
                                             ├───────────────────────────> │ team-promotion  │
                                             │         (:50061)            │    (:50061)     │
                                             │                             └─────────────────┘
                      ┌──────────────────────┼──────────────────────┐
                      │ Consume Payment      │ Outbox Produce       │
                      ▼                      ▼                      ▼
           ┌──────────────────────┐┌──────────────────┐┌────────────────────────┐
           │ Kafka payment.events ││ Kafka order.events││ PostgreSQL order_db   │
           └──────────────────────┘└──────────────────┘└────────────────────────┘
```

### Core Responsibilities
- **Shopping Cart Management**: Real-time cart CRUD with product snapshotting, multi-vendor grouping, and server-side shipping fee calculation.
- **Distributed Purchase Saga Orchestrator**: Multi-service 2-phase coordination during checkout:
  1. Validates and reserves promotional vouchers via `team-promotion`.
  2. Durably records stock-reservation intent and calls `team-domain` to atomically lock inventory with deterministic reservation keys.
  3. Splits orders by seller (Multi-vendor Split), saves orders to `order_db`, and commits stock holds.
  4. Executes background compensating transactions (`compensate`) to release reserved stock and vouchers if any stage fails.
- **Asynchronous Payment Consumer**: Consumes `platform.payment.v1.PaymentSettled` events from Kafka (`payment.events`), dedupes via `processed_events` ledger, transitions order status to `PAID`, commits voucher holds, and enqueues outbox events.
- **Transactional Outbox Publishing**: Publishes `platform.order.v1.OrderPaid` and related events to `order.events` using Postgres `FOR UPDATE SKIP LOCKED` background relaying.
- **RMA (Return & Refund Management)**: Lifecycle state machine for buyer return requests, seller approval/rejection, and refund authorization.
- **Shipment & Logistics Tracking**: Carrier integration (SPX, GHN, GHTK) with tracking codes and waypoint checkpoint logging.

---

## 2. Technology Stack & Key Libraries

| Component / Layer | Technology / Library | Version | Description |
|---|---|---|---|
| **Language Runtime** | Go | `1.22` | Polyrepo-wide standard Go version |
| **RPC Framework** | `google.golang.org/grpc` | `v1.66.0` | High-performance unary and streaming gRPC server |
| **Serialization** | `google.golang.org/protobuf` | `v1.34.2` | Protocol Buffers v2 schema contracts |
| **Database Driver** | `github.com/jackc/pgx/v5` | `v5.6.0` | PostgreSQL driver and connection pooling (`pgxpool`) |
| **Event Streaming** | `github.com/twmb/franz-go` | `v1.18.0` | Native Go Kafka client for both consumer groups and outbox producers |
| **Feature Flagging** | `github.com/open-feature/go-sdk` | `v1.14.0` | Vendor-neutral OpenFeature evaluation API |
| **Flag Provider** | `go.flipt.io/flipt-openfeature-provider` | `v0.2.0` | Flipt backend provider for dynamic runtime feature toggles |
| **Observability (Tracing)** | `go.opentelemetry.io/otel` | `v1.28.0` | OpenTelemetry SDK and OTLP exporter for distributed trace propagation |
| **gRPC Tracing** | `otelgrpc` | `v0.53.0` | Automatic span creation and W3C header context injection |
| **Identifiers** | `github.com/google/uuid` | `v1.6.0` | Deterministic SHA-1 UUIDs for reservation idempotency & UUIDv4 IDs |

---

## 3. System Architecture & Component Diagram

```mermaid
flowchart TD
    subgraph Clients["Edge Ingress"]
        GW["team-gateway (:8080)"]
    end

    subgraph OrderService["team-order Microservice (:50055)"]
        subgraph TransportLayer["Transport & Interceptors"]
            AUTH["Auth Interceptor (x-principal-*)"]
            TRACE["OTel Tracing Interceptor"]
            CART_H["CartHandler"]
            ORDER_H["OrderHandler"]
        end

        subgraph ServiceLayer["Business Logic & Orchestration"]
            CART_SVC["CartService"]
            SAGA_ORCH["OrderService (Saga Orchestrator)"]
            PROMO_SVC["Redemption / Voucher Helper"]
            PAY_CONSUMER["PaymentConsumer (Kafka Reader)"]
        end

        subgraph OutboxEngine["Transactional Outbox"]
            OUT_RELAY["Order Outbox Relayer"]
        end

        subgraph RepositoryLayer["Repository Layer (PostgreSQL)"]
            CART_REPO["PostgresCartRepository"]
            ORDER_REPO["PostgresOrderRepository"]
            SAGA_REPO["PostgresSagaRepository"]
            DEDUPE_REPO["PostgresProcessedEventRepository"]
            RETURN_REPO["PostgresReturnRepository"]
            SHIP_REPO["PostgresShipmentRepository"]
            OUT_REPO["PostgresOutboxRepository"]
        end
    end

    subgraph UpstreamServices["Upstream Microservices (gRPC)"]
        DOM["team-domain (:50051)<br/>ReserveStock / ReleaseStock"]
        IDN["team-identity (:50053)<br/>Address Lookup"]
        PRM["team-promotion (:50061)<br/>Voucher Validate & Commit"]
    end

    subgraph StorageInfra["Data & Streaming Infrastructure"]
        DB[(PostgreSQL order_db)]
        KAFKA_IN{Kafka: payment.events}
        KAFKA_OUT{Kafka: order.events}
    end

    GW -->|"Cart / Order RPCs"| AUTH
    AUTH --> TRACE
    TRACE --> CART_H
    TRACE --> ORDER_H

    CART_H --> CART_SVC
    CART_SVC --> CART_REPO

    ORDER_H --> SAGA_ORCH
    SAGA_ORCH --> SAGA_REPO
    SAGA_ORCH --> ORDER_REPO
    SAGA_ORCH --> CART_REPO
    SAGA_ORCH --> RETURN_REPO
    SAGA_ORCH --> SHIP_REPO
    SAGA_ORCH --> PROMO_SVC

    SAGA_ORCH -->|"gRPC"| DOM
    SAGA_ORCH -->|"gRPC"| IDN
    PROMO_SVC -->|"gRPC"| PRM

    KAFKA_IN -->|"Consume PaymentSettled"| PAY_CONSUMER
    PAY_CONSUMER --> DEDUPE_REPO
    PAY_CONSUMER --> ORDER_REPO
    PAY_CONSUMER -->|"Commit Voucher"| PRM
    PAY_CONSUMER -->|"Enqueue Event"| OUT_REPO

    OUT_RELAY -->|"Claim Pending (SKIP LOCKED)"| OUT_REPO
    OUT_RELAY -->|"Produce OrderPaid"| KAFKA_OUT

    CART_REPO --> DB
    ORDER_REPO --> DB
    SAGA_REPO --> DB
    DEDUPE_REPO --> DB
    RETURN_REPO --> DB
    SHIP_REPO --> DB
    OUT_REPO --> DB
```

---

## 4. Internal Package Structure

The microservice is structured into distinct, decoupled packages:

```
team-order/
├── cmd/
│   └── server/
│       └── main.go                 # Application entrypoint, DI container, signal handling
├── generated/                      # Buf-generated Go protobuf stubs
│   └── platform/
│       ├── common/v1/              # Common schemas (Principal, Pagination, Money)
│       ├── events/v1/              # EventEnvelope schemas
│       ├── identity/v1/            # AddressServiceClient
│       ├── listing/v1/             # ListingServiceClient (ReserveStock/ReleaseStock)
│       ├── order/v1/               # CartService, OrderService, Return, Shipment contracts
│       ├── payment/v1/             # PaymentSettled event payloads
│       └── promotion/v1/           # VoucherServiceClient (Reserve/Commit/Release)
├── internal/
│   ├── bootstrap/                  # Infrastructure resource wiring
│   │   ├── kafka.go                # Franz-go consumer loop & publisher setup
│   │   └── resources.go            # Pgxpool connection lifecycle
│   ├── config/                     # Environment variable binding & validation
│   │   └── config.go               # Config struct with default tags
│   ├── consumer/                   # Kafka event consumers
│   │   └── payment.go              # Idempotent PaymentSettled event consumer
│   ├── events/                     # Kafka event publisher & outbox relayer
│   │   ├── publisher.go            # Topic publisher interface
│   │   └── relayer.go              # Background outbox polling worker
│   ├── featureflags/               # OpenFeature & Flipt dynamic feature flag client
│   │   ├── featureflags.go         # Domain feature flag evaluations
│   │   └── provider.go             # Flipt provider configuration
│   ├── grpcserver/                 # gRPC server construction & health service
│   │   └── server.go               # Interceptors, reflection, and listener binding
│   ├── handler/                    # gRPC transport adaptors
│   │   ├── cart.go                 # CartService gRPC RPC implementation
│   │   └── order.go                # OrderService, RMA, Logistics, and Saga RPCs
│   ├── interceptor/                # Request interceptors
│   │   ├── auth.go                 # x-principal-* header extractor & scope validator
│   │   └── tracing.go              # OpenTelemetry span injector
│   ├── repository/                 # Database layer & SQL queries
│   │   ├── cart.go                 # Cart items SQL (PostgreSQL & InMemory fake)
│   │   ├── order.go                # Orders and order_items SQL
│   │   ├── outbox.go / outbox_pg.go # Transactional outbox persistence & SKIP LOCKED claims
│   │   ├── processed_events.go     # Consumer idempotency deduplication ledger
│   │   ├── return.go               # RMA return requests SQL
│   │   ├── saga.go                 # Saga orchestration state & stock reservation holds
│   │   └── shipment.go             # Logistics shipments and checkpoint logs SQL
│   ├── service/                    # Core business logic
│   │   ├── cart.go                 # Cart calculations and item mutations
│   │   ├── order.go                # Order lifecycle, shipping calculation, RMA, shipments
│   │   ├── redemption.go           # Promotion voucher reservation & rollback helper
│   │   └── saga.go                 # Deterministic reservation ID generation & compensation logic
│   └── upstream/                   # Upstream gRPC client abstractions
│       └── domain.go               # team-domain stock reservation adapter
└── migrations/                     # Database migrations applied via golang-migrate
```

---

## 5. Database Schema & Tables

`team-order` exclusively owns `order_db` on PostgreSQL. Below is the complete entity-relationship model:

```mermaid
erDiagram
    cart_items }o--|| orders : "Converted upon checkout"
    orders ||--|{ order_items : "order_id"
    orders ||--o{ order_returns : "order_id"
    orders ||--o{ shipments : "order_id"
    shipments ||--|{ shipment_checkpoints : "shipment_id"
    order_sagas ||--|{ order_reservations : "saga_id"
    orders ||--o{ order_reservations : "order_id"
    orders ||--o{ order_outbox_events : "aggregate_id"

    cart_items {
        TEXT id PK
        TEXT user_id "Indexed"
        TEXT listing_id
        TEXT variant_id
        INT quantity
        BIGINT unit_price
        TEXT title
        TEXT variant_name
        TEXT image_url
        TEXT seller_id
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    orders {
        TEXT id PK
        TEXT buyer_id "Indexed"
        TEXT seller_id "Indexed"
        INT status "1:PENDING 2:PAID 3:SHIPPED 4:COMPLETED 5:CANCELLED"
        BIGINT total_amount
        BIGINT items_subtotal
        BIGINT shipping_fee
        INT payment_method "1:COD 2:MOMO 3:BANK 4:CARD"
        TEXT currency
        JSONB shipping_address
        TEXT tracking_number
        TEXT voucher_code
        BIGINT discount_amount
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    order_items {
        TEXT id PK
        TEXT order_id FK "Indexed"
        TEXT listing_id
        TEXT variant_id
        TEXT title
        TEXT variant_name
        INT quantity
        BIGINT unit_price
        TEXT image_url
    }

    order_sagas {
        TEXT id PK
        TEXT buyer_id "Indexed"
        INT status "1:PENDING 2:COMPLETED 3:COMPENSATED 4:FAILED"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    order_reservations {
        TEXT id PK "Deterministic Reservation Key"
        TEXT saga_id FK "Indexed"
        TEXT order_id FK "Set on Commit"
        TEXT seller_id
        TEXT buyer_id
        TEXT listing_id
        TEXT variant_id
        INT quantity
        INT status "1:PENDING 2:RESERVED 3:COMMITTED 4:RELEASED 5:RELEASE_FAILED 6:FAILED"
        TIMESTAMPTZ expires_at "Indexed"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    processed_events {
        TEXT event_id PK "Consumer Dedupe Key"
        TEXT consumer "team-order.payment"
        TIMESTAMPTZ processed_at
    }

    order_returns {
        TEXT id PK
        TEXT order_id FK "Indexed"
        TEXT buyer_id "Indexed"
        TEXT seller_id "Indexed"
        TEXT reason
        BIGINT refund_amount
        INT status "1:PENDING 2:APPROVED 3:REJECTED 4:REFUNDED"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    shipments {
        TEXT id PK
        TEXT order_id FK "Indexed"
        TEXT carrier "SPX | GHN | GHTK"
        TEXT tracking_code UK "Unique"
        INT status "1:PENDING 2:PICKED_UP 3:IN_TRANSIT 4:DELIVERED 5:FAILED"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    shipment_checkpoints {
        TEXT id PK
        TEXT shipment_id FK "Indexed"
        TIMESTAMPTZ timestamp
        TEXT location
        TEXT description
        TIMESTAMPTZ created_at
    }

    order_outbox_events {
        TEXT event_id PK "UUID"
        TEXT aggregate_type "Order"
        TEXT aggregate_id "order_id (Partition Key)"
        TEXT event_type "Proto Message Name"
        BYTEA payload "Marshalled EventEnvelope"
        TEXT request_id
        TEXT status "pending | published | failed"
        INT attempts
        TIMESTAMPTZ available_at "Indexed (Claim)"
        TIMESTAMPTZ locked_until
        TIMESTAMPTZ published_at
        TEXT error
        TIMESTAMPTZ created_at
    }
```

---

## 6. Core Business Workflows

### Workflow 1: Shopping Cart Management
Buyers manage items in their cart in real time. Product metadata (title, price, image) is snapshotted from `team-domain` upon addition.

```mermaid
sequenceDiagram
    autonumber
    actor Buyer as Buyer Client
    participant GW as team-gateway
    participant CartH as CartHandler
    participant CartSvc as CartService
    participant Domain as team-domain (:50051)
    participant Repo as PostgresCartRepository
    participant DB as PostgreSQL (order_db)

    Buyer->>GW: AddToCart(listing_id, variant_id, qty=2)
    GW->>CartH: gRPC AddToCart + x-principal-id="user_123"
    CartH->>CartSvc: AddToCart(buyerID="user_123", listingID, variantID, qty)
    CartSvc->>Domain: GetListing(id=listingID)
    Domain-->>CartSvc: Listing details (Title, Price, ImageURL, SellerID)
    CartSvc->>Repo: UpsertItem(itemSnapshot)
    Repo->>DB: INSERT INTO cart_items (...) ON CONFLICT (user_id, listing_id, variant_id) DO UPDATE SET quantity = quantity + EXCLUDED.quantity
    DB-->>Repo: Saved Item
    Repo-->>CartSvc: Saved Item
    CartSvc-->>CartH: CartItem
    CartH-->>GW: AddToCartResponse
    GW-->>Buyer: 200 OK (Item Added)
```

### Workflow 2: Distributed Purchase Saga Orchestrator
When a buyer initiates checkout, `team-order` coordinates a multi-service 2-phase saga. If any step fails, compensating transactions release all held resources on an independent background context.

```mermaid
sequenceDiagram
    autonumber
    actor Buyer as Buyer Client
    participant OrderSvc as OrderService (Saga Orchestrator)
    participant SagaDB as PostgreSQL (order_sagas & order_reservations)
    participant Promo as team-promotion (:50061)
    participant Domain as team-domain (:50051)
    participant OrderDB as PostgreSQL (orders & order_items)
    participant CartDB as PostgreSQL (cart_items)

    Buyer->>OrderSvc: CreateOrder(voucher_code="DISCOUNT10", item_ids=[...])
    OrderSvc->>SagaDB: CreateSaga(status=PENDING)
    OrderSvc->>OrderSvc: Group items by seller_id (Multi-vendor Split)

    Note over OrderSvc,Promo: Step 1: Promotion Voucher Reservation
    opt Voucher Provided
        OrderSvc->>Promo: ValidateAndReserve(code="DISCOUNT10", order_id, amount)
        alt Voucher Invalid / Exhausted
            Promo-->>OrderSvc: Error (Rejected)
            OrderSvc->>OrderSvc: Abort Saga & Return Error
        else Voucher Held
            Promo-->>OrderSvc: 200 OK (Discount: 50.000 VND)
        end
    end

    Note over OrderSvc,Domain: Step 2: Multi-Seller Inventory Reservation
    loop For Each Seller Order
        OrderSvc->>SagaDB: Persist Reservation Intent (status=PENDING, key=resID)
        OrderSvc->>Domain: ReserveStock(listing_id, variant_id, qty, reservation_id=resID)
        alt Stock Available
            Domain-->>OrderSvc: 200 OK
            OrderSvc->>SagaDB: UpdateReservationStatus(status=RESERVED)
        else Insufficient Stock
            Domain-->>OrderSvc: 409 Conflict (OutOfStock)
            OrderSvc->>SagaDB: UpdateReservationStatus(status=FAILED)
            Note over OrderSvc,Domain: Compensation Triggered (Fresh Background Context)
            OrderSvc->>Domain: ReleaseStock(for all previously RESERVED items)
            opt Voucher was held
                OrderSvc->>Promo: ReleaseReservation(order_id)
            end
            OrderSvc->>SagaDB: UpdateSagaStatus(status=COMPENSATED)
            OrderSvc-->>Buyer: 409 Conflict ("Insufficient stock")
        end
    end

    Note over OrderSvc,CartDB: Step 3: Order Persistence & Commit
    OrderSvc->>OrderDB: INSERT INTO orders & order_items (status=PENDING)
    OrderSvc->>SagaDB: CommitReservation(order_id) -> Status=COMMITTED
    OrderSvc->>SagaDB: UpdateSagaStatus(status=COMPLETED)
    OrderSvc->>CartDB: Remove checked-out items from cart_items
    OrderSvc-->>Buyer: 200 OK (Orders Created)
```

### Workflow 3: Asynchronous Payment Settlement & Outbox Publishing
Payment settlement is completely decoupled via Kafka events. The payment consumer applies events idempotently and registers outbox events atomically.

```mermaid
sequenceDiagram
    autonumber
    participant KafkaPay as Kafka (payment.events)
    participant Consumer as PaymentConsumer
    participant Dedupe as PostgreSQL (processed_events)
    participant OrderDB as PostgreSQL (orders)
    participant OutboxDB as PostgreSQL (order_outbox_events)
    participant Promo as team-promotion (:50061)
    participant Relayer as Outbox Relayer
    participant KafkaOrder as Kafka (order.events)

    KafkaPay->>Consumer: EventEnvelope(PaymentSettled { order_id, transaction_id, status=SETTLED })
    Consumer->>Dedupe: INSERT INTO processed_events (event_id, consumer) VALUES ($id, 'team-order.payment')
    alt Duplicate Event (Primary Key Conflict)
        Dedupe-->>Consumer: ErrDuplicate
        Consumer->>Consumer: Ignore (No-Op, Idempotent Return)
    else First Time Processing
        Consumer->>OrderDB: BEGIN Transaction
        Consumer->>OrderDB: UPDATE orders SET status = PAID, updated_at = now() WHERE id = $order_id
        Consumer->>OutboxDB: INSERT INTO order_outbox_events (event_id, aggregate_id, event_type='OrderPaid', payload)
        Consumer->>OrderDB: COMMIT Transaction

        opt Order had voucher
            Consumer->>Promo: CommitReservation(order_id)
        end
    end

    Note over Relayer,KafkaOrder: Background Outbox Relayer Loop
    loop Every 500ms
        Relayer->>OutboxDB: SELECT * FROM order_outbox_events WHERE status='pending' FOR UPDATE SKIP LOCKED
        Relayer->>KafkaOrder: Produce(topic="order.events", key=order_id, value=payload)
        Relayer->>OutboxDB: UPDATE order_outbox_events SET status='published', published_at=now() WHERE event_id=$id
    end
```

### Workflow 4: RMA (Return & Refund) State Machine
Buyers can request returns on paid/shipped orders. Sellers review and approve or reject the request, transitioning to refunded upon settlement.

```mermaid
stateDiagram-v2
    [*] --> PENDING : Buyer creates return request (CreateReturnRequest)
    PENDING --> APPROVED : Seller / Admin Approves (UpdateReturnStatus)
    PENDING --> REJECTED : Seller / Admin Rejects
    APPROVED --> REFUNDED : Payment refund processed (UpdateReturnStatus)
    APPROVED --> REJECTED : Dispute resolution rejected
    REJECTED --> [*]
    REFUNDED --> [*]
```

### Workflow 5: Logistics & Shipment Tracking
Sellers initiate carrier shipments which generate tracking codes and initial checkpoints.

```mermaid
sequenceDiagram
    autonumber
    actor Seller as Seller
    participant OrderH as OrderHandler
    participant OrderSvc as OrderService
    participant ShipRepo as PostgresShipmentRepository
    participant OrderRepo as PostgresOrderRepository
    participant DB as PostgreSQL (order_db)

    Seller->>OrderH: CreateShipment(order_id, carrier="SPX")
    OrderH->>OrderSvc: CreateShipment(order_id, carrier="SPX")
    OrderSvc->>OrderSvc: Generate tracking_code: SPX-VN-ORD12345-890
    OrderSvc->>ShipRepo: CreateShipment(shipment + initialCheckpoint)
    ShipRepo->>DB: INSERT INTO shipments & shipment_checkpoints
    OrderSvc->>OrderRepo: UpdateOrderStatus(order_id, status=SHIPPED, tracking_number)
    OrderRepo->>DB: UPDATE orders SET status = 3, tracking_number = $code WHERE id = $order_id
    OrderSvc-->>OrderH: Shipment (with Checkpoints)
    OrderH-->>Seller: 200 OK (Shipment Created)
```

---

## 7. gRPC Services & RPC Contracts

### A. `platform.order.v1.CartService`

| Method | Request Payload | Response Payload | Scope | Description |
|---|---|---|---|---|
| `GetCart` | `GetCartRequest {}` | `GetCartResponse { cart }` | `order.read` | Fetches active user's cart and calculated subtotal |
| `AddToCart` | `AddToCartRequest { listing_id, variant_id, quantity }` | `AddToCartResponse { item }` | `order.write` | Upserts product into cart with catalog snapshot |
| `UpdateCartItem` | `UpdateCartItemRequest { item_id, quantity }` | `UpdateCartItemResponse { item }` | `order.write` | Modifies item quantity (quantity <= 0 deletes) |
| `RemoveFromCart` | `RemoveFromCartRequest { item_id }` | `RemoveFromCartResponse {}` | `order.write` | Deletes specific item from cart |
| `ClearCart` | `ClearCartRequest {}` | `ClearCartResponse {}` | `order.write` | Clears all items in caller's cart |

### B. `platform.order.v1.OrderService`

| Method | Request Payload | Response Payload | Scope | Description |
|---|---|---|---|---|
| `CreateOrder` | `CreateOrderRequest { address_id, item_ids, payment_method, voucher_code }` | `CreateOrderResponse { orders }` | `order.write` | Executes 4-step Purchase Saga across sellers |
| `GetOrder` | `GetOrderRequest { id }` | `GetOrderResponse { order }` | `order.read` | Fetches single order details (Buyer/Seller/Admin) |
| `ListBuyerOrders`| `ListBuyerOrdersRequest { status_filter }` | `ListBuyerOrdersResponse { orders }` | `order.read` | Lists orders placed by authenticated buyer |
| `ListSellerOrders`| `ListSellerOrdersRequest { status_filter }` | `ListSellerOrdersResponse { orders }` | `order.read` | Lists orders received by authenticated seller |
| `UpdateOrderStatus`| `UpdateOrderStatusRequest { id, status, tracking_number }` | `UpdateOrderStatusResponse { order }` | `order.write` | Updates order state (PAID, SHIPPED, etc.) |
| `CancelOrder` | `CancelOrderRequest { id }` | `CancelOrderResponse { order }` | `order.write` | Cancels order and releases stock in `team-domain` |
| `CalculateShippingFee`| `CalculateShippingFeeRequest { address_id, city, items_subtotal }` | `CalculateShippingFeeResponse { fee, is_free, message }` | `order.read` | Calculates regional shipping rules & promotions |
| `GetSagaState` | `GetSagaStateRequest { order_id }` | `GetSagaStateResponse { saga_id, steps, status }` | `order.read` | Observability endpoint for saga step visualization |
| `ForceFailSaga` | `ForceFailSagaRequest { fail_at_step }` | `ForceFailSagaResponse { success }` | `admin` | Fault-injection endpoint for E2E chaos testing |
| `CreateReturnRequest` | `CreateReturnRequestRequest { order_id, reason, refund_amount }` | `CreateReturnRequestResponse { return_request }` | `order.write` | Initiates RMA return request |
| `GetReturnRequest` | `GetReturnRequestRequest { id }` | `GetReturnRequestResponse { return_request }` | `order.read` | Retrieves RMA return details |
| `UpdateReturnStatus` | `UpdateReturnStatusRequest { id, status }` | `UpdateReturnStatusResponse { return_request }` | `order.write` | Seller/Admin approves, rejects, or refunds RMA |
| `CreateShipment` | `CreateShipmentRequest { order_id, carrier, tracking_code }` | `CreateShipmentResponse { shipment }` | `order.write` | Creates shipment tracking and initial waypoint |
| `GetShipmentTracking`| `GetShipmentTrackingRequest { tracking_code, order_id, shipment_id }` | `GetShipmentTrackingResponse { shipment }` | `order.read` | Fetches tracking logs and all movement checkpoints |

---

## 8. Configuration & Environment Variables

| Variable | Type | Default | Description |
|---|---|---|---|
| `ENV` | `string` | `local` | Environment mode (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `string` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `bool` | `true` | Log format in JSON |
| `GRPC_HOST` | `string` | `0.0.0.0` | Bind host for gRPC server |
| `GRPC_PORT` | `int` | `50055` | gRPC server listening port |
| `GRPC_REFLECTION_ENABLED`| `bool` | `true` | Enable gRPC reflection |
| `SHUTDOWN_GRACE_SECONDS` | `float` | `10` | Graceful shutdown deadline |
| `DATABASE_ENABLED` | `bool` | `true` | Enable PostgreSQL database connection pool |
| `DATABASE_URL` | `string` | `postgres://postgres:postgres@localhost:5433/order_db?sslmode=disable` | Database connection DSN |
| `DB_MAX_CONNS` | `int32` | `10` | Maximum connections in `pgxpool` |
| `UPSTREAM_DOMAIN_ADDR` | `string` | `localhost:50051` | gRPC endpoint of `team-domain` for stock locking |
| `UPSTREAM_IDENTITY_ADDR`| `string` | `localhost:50053` | gRPC endpoint of `team-identity` for address lookup |
| `UPSTREAM_PROMOTION_ADDR`| `string` | `localhost:50061` | gRPC endpoint of `team-promotion` for vouchers |
| `KAFKA_BROKERS` | `string` | `localhost:9092` | Kafka broker endpoints |
| `KAFKA_PAYMENT_TOPIC` | `string` | `payment.events` | Topic to consume payment settlement events |
| `KAFKA_ORDER_TOPIC` | `string` | `order.events` | Topic to publish order status outbox events |
| `OTEL_ENABLED` | `bool` | `false` | Enable OpenTelemetry distributed tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT`| `string` | `""` | OpenTelemetry Collector endpoint (`localhost:4317`) |

---

## 9. Local Development, Migrations & Testing

### Running Locally

```bash
# 1. Start required infra from platform-core
cd ../platform-core/infra && docker compose -p platform-core up -d postgres-listing redpanda

# 2. Configure environment
cd ../../team-order
cp .env.example .env

# 3. Apply database migrations
make migrate

# 4. Run service
make run
```

### Verification via grpcurl

```bash
# View user cart
grpcurl -plaintext -H 'x-principal-id: buyer-101' -H 'x-principal-type: user' -H 'x-principal-scopes: order.read' \
  localhost:50055 platform.order.v1.CartService/GetCart

# Add product to cart
grpcurl -plaintext -H 'x-principal-id: buyer-101' -H 'x-principal-type: user' -H 'x-principal-scopes: order.write' \
  -d '{"listing_id": "listing-uuid-1", "quantity": 1}' \
  localhost:50055 platform.order.v1.CartService/AddToCart

# Execute 4-step Purchase Saga Checkout
grpcurl -plaintext -H 'x-principal-id: buyer-101' -H 'x-principal-type: user' -H 'x-principal-scopes: order.write' \
  -d '{"payment_method": 1, "voucher_code": "DISCOUNT10"}' \
  localhost:50055 platform.order.v1.OrderService/CreateOrder
```

### Quality Gate & Test Suites

```bash
make check      # Runs check-env, gofmt, go vet, and all test suites
```

Comprehensive test coverage includes:
- **`internal/config`**: Environment parsing, validation, and `.env.example` drift gates.
- **`internal/repository`**: Real Postgres and InMemory test doubles for orders, carts, sagas, outbox, returns, and shipments.
- **`internal/service`**: Saga orchestrator compensation tests, shipping rules, and RMA status transition validation.
- **`internal/consumer`**: Idempotent `PaymentSettled` event handling and deduplication ledger verification.
- **`internal/handler`**: In-process gRPC handler tests with ephemeral ports and mock principals.
