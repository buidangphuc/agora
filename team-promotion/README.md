# team-promotion — Voucher, Flash-Sale & Promotion Microservice

`team-promotion` is the core **promotions and marketing engine** microservice in the Agora marketplace architecture. It manages shop/platform **vouchers**, time-limited **flash-sale campaigns**, seller **subscription tiers**, and **sponsored ad placements**.

The service operates on the strict financial boundary rule (**AGENTS.md §7**): *team-promotion performs pure discount calculations and quota management — it never moves money.*

It runs on Go 1.22, provides high-throughput gRPC APIs (`:50061`), owns an isolated PostgreSQL database `promotion_db`, integrates OpenFeature/Flipt for dynamic kill-switches, and publishes state changes asynchronously to the Kafka topic `promotion.events` (ADR-0002).

---

## 1. Service Overview & Responsibilities

- **Voucher Lifecycle & Rule Evaluation**:
  - Manages **Platform-wide** (all shops) and **Shop-scoped** (specific seller) vouchers.
  - Supports percentage discounts (with optional `max_discount` caps) and fixed amount discounts with `min_spend` thresholds.
  - Enforces time validity windows (`starts_at`, `ends_at`) and global redemption quotas.
- **Two-Phase Voucher Reservation (Saga-Ready)**:
  - Backs checkout flows with idempotent redemption holds (`ValidateAndReserve` $\rightarrow$ `CommitReservation` / `ReleaseReservation`).
  - Idempotent by `reservation_id` (mirrors inventory reservation ADR-0008), preventing double redemptions during network retries or distributed checkout sagas.
- **Flash-Sale Campaigns**:
  - Manages time-bounded promotional flash-sales with dedicated stock allocations (`stock_cap`, `stock_sold`).
  - Provides active campaign lookups and live remaining stock calculations for storefront meters and banners.
  - Guarded by a dynamic OpenFeature kill-switch (`FlagFlashSaleEnabled`) with fail-open behavior.
- **Seller Subscriptions & Sponsored Ads**:
  - Manages seller shop upgrade tiers (Free, Pro, Premium) and entitlement limits.
  - Provides sponsored product placement bidding and ranking for search/category promotion slots.
- **Event-Driven Integration**: Emits `VoucherChanged` and `FlashSaleChanged` events wrapped in `platform.events.v1.EventEnvelope` to Kafka `promotion.events`.

---

## 2. Technology Stack & Key Libraries

| Component / Layer | Technology / Library | Version / Details |
|---|---|---|
| **Language & Runtime** | Go | 1.22 |
| **Relational Database** | PostgreSQL / `jackc/pgx/v5` | `v5.6.0` (Connection pool with `pgxpool`) |
| **Feature Flagging** | OpenFeature Go SDK / Flipt Provider | `open-feature/go-sdk v1.14.0`, `flipt-openfeature-provider v0.2.0` |
| **Messaging & Events** | Franz-go (`twmb/franz-go`) | `v1.18.0` (Kafka producer for `promotion.events`) |
| **RPC Framework** | gRPC Go / Protobuf | `v1.66.0` / `v1.34.2` (`platform.promotion.v1`) |
| **Observability** | OpenTelemetry Go (`otel`, `otelgrpc`) | `v1.28.0` (Traces exported via OTLP/gRPC `:4317`) |
| **Logging** | Structured Logger (`log/slog`) | JSON formatted structured logging |

---

## 3. System Architecture Diagram

```mermaid
flowchart TD
    subgraph Clients["Clients & Edge"]
        Gateway["team-gateway (:8080)"]
        OrderSaga["team-order (:50055 / Checkout Saga)"]
    end

    subgraph TeamPromotion["team-promotion Microservice (:50061)"]
        GRPCServer["gRPC Server (:50061)"]
        AuthInterceptor["Zero-Trust Principal Interceptor"]

        subgraph Handlers["gRPC Handlers"]
            VoucherH["VoucherHandler"]
            FlashSaleH["FlashSaleHandler"]
            SubH["SubscriptionHandler"]
            SponH["SponsoredHandler"]
        end

        subgraph Services["Core Domain Services"]
            VoucherSvc["VoucherService"]
            FlashSaleSvc["FlashSaleService"]
            SubSvc["SubscriptionService"]
            SponSvc["SponsoredService"]
        end

        subgraph Repositories["Data Repositories (Postgres / InMemory)"]
            VoucherRepo["VoucherRepo"]
            ResRepo["ReservationRepo"]
            FlashSaleRepo["FlashSaleRepo"]
            SubRepo["SubscriptionRepo"]
            SponRepo["SponsoredRepo"]
        end

        Emitter["Event Emitter (best-effort)"]
        Flags["OpenFeature / Flipt Provider"]
    end

    subgraph Infrastructure["Persistence & Broker"]
        PostgresDB[("PostgreSQL (promotion_db :5440)")]
        KafkaCluster["Redpanda / Kafka (:9092)"]
        TopicEvents["Topic: promotion.events"]
        FliptServer["Flipt Service (:8080 / :9000)"]
    end

    Gateway -->|gRPC: CRUD & Storefront Queries| GRPCServer
    OrderSaga -->|gRPC: ValidateAndReserve / Commit / Release| GRPCServer
    GRPCServer --> AuthInterceptor
    AuthInterceptor --> Handlers

    VoucherH --> VoucherSvc
    FlashSaleH --> FlashSaleSvc
    SubH --> SubSvc
    SponH --> SponSvc

    VoucherSvc --> VoucherRepo
    VoucherSvc --> ResRepo
    FlashSaleSvc --> FlashSaleRepo
    FlashSaleSvc --> Flags
    SubSvc --> SubRepo
    SponSvc --> SponRepo

    VoucherSvc --> Emitter
    FlashSaleSvc --> Emitter

    VoucherRepo --> PostgresDB
    ResRepo --> PostgresDB
    FlashSaleRepo --> PostgresDB
    SubRepo --> PostgresDB
    SponRepo --> PostgresDB

    Flags -.->|Evaluate Flag| FliptServer
    Emitter -->|Publish EventEnvelope| KafkaCluster
    KafkaCluster --- TopicEvents
```

---

## 4. Internal Package Structure

```
team-promotion/
├── cmd/
│   └── server/                 # Microservice entrypoint (cmd/server/main.go)
├── internal/
│   ├── bootstrap/              # Postgres pgxpool, Kafka producer, OpenFeature lifecycles
│   ├── config/                 # Environment variables parsing and configuration gate
│   ├── featureflags/           # OpenFeature client provider & Flipt integration
│   ├── grpcserver/             # gRPC server construction & service registrations
│   ├── handler/                # gRPC service implementation
│   │   ├── voucher.go          # VoucherService RPC handler (CRUD + 2-phase reservations)
│   │   ├── flashsale.go        # FlashSaleService RPC handler
│   │   ├── subscription.go     # SubscriptionService RPC handler
│   │   └── sponsored.go        # SponsoredService RPC handler
│   ├── interceptor/            # Auth metadata extraction (x-principal-*) & tracing interceptors
│   ├── producer/               # Kafka publisher & envelope emitter for promotion.events
│   ├── repository/             # Data access interfaces & PostgreSQL implementations
│   │   ├── voucher.go          # Voucher CRUD & atomic quota decrement queries
│   │   ├── reservation.go      # Idempotent reservation queries & status updates
│   │   ├── flashsale.go        # Flash sale campaign queries & active window filters
│   │   ├── subscription.go     # Seller subscription tiers & plan repository
│   │   └── sponsored.go        # Sponsored ad campaigns & bid ranking repository
│   └── service/                # Core business logic
│       ├── voucher.go          # Discount calculation math & 2-phase reservation logic
│       ├── flashsale.go        # Campaign lifecycle, active lookups & stock meters
│       ├── subscription.go     # Tier entitlement checks
│       └── sponsored.go        # Slot ranking & bid auction logic
├── migrations/                 # PostgreSQL migration scripts (.up.sql / .down.sql)
├── proto/                      # Vendored protobuf definitions from platform-core
└── generated/                  # Generated Go protobuf code
```

---

## 5. Database Schema & Data Models

The service owns the `promotion_db` schema in PostgreSQL with versioned migrations:

### A. Vouchers & Reservations (`migrations/0001_promotion.up.sql`)

```sql
CREATE TABLE IF NOT EXISTS vouchers (
    id             TEXT PRIMARY KEY,
    code           TEXT NOT NULL UNIQUE,
    scope          INT NOT NULL DEFAULT 0,      -- 1: Shop, 2: Platform
    seller_id      TEXT NOT NULL DEFAULT '',    -- Populated for shop-scoped vouchers
    discount_type  INT NOT NULL DEFAULT 0,      -- 1: Percent, 2: Fixed
    discount_value BIGINT NOT NULL DEFAULT 0,   -- Percent (1-100) or minor units
    min_spend      BIGINT NOT NULL DEFAULT 0,   -- Minimum subtotal required
    max_discount   BIGINT NOT NULL DEFAULT 0,   -- Max cap for percentage discounts (0 = uncapped)
    quota          BIGINT NOT NULL DEFAULT 0,   -- Max total redemptions (0 = unlimited)
    used           BIGINT NOT NULL DEFAULT 0,   -- Redeemed count
    starts_at      TIMESTAMPTZ,
    ends_at        TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS voucher_reservations (
    id              TEXT PRIMARY KEY,
    reservation_id  TEXT NOT NULL UNIQUE,       -- Caller saga ID for idempotency
    voucher_id      TEXT NOT NULL REFERENCES vouchers(id) ON DELETE CASCADE,
    buyer_id        TEXT NOT NULL,
    discount_amount BIGINT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'reserved', -- 'reserved' | 'committed' | 'released'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### B. Flash-Sale Campaigns (`migrations/0001_promotion.up.sql`)

```sql
CREATE TABLE IF NOT EXISTS flash_sale_campaigns (
    id          TEXT PRIMARY KEY,
    listing_id  TEXT NOT NULL,
    variant_id  TEXT NOT NULL DEFAULT '',
    sale_price  BIGINT NOT NULL,               -- Promotional sale price in minor units
    stock_cap   BIGINT NOT NULL DEFAULT 0,      -- Total items allocated for flash-sale
    stock_sold  BIGINT NOT NULL DEFAULT 0,      -- Total items sold under promotional price
    starts_at   TIMESTAMPTZ,
    ends_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### C. Subscriptions & Sponsored Ads (`migrations/0002_subscriptions.up.sql`, `0003_sponsored.up.sql`)
- `subscription_plans`: Plan reference tiers (`plan_free`, `plan_pro`, `plan_premium`), pricing, and entitlement limits.
- `seller_subscriptions`: One active tier subscription record per seller.
- `ad_campaigns`: Seller pay-per-slot ad placements ranked by `bid DESC` for active slots.

---

## 6. Key Workflows & Business Rules

### A. Voucher Eligibility & Discount Calculation
When a buyer applies a voucher code, `ValidateAndReserve` evaluates the following rules:
1. **Time Window**: $T_{\text{now}} \ge \text{starts\_at}$ and $T_{\text{now}} \le \text{ends\_at}$.
2. **Minimum Spend**: $\text{cart\_subtotal} \ge \text{min\_spend}$.
3. **Quota Availability**: If $\text{quota} > 0$, requires $\text{used} < \text{quota}$.
4. **Seller Scope**: If `scope == VOUCHER_SCOPE_SHOP`, requires `voucher.seller_id == cart.seller_id`.
5. **Discount Arithmetic**:
   - **Percent Discount**:
     $$\text{discount} = \min\left(\lfloor \frac{\text{subtotal} \times \text{discount\_value}}{100} \rfloor, \, \text{max\_discount} > 0 \text{ ? } \text{max\_discount} : \infty\right)$$
   - **Fixed Amount Discount**:
     $$\text{discount} = \min(\text{discount\_value}, \, \text{subtotal})$$
   - Clamped to ensure $0 \le \text{discount} \le \text{subtotal}$.

### B. Two-Phase Voucher Reservation (Distributed Checkout Saga)
To prevent race conditions, overselling voucher quotas, and double redemption during checkout retries:

```mermaid
sequenceDiagram
    autonumber
    participant OrderSaga as team-order (Checkout Saga)
    participant Promotion as team-promotion (:50061)
    participant Postgres as PostgreSQL (promotion_db)
    participant Kafka as Kafka (promotion.events)

    Note over OrderSaga,Promotion: Phase 1: Validate & Place Hold
    OrderSaga->>Promotion: ValidateAndReserve(reservation_id, code, buyer_id, subtotal, seller_id)
    alt Reservation ID already exists (Retry)
        Promotion->>Postgres: SELECT * FROM voucher_reservations WHERE reservation_id = ?
        Postgres-->>Promotion: Existing Hold
        Promotion-->>OrderSaga: ReserveResult(Valid=true, DiscountAmount, VoucherID)
    else New Reservation
        Promotion->>Postgres: SELECT * FROM vouchers WHERE code = ?
        Promotion->>Promotion: Validate Rules & Compute Discount
        Promotion->>Postgres: INSERT INTO voucher_reservations (reservation_id, voucher_id, status='reserved')
        Postgres-->>Promotion: OK
        Promotion-->>OrderSaga: ReserveResult(Valid=true, DiscountAmount, VoucherID)
    end

    Note over OrderSaga,Promotion: Phase 2a: Checkout Success (Commit)
    OrderSaga->>Promotion: CommitReservation(reservation_id)
    Promotion->>Postgres: UPDATE voucher_reservations SET status='committed' WHERE reservation_id = ?
    Promotion->>Postgres: UPDATE vouchers SET used = used + 1 WHERE id = ?
    Promotion-->>OrderSaga: CommitResponse(committed=true)

    Note over OrderSaga,Promotion: Phase 2b: Checkout Failure / Rollback (Release)
    OrderSaga->>Promotion: ReleaseReservation(reservation_id)
    Promotion->>Postgres: UPDATE voucher_reservations SET status='released' WHERE reservation_id = ?
    Promotion-->>OrderSaga: ReleaseResponse(released=true)

    opt State Change Emission
        Promotion->>Kafka: Publish VoucherChanged EventEnvelope
    end
```

### C. Flash-Sale Campaigns & Stock Meters
- **Active Window Check**: Fetches campaigns where `starts_at <= now <= ends_at` for a target listing.
- **Stock Metering**: Live available promotional units are calculated as $\max(0, \, \text{stock\_cap} - \text{stock\_sold})$.
- **Kill-Switch**: Evaluates `FlagFlashSaleEnabled` via OpenFeature. If disabled by operations or if Flipt is unreachable, it fails open cleanly without causing downtime.

---

## 7. Configuration & Environment Variables

| Variable | Type | Default | Description |
|---|---|---|---|
| `ENV` | `string` | `local` | Environment mode (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `string` | `info` | Log verbosity (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `bool` | `true` | Log format in JSON |
| `GRPC_HOST` | `string` | `0.0.0.0` | Bind host for gRPC server |
| `GRPC_PORT` | `int` | `50061` | gRPC server listening port |
| `GRPC_REFLECTION_ENABLED` | `bool` | `true` | Enable gRPC reflection |
| `SHUTDOWN_GRACE_SECONDS` | `float` | `10` | Server shutdown drain timeout |
| `DATABASE_ENABLED` | `bool` | `true` | Enable PostgreSQL connection |
| `DATABASE_URL` | `string` | `postgres://postgres:postgres@localhost:5440/promotion_db?sslmode=disable` | PostgreSQL DSN |
| `DATABASE_MAX_CONNS` | `int` | `10` | Max database connections in pool |
| `KAFKA_ENABLED` | `bool` | `false` | Enable Kafka event publisher |
| `KAFKA_BROKERS` | `string` | `localhost:9092` | Comma-separated Kafka broker addresses |
| `KAFKA_PROMOTION_TOPIC` | `string` | `promotion.events` | Output topic for promotion events |
| `FLIPT_ENABLED` | `bool` | `false` | Enable Flipt feature flag provider |
| `FLIPT_URL` | `string` | `http://localhost:8080` | Flipt service endpoint |
| `OTEL_ENABLED` | `bool` | `false` | Enable OpenTelemetry tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `string` | `""` | OTLP gRPC collector endpoint (`localhost:4317`) |
| `OTEL_SERVICE_NAME` | `string` | `team-promotion` | Tracing service name |

---

## 8. How to Run & Verify

### Local Development

```bash
# 1. Start PostgreSQL (promotion_db on :5440) and Redpanda from platform-core infra
cd ../platform-core/infra && docker compose -p platform-core up -d postgres-promotion redpanda

# 2. Configure environment
cd ../../team-promotion
cp .env.example .env

# 3. Build & start gRPC Promotion Server (:50061)
make server
```

### Verification via grpcurl

```bash
# Get Voucher by code
grpcurl -plaintext -H 'x-principal-scopes: promotion:read' \
  -d '{"code":"SUMMER2026"}' \
  localhost:50061 platform.promotion.v1.VoucherService/GetVoucher

# Validate and Reserve Voucher
grpcurl -plaintext -H 'x-principal-scopes: promotion:write' \
  -d '{"reservation_id":"res_12345","code":"SUMMER2026","buyer_id":"buyer_1","cart_subtotal":500000}' \
  localhost:50061 platform.promotion.v1.VoucherService/ValidateAndReserve

# Health check
grpcurl -plaintext localhost:50061 grpc.health.v1.Health/Check
```

### Quality Gate

```bash
make check      # Runs check-env, gofmt, go vet, and unit/integration tests
```

