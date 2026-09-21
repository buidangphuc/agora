# team-payment — Mock Payment Gateway & Seller Wallet Ledger Microservice

`team-payment` is the core **payment processing, seller wallet ledger, and settlement** microservice in the Agora marketplace architecture. It orchestrates mock payment transactions across multiple checkout methods, executes transactional outbox event publishing for order state progression, and maintains an append-only double-entry wallet ledger for seller balance accounting and payouts.

The service strictly complies with the Agora marketplace financial safety policy (**AGENTS.md §7**): *all payment, wallet, and payout transactions are strictly MOCK and move zero real money.*

It runs on Go 1.22, exposes high-performance gRPC APIs on `:50056`, manages its dedicated PostgreSQL database `payment_db`, and produces order settlement events to Kafka (`payment.events`) via a transactional outbox relayer (ADR-0002, ADR-0009).

---

## 1. Service Overview & Responsibilities

- **Mock Payment Gateway**:
  - Handles payment transactions across multiple simulated rails: **Cash on Delivery (COD)**, **Mock MoMo QR**, **Mock Bank Transfer**, and **Mock Credit/Debit Card**.
  - Simulates instant approval or rejection scenarios and surfaces simulated payment gateway URLs (`/checkout/pay/<order_id>`).
  - Processes transaction refunds upon customer returns, order cancellations, or dispute resolutions.
- **Transactional Outbox & Event-Driven Settlement (AD4 / ADR-0009)**:
  - Eliminates dual-write hazards: on payment approval, updates `payment_transactions.status = PAID` and inserts a `PaymentSettled` event into `payment_outbox_events` in the **same database transaction**.
  - An asynchronous background **Relayer** claims pending outbox events using `FOR UPDATE SKIP LOCKED` and publishes them to Kafka `payment.events` (partitioned by `order_id`) with exponential backoff retries.
  - Replaces fragile synchronous gRPC state mutation with at-least-once, decoupled event distribution to `team-order`.
- **Append-Only Double-Entry Wallet Ledger (`wallet_ledger`)**:
  - Implements an immutable, append-only ledger for seller balance tracking: seller balance is calculated on demand as $\sum(\text{amount})$.
  - Positive entries represent credits (e.g., `ORDER_SETTLEMENT`), while negative entries represent debits or pending payout holds (`PAYOUT`, `REFUND_DEDUCTION`).
  - Completely eliminates race-prone mutable balance columns and provides an immutable audit trail.
- **Seller Payout Settlement**:
  - Validates payout requests against real-time ledger balance ($SUM(\text{amount})$).
  - Books pending debits to immediately hold funds and records bank transfer destination details (`bank_code`, `account_number`, `account_name`).

---

## 2. Technology Stack & Key Libraries

| Component / Layer | Technology / Library | Version / Details |
|---|---|---|
| **Language & Runtime** | Go | 1.22 |
| **Relational Database** | PostgreSQL / `jackc/pgx/v5` | `v5.6.0` (Connection pool with `pgxpool`) |
| **Messaging & Events** | Franz-go (`twmb/franz-go`) | `v1.18.0` (Kafka producer for `payment.events`) |
| **RPC Framework** | gRPC Go / Protobuf | `v1.66.0` / `v1.34.2` (`platform.payment.v1`) |
| **Upstream RPC Client** | gRPC Client to `team-order` | Inter-service verification on `:50055` (`GetOrder`) |
| **ID Generation** | Google UUID (`google/uuid`) | `v1.6.0` (Stable event IDs & dedupe keys) |
| **Observability** | OpenTelemetry Go (`otel`, `otelgrpc`) | `v1.28.0` (Traces exported via OTLP/gRPC `:4317`) |
| **Logging** | Structured Logger (`log/slog`) | JSON formatted structured logging |

---

## 3. System Architecture Diagram

```mermaid
flowchart TD
    subgraph Clients["Clients & Edge"]
        Gateway["team-gateway (:8080)"]
    end

    subgraph Upstream["Upstream Services"]
        TeamOrder["team-order (:50055)"]
    end

    subgraph TeamPayment["team-payment Microservice (:50056)"]
        GRPCServer["gRPC Server (:50056)"]
        AuthInterceptor["Zero-Trust Principal Interceptor"]

        subgraph Handlers["gRPC Handlers"]
            PaymentH["PaymentHandler"]
            WalletLedgerH["WalletLedgerHandler"]
        end

        subgraph CoreServices["Domain Services"]
            PaymentSvc["PaymentService"]
        end

        subgraph StorageLayer["Repositories & Outbox"]
            PaymentRepo["PaymentRepository"]
            WalletRepo["WalletRepository"]
            LedgerRepo["LedgerRepository"]
            OutboxStore["OutboxStore (pgxpool)"]
            TxWriter["PaymentTxWriter (Atomic DB Tx)"]
        end

        subgraph BackgroundWorkers["Async Background Workers"]
            RelayerWorker["Outbox Relayer Worker (SKIP LOCKED)"]
        end
    end

    subgraph Infrastructure["Persistence & Broker"]
        PostgresDB[("PostgreSQL (payment_db :5436)")]
        KafkaCluster["Redpanda / Kafka (:9092)"]
        TopicPayment["Topic: payment.events"]
    end

    Gateway -->|gRPC: CreatePayment / ProcessMockPayment| GRPCServer
    Gateway -->|gRPC: GetWalletBalance / RequestPayout| GRPCServer
    GRPCServer --> AuthInterceptor
    AuthInterceptor --> Handlers

    PaymentH --> PaymentSvc
    WalletLedgerH --> PaymentSvc

    PaymentSvc -->|gRPC: GetOrder| TeamOrder
    PaymentSvc --> PaymentRepo
    PaymentSvc --> WalletRepo
    PaymentSvc --> LedgerRepo
    PaymentSvc --> TxWriter

    TxWriter -->|1. Update Payment Status & 2. Insert Outbox Row| PostgresDB
    PaymentRepo --> PostgresDB
    WalletRepo --> PostgresDB
    LedgerRepo --> PostgresDB
    OutboxStore --> PostgresDB

    RelayerWorker -->|Claim Pending Rows FOR UPDATE SKIP LOCKED| OutboxStore
    RelayerWorker -->|Publish EventEnvelope| KafkaCluster
    KafkaCluster --- TopicPayment
    TopicPayment -.->|Consume PaymentSettled| TeamOrder
```

---

## 4. Internal Package Structure

```
team-payment/
├── cmd/
│   └── server/                 # Microservice entrypoint (cmd/server/main.go)
├── internal/
│   ├── bootstrap/              # PostgreSQL pgxpool, Kafka producer, relayer lifecycle
│   ├── config/                 # Environment variables parsing and configuration gate
│   ├── events/                 # EventEnvelope builders, Kafka publisher, outbox relayer
│   │   ├── publisher.go        # Franz-go producer for payment.events
│   │   └── relayer.go          # Polling worker claiming outbox rows with exponential backoff
│   ├── grpcserver/             # gRPC server construction & service registrations
│   ├── handler/                # gRPC service implementation
│   │   ├── payment.go          # PaymentService RPC handler (Create, Process, Refund, Payout)
│   │   └── wallet_ledger.go    # WalletLedgerService RPC handler (Balance, Ledger history)
│   ├── interceptor/            # Auth metadata extraction (x-principal-*) & tracing interceptors
│   ├── repository/             # Data access interfaces & PostgreSQL implementations
│   │   ├── payment.go          # Payment transaction CRUD & status updates
│   │   ├── ledger.go           # Append-only double-entry wallet ledger repository
│   │   ├── wallet.go           # Snapshot wallet & legacy payout request repository
│   │   ├── outbox.go           # Outbox domain types & interfaces
│   │   └── outbox_pg.go        # PostgreSQL transactional outbox implementation
│   ├── service/                # Core business logic
│   │   ├── payment.go          # Payment orchestration, settlement, and mock gateway logic
│   │   └── wallet_ledger.go    # Ledger calculations, payout balance verification, cursors
│   └── upstream/               # Upstream gRPC client adapters (team-order)
├── migrations/                 # PostgreSQL migration scripts (.up.sql / .down.sql)
├── proto/                      # Vendored protobuf definitions from platform-core
└── generated/                  # Generated Go protobuf code
```

---

## 5. Database Schema & Data Models

The service owns the `payment_db` schema in PostgreSQL with versioned migrations:

### A. Payment Transactions (`migrations/0001_initial.up.sql`)

```sql
CREATE TABLE IF NOT EXISTS payment_transactions (
    id                 VARCHAR(64) PRIMARY KEY,
    order_id           VARCHAR(64) NOT NULL,
    buyer_id           VARCHAR(64) NOT NULL,
    amount             BIGINT NOT NULL,                     -- Amount in minor units (VND)
    currency           VARCHAR(8) NOT NULL DEFAULT 'VND',
    method             INT NOT NULL DEFAULT 1,              -- 1: COD, 2: MOMO, 3: BANK, 4: CARD
    status             INT NOT NULL DEFAULT 1,              -- 1: PENDING, 2: PAID, 3: FAILED, 4: REFUNDED
    provider_reference VARCHAR(128) NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_payment_order_id ON payment_transactions (order_id);
CREATE INDEX IF NOT EXISTS idx_payment_buyer_id ON payment_transactions (buyer_id);
```

### B. Transactional Outbox Events (`migrations/0003_payment_outbox.up.sql`)

```sql
CREATE TABLE IF NOT EXISTS payment_outbox_events (
    event_id       TEXT PRIMARY KEY,                        -- Stable EventEnvelope.event_id
    aggregate_type TEXT NOT NULL,                           -- 'Payment'
    aggregate_id   TEXT NOT NULL,                           -- order_id (Kafka partition key)
    event_type     TEXT NOT NULL,                           -- 'platform.payment.v1.PaymentSettled'
    payload        BYTEA NOT NULL,                          -- Marshalled EventEnvelope proto bytes
    request_id     TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending',         -- 'pending' | 'published' | 'failed'
    attempts       INT NOT NULL DEFAULT 0,
    available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until   TIMESTAMPTZ,
    published_at   TIMESTAMPTZ,
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS payment_outbox_events_claim_idx
    ON payment_outbox_events (available_at, created_at) WHERE status = 'pending';
```

### C. Append-Only Wallet Ledger (`migrations/0004_wallet_ledger.up.sql`)

```sql
CREATE TABLE IF NOT EXISTS wallet_ledger (
    id         VARCHAR(64) PRIMARY KEY,
    seller_id  VARCHAR(64) NOT NULL,
    type       VARCHAR(48) NOT NULL,                        -- ORDER_SETTLEMENT | PAYOUT | REFUND_DEDUCTION
    amount     BIGINT NOT NULL,                             -- Signed minor units (+credit, -debit)
    status     VARCHAR(24) NOT NULL DEFAULT 'COMPLETED',    -- PENDING | COMPLETED | REJECTED
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_wallet_ledger_seller_id ON wallet_ledger (seller_id);
CREATE INDEX IF NOT EXISTS idx_wallet_ledger_seller_created ON wallet_ledger (seller_id, created_at DESC, id DESC);
```

---

## 6. Key Workflows & Settlement Mechanism

### A. Mock Payment Execution & Transactional Outbox
1. **Creation**: `CreatePayment` queries `team-order.GetOrder(order_id)` to verify the order exists and is in `ORDER_STATUS_PENDING`. It creates an idempotent `PaymentTransaction` in `PAYMENT_STATUS_PENDING`.
2. **Settlement**: When `ProcessMockPayment(simulate_success=true)` is executed:
   - Opens a PostgreSQL transaction (`SettleTx`).
   - Updates `payment_transactions.status = PAID`.
   - Marshals a `PaymentSettled` event into a `platform.events.v1.EventEnvelope` and writes it to `payment_outbox_events`.
   - Both writes commit atomically. Dual-write failure is impossible.
3. **Wallet Credit**: Best-effort credit entry (`ORDER_SETTLEMENT`, `+amount`) is appended to the seller's `wallet_ledger`.
4. **Relayer Drain**: The background `Relayer` claims pending rows via `SELECT ... FOR UPDATE SKIP LOCKED`, produces the bytes to Kafka topic `payment.events` (keyed on `order_id`), and marks the row `published`. `team-order` consumes this event to advance its order state to `ORDER_STATUS_PAID`.

```mermaid
sequenceDiagram
    autonumber
    participant Buyer as Buyer / Frontend
    participant Gateway as team-gateway
    participant Payment as team-payment (:50056)
    participant Postgres as PostgreSQL (payment_db)
    participant Relayer as Outbox Relayer
    participant Kafka as Kafka (payment.events)
    participant OrderSvc as team-order (:50055)

    Buyer->>Gateway: Submit Payment (order_id, method=MOCK_CARD)
    Gateway->>Payment: ProcessMockPayment(tx_id, simulate_success=true)
    
    rect rgb(240, 248, 255)
        Note over Payment,Postgres: Atomic Database Transaction (SettleTx)
        Payment->>Postgres: BEGIN TRANSACTION
        Payment->>Postgres: UPDATE payment_transactions SET status = PAID WHERE id = ?
        Payment->>Postgres: INSERT INTO payment_outbox_events (event_id, aggregate_id, payload, status='pending')
        Payment->>Postgres: COMMIT
    end

    Payment->>Postgres: INSERT INTO wallet_ledger (seller_id, type='ORDER_SETTLEMENT', amount=+amount)
    Payment-->>Gateway: ProcessMockPaymentResponse(Success=true)
    Gateway-->>Buyer: Payment Succeeded

    loop Asynchronous Outbox Drain
        Relayer->>Postgres: SELECT * FROM payment_outbox_events WHERE status='pending' FOR UPDATE SKIP LOCKED
        Postgres-->>Relayer: Pending Outbox Rows
        Relayer->>Kafka: Produce EventEnvelope (PaymentSettled, key=order_id)
        Kafka-->>Relayer: ACK
        Relayer->>Postgres: UPDATE payment_outbox_events SET status='published', published_at=NOW()
    end

    Kafka->>OrderSvc: Consume PaymentSettled Event
    OrderSvc->>OrderSvc: Advance Order Status -> ORDER_STATUS_PAID
```

### B. Double-Entry Wallet Ledger & Payout Settlement
- **Balance Calculation**:
  $$\text{Balance}(\text{seller\_id}) = \sum_{e \in \text{wallet\_ledger}(\text{seller\_id})} e.\text{amount}$$
- **Payout Hold**: When a seller calls `RequestWalletPayout(amount)`:
  1. Computes the real-time balance. If $\text{amount} > \text{balance}$, rejects with `ErrInsufficientBalance`.
  2. Appends a debit row to `wallet_ledger` with `type = 'PAYOUT'`, `amount = -amount`, and `status = 'PENDING'`.
  3. The seller's effective balance drops immediately, preventing race conditions on concurrent payout attempts.
- **Refund Deductions**: In dispute or cancellation cases, `RefundPayment` writes an audit deduction (`REFUND_DEDUCTION`, `-amount`) to ensure correct balance reconciliation.

---

## 7. Configuration & Environment Variables

| Variable | Type | Default | Description |
|---|---|---|---|
| `ENV` | `string` | `local` | Environment mode (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `string` | `info` | Log verbosity (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `bool` | `true` | Log format in JSON |
| `GRPC_HOST` | `string` | `0.0.0.0` | Bind host for gRPC server |
| `GRPC_PORT` | `int` | `50056` | gRPC server listening port |
| `GRPC_REFLECTION_ENABLED`| `bool` | `true` | Enable gRPC reflection |
| `SHUTDOWN_GRACE_SECONDS` | `float` | `10` | Server shutdown drain timeout |
| `DATABASE_ENABLED` | `bool` | `true` | Enable PostgreSQL connection |
| `DATABASE_URL` | `string` | `postgres://postgres:postgres@localhost:5436/payment_db?sslmode=disable` | PostgreSQL DSN |
| `DATABASE_MAX_CONNS` | `int` | `10` | Max database connections in pool |
| `UPSTREAM_ORDER_ADDR` | `string` | `localhost:50055` | gRPC upstream address for `team-order` |
| `KAFKA_ENABLED` | `bool` | `false` | Enable Kafka outbox relayer |
| `KAFKA_BROKERS` | `string` | `localhost:9092` | Comma-separated Kafka broker addresses |
| `KAFKA_PAYMENT_TOPIC` | `string` | `payment.events` | Output topic for payment events |
| `OTEL_ENABLED` | `bool` | `false` | Enable OpenTelemetry tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `string` | `""` | OTLP gRPC collector endpoint (`localhost:4317`) |
| `OTEL_SERVICE_NAME` | `string` | `team-payment` | Tracing service name |

---

## 8. How to Run & Verify

### Local Development

```bash
# 1. Start PostgreSQL (payment_db on :5436) and Redpanda from platform-core infra
cd ../platform-core/infra && docker compose -p platform-core up -d postgres-payment redpanda

# 2. Configure environment
cd ../../team-payment
cp .env.example .env

# 3. Start gRPC Payment Server (:50056)
make server
```

### Verification via grpcurl

```bash
# Create a mock payment for an order
grpcurl -plaintext -H 'x-principal-scopes: payment:write' \
  -d '{"order_id":"ord_12345","method":4}' \
  localhost:50056 platform.payment.v1.PaymentService/CreatePayment

# Process simulated mock payment
grpcurl -plaintext -H 'x-principal-scopes: payment:write' \
  -d '{"transaction_id":"tx_12345","simulate_success":true}' \
  localhost:50056 platform.payment.v1.PaymentService/ProcessMockPayment

# Check seller wallet balance
grpcurl -plaintext -H 'x-principal-id: seller_123' -H 'x-principal-scopes: payment:read' \
  -d '{"seller_id":"seller_123"}' \
  localhost:50056 platform.payment.v1.PaymentService/GetSellerWallet

# Health check
grpcurl -plaintext localhost:50056 grpc.health.v1.Health/Check
```

### Quality Gate

```bash
make check      # Runs check-env, gofmt, go vet, and unit/integration tests
```
