# team-domain — Catalog & Listing Write-Model Microservice

`team-domain` is the authoritative source-of-truth microservice for the marketplace product catalog, categories, seller storefronts, bundle deals, and inventory reservations in the Agora polyrepo e-commerce platform.

In accordance with **CQRS (ADR-0005)** and **Broker Selection (ADR-0002)**, `team-domain` acts strictly as the **write-model**. Whenever a listing is created, updated, or deleted, it executes state transitions in its dedicated PostgreSQL database (`listing_db`) and records domain events in an atomic **Transactional Outbox**. A background relayer publishes these events to Kafka (`listing.events`) wrapped in a standard `platform.events.v1.EventEnvelope` for downstream read-models (such as `team-search` via OpenSearch) to consume asynchronously.

---

## 1. Service Overview & Business Context

```
┌─────────────────┐       gRPC       ┌─────────────────┐   Transactional    ┌──────────────────────┐
│  team-gateway   │ ───────────────> │   team-domain   │ ──── Outbox ─────> │ PostgreSQL listing_db│
│     (:8080)     │  (Principal FW)  │    (:50051)     │                    └──────────────────────┘
└─────────────────┘                  └────────┬────────┘
                                              │ Background Relay
                                              ▼
                                     ┌─────────────────┐
                                     │  Kafka Broker   │
                                     │ (listing.events)│
                                     └────────┬────────┘
                                              │ Consumer (CQRS Projection)
                                              ▼
                                     ┌─────────────────┐
                                     │   team-search   │ ──> OpenSearch (:9200)
                                     │    (:50052)     │
                                     └─────────────────┘
```

### Core Responsibilities
- **Listing Lifecycle & Seller Ownership Guard**: Authoritative CRUD operations for product listings and multi-SKU variants. Enforces strict zero-trust authorization: only the authenticated listing owner (`seller_id`) or platform `admin` can mutate or delete catalog entries.
- **Hierarchical Taxonomy**: Multi-level category tree management with URL slugs, display ordering, and icon assets.
- **Seller Storefronts & Bundle Deals**: Dedicated seller shop configuration (custom slug, banner, theme, featured listings) and multi-product bundle pricing.
- **Atomic & Idempotent Stock Reservation**: High-concurrency inventory locking with SQL-level stock guards (`stock >= quantity`) and deterministic reservation keys for distributed checkout sagas orchestrated by `team-order`.
- **Automatic TTL Stock Sweeper**: Background cleanup loop that reclaims expired, uncommitted stock reservations to eliminate inventory leakage from crashed sagas.
- **Direct-to-Storage Media Pipeline**: AWS S3 / MinIO SigV4 presigned upload URL generation allowing frontend clients to upload media assets directly to object storage.
- **Transactional Outbox & Kafka Publishing**: Guaranteed at-least-once event delivery to the `listing.events` topic without distributed transaction (2PC) overhead or dual-write hazards.

---

## 2. Technology Stack & Key Libraries

| Component / Layer | Technology / Library | Version | Description |
|---|---|---|---|
| **Language Runtime** | Go | `1.22` | Core backend runtime pinned for reproducible builds |
| **RPC Framework** | `google.golang.org/grpc` | `v1.66.0` | High-performance unary and streaming gRPC server |
| **Serialization** | `google.golang.org/protobuf` | `v1.34.2` | Protocol Buffers v2 contract representation |
| **Database Driver** | `github.com/jackc/pgx/v5` | `v5.6.0` | High-performance PostgreSQL driver and connection pool (`pgxpool`) |
| **Event Streaming** | `github.com/twmb/franz-go` | `v1.18.0` | High-throughput native Go Kafka client for outbox event publishing |
| **Observability (Tracing)** | `go.opentelemetry.io/otel` | `v1.28.0` | OpenTelemetry SDK and OTLP/gRPC exporter for distributed traces |
| **gRPC Tracing** | `otelgrpc` | `v0.53.0` | Interceptor extracting trace context from W3C headers |
| **Identifiers** | `github.com/google/uuid` | `v1.6.0` | UUID v4 generation for listings, variants, and event IDs |
| **Object Storage** | AWS SDK / MinIO SigV4 | Custom SigV4 | Presigned URL generation for client-side uploads |

---

## 3. System Architecture & Component Diagram

```mermaid
flowchart TD
    subgraph Clients["Edge & Upstream Services"]
        GW["team-gateway (:8080)"]
        ORD["team-order (:50055)"]
    end

    subgraph DomainService["team-domain Microservice (:50051)"]
        subgraph TransportLayer["Transport & Interceptors"]
            AUTH["Auth Interceptor (Scope Validation)"]
            TRACE["OTel Tracing Interceptor"]
            GRPC_H["gRPC Listing & Storefront Handler"]
        end

        subgraph ServiceLayer["Domain Business Logic"]
            LS["ListingService"]
            SS["StorefrontService"]
            BS["BundleService"]
            SWEEP["Reservation Sweeper (TTL Cleanup)"]
        end

        subgraph OutboxEngine["Transactional Outbox Engine"]
            TXW["TxWriter (Atomic Commit)"]
            RELAY["Outbox Relayer (Background Poller)"]
        end

        subgraph RepoLayer["Repository Layer (PostgreSQL)"]
            PREPO["PostgresListingRepository"]
            SREPO["PostgresStorefrontRepository"]
            BREPO["PostgresBundleRepository"]
            OUTREPO["PostgresOutboxRepository"]
        end
    end

    subgraph StorageInfra["Data & Messaging Infrastructure"]
        DB[(PostgreSQL listing_db :5433)]
        KAFKA{Kafka / Redpanda :9092}
        S3[Object Storage / MinIO :9000]
    end

    GW -->|"gRPC + x-principal-* metadata"| AUTH
    ORD -->|"gRPC ReserveStock / ReleaseStock"| AUTH

    AUTH --> TRACE --> GRPC_H
    GRPC_H --> LS
    GRPC_H --> SS
    GRPC_H --> BS

    LS --> TXW
    LS --> PREPO
    SS --> SREPO
    BS --> BREPO
    SWEEP --> PREPO

    TXW -->|"Single DB Transaction"| PREPO
    TXW -->|"Single DB Transaction"| OUTREPO

    PREPO --> DB
    SREPO --> DB
    BREPO --> DB
    OUTREPO --> DB

    RELAY -->|"Claim Pending (SKIP LOCKED)"| OUTREPO
    RELAY -->|"Publish EventEnvelope"| KAFKA
    GRPC_H -.->|"Presigned SigV4 URL"| S3
```

---

## 4. Internal Package Structure

The repository follows a clean domain-driven layered architecture:

```
team-domain/
├── cmd/
│   └── server/
│       └── main.go                 # Application entrypoint & dependency injection wire-up
├── generated/                      # Buf-generated Go protobuf stubs (vendored)
│   └── platform/
│       ├── common/v1/              # Common primitives (Principal, Pagination)
│       ├── events/v1/              # EventEnvelope schemas
│       └── listing/v1/             # ListingService, Category, Storefront, Bundle contracts
├── internal/
│   ├── bootstrap/                  # Lifecycle management, pgxpool & Kafka resource initialization
│   │   ├── addon.go                # Addon initializers (Outbox Relayer, Sweeper)
│   │   ├── lifecycle.go            # Graceful shutdown coordination
│   │   └── resources.go            # Connection pool & Kafka client managers
│   ├── config/                     # Environment configuration loader with drift validation
│   │   ├── config.go               # Struct tags, defaults, and validation
│   │   └── envcheck.go             # Parity check between struct and .env.example
│   ├── events/                     # Kafka publishing and outbox polling
│   │   ├── publisher.go            # Franz-go producer wrapping EventEnvelope
│   │   └── relayer.go              # Poller claiming pending outbox events (SKIP LOCKED)
│   ├── grpcserver/                 # gRPC server setup, reflection, health checks
│   │   └── server.go               # Server construction and listener binding
│   ├── handler/                    # gRPC Transport adapters (wire protobuf <-> domain types)
│   │   ├── bundle.go               # Bundle deal RPC implementations
│   │   ├── listing.go              # Catalog, category, and inventory reservation RPCs
│   │   └── storefront.go           # Seller storefront customisation RPCs
│   ├── interceptor/                # gRPC server interceptors
│   │   ├── auth.go                 # Zero-trust x-principal-* header extractor & scope evaluator
│   │   └── tracing.go              # OpenTelemetry span injector
│   ├── observability/              # OpenTelemetry tracing setup (OTLP gRPC)
│   │   └── tracer.go               # Global tracer provider setup
│   ├── repository/                 # Database access interfaces & PostgreSQL implementations
│   │   ├── bundle.go / bundle_pg.go      # Bundle repository contract and pgx queries
│   │   ├── category.go                   # Category taxonomy queries
│   │   ├── listing.go / listing_pg.go    # Listing, variant, and stock reservation SQL
│   │   ├── outbox.go / outbox_pg.go      # Transactional outbox claim and mark queries
│   │   └── storefront.go / storefront_pg.go # Storefront persistence
│   ├── service/                    # Pure domain business rules & orchestration
│   │   ├── bundle.go               # Bundle business validation
│   │   ├── listing.go              # Ownership validation, listing operations, outbox triggers
│   │   ├── storefront.go           # Storefront slug validation
│   │   └── sweeper.go              # Background sweeper for expired stock reservations
│   └── storage/                    # S3 / MinIO Presigned URL generator
│       └── storage.go              # SigV4 signer for direct media uploads
└── migrations/                     # SQL migration scripts applied via golang-migrate
```

---

## 5. Database Schema & Tables

`team-domain` exclusively owns `listing_db` (Postgres on port `5433`). Below is the entity-relationship model:

```mermaid
erDiagram
    categories ||--o{ categories : "parent_id"
    categories ||--o{ listings : "category_id"
    listings ||--|{ listing_variants : "listing_id"
    listings ||--o{ reservations : "listing_id"
    listing_variants ||--o{ reservations : "variant_id"
    storefronts ||--|| listings : "seller_id"
    bundles ||--o{ listings : "contains listing_ids"

    listings {
        TEXT id PK
        TEXT seller_id "Indexed"
        TEXT category_id "Indexed"
        TEXT title
        TEXT description
        BIGINT price
        TEXT currency
        TEXT status "draft | published | rejected"
        TEXT[] image_keys
        INT stock "Base inventory"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    listing_variants {
        TEXT id PK
        TEXT listing_id FK "Indexed"
        TEXT name
        TEXT sku
        BIGINT price
        INT stock "Variant inventory"
        TEXT image_url
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    categories {
        TEXT id PK
        TEXT name
        TEXT slug UK "Unique"
        TEXT parent_id FK "Indexed"
        INT display_order "Indexed"
        TEXT icon_url
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    reservations {
        TEXT reservation_id PK "Idempotency Key"
        TEXT listing_id "Indexed"
        TEXT variant_id
        INT quantity
        TEXT status "active | released"
        TIMESTAMPTZ expires_at "Indexed (Sweep)"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ released_at
    }

    outbox_events {
        TEXT event_id PK "UUID"
        TEXT aggregate_type "Listing"
        TEXT aggregate_id "listing_id (Partition Key)"
        TEXT event_type "Proto Message Name"
        BYTEA payload "Marshalled EventEnvelope"
        BYTEA principal
        TEXT request_id
        TEXT status "pending | published | failed"
        INT attempts
        TIMESTAMPTZ available_at "Indexed (Claim)"
        TIMESTAMPTZ locked_until
        TIMESTAMPTZ published_at
        TEXT error
        TIMESTAMPTZ created_at
    }

    storefronts {
        TEXT seller_id PK "Owner Principal"
        TEXT slug UK "Unique Public Handle"
        JSONB config "Banner, Tagline, Theme"
        TIMESTAMPTZ updated_at
    }

    bundles {
        TEXT id PK
        TEXT seller_id "Indexed"
        TEXT title
        TEXT[] listing_ids
        BIGINT bundle_price
        TIMESTAMPTZ created_at
    }
```

### Key Table Roles
- `listings`: Primary catalog entity holding base pricing, stock, description, and seller ownership.
- `listing_variants`: SKUs under a listing (e.g. Size/Color), each having distinct stock counters and optional price overrides.
- `categories`: Self-referential hierarchical taxonomy for multi-level browsing.
- `reservations`: Durable inventory holds placed by `team-order` with TTL expiry timestamps (`expires_at`) to ensure idempotency and automatic recovery.
- `outbox_events`: Transactional outbox holding serialized `EventEnvelope` payloads committed atomically alongside domain entity changes.
- `storefronts`: Public custom shop metadata for individual sellers, identified by unique slug handles.
- `bundles`: Multi-product discounted packaging managed by sellers.

---

## 6. Core Business Workflows

### Workflow 1: Listing CRUD with Seller Ownership Guard
All write operations require caller authentication and enforce that sellers can only modify their own listings (unless the caller holds `admin` role).

```mermaid
sequenceDiagram
    autonumber
    actor Seller as Seller Client
    participant GW as team-gateway
    participant Auth as Interceptor
    participant H as ListingHandler
    participant Svc as ListingService
    participant Repo as PostgresListingRepository
    participant Outbox as PostgresOutboxRepository
    participant DB as PostgreSQL (listing_db)

    Seller->>GW: UpdateListing(Listing) + Bearer JWT
    GW->>GW: Verify RS256 JWT via JWKS
    GW->>Auth: gRPC UpdateListing + x-principal-id=seller_123, scopes=[listing.write]
    Auth->>Auth: Validate required scope 'listing.write'
    Auth->>H: Dispatch to Handler
    H->>Svc: UpdateWithEvent(listing, ownerID="seller_123", isAdmin=false)
    Svc->>Repo: Get(listing.id)
    Repo->>DB: SELECT * FROM listings WHERE id = $1
    DB-->>Repo: Return existing listing
    Repo-->>Svc: existing listing (seller_id = "seller_123")

    alt Seller ID Mismatch
        Svc-->>H: ErrForbidden (codes.PermissionDenied)
        H-->>Seller: 403 Permission Denied
    else Ownership Verified
        Svc->>Svc: Preserve immutable seller_id
        Svc->>DB: BEGIN Transaction
        Svc->>Repo: UpdateTx(listing)
        Repo->>DB: UPDATE listings ... & REPLACE listing_variants
        Svc->>Outbox: EnqueueTx(EventEnvelope(ListingChanged:UPDATED))
        Outbox->>DB: INSERT INTO outbox_events ...
        Svc->>DB: COMMIT Transaction
        Svc-->>H: Updated Listing
        H-->>GW: UpdateListingResponse
        GW-->>Seller: 200 OK (Updated Listing)
    end
```

### Workflow 2: Atomic Stock Reservation & Automatic TTL Expiry Sweeper
During checkout, `team-order` calls `ReserveStock` with a deterministic `reservation_id`. Stock decrement is atomic at the database level. If a checkout saga crashes before completing, the background sweeper reclaims expired stock.

```mermaid
sequenceDiagram
    autonumber
    participant Order as team-order (Saga)
    participant Domain as team-domain (:50051)
    participant DB as PostgreSQL (listing_db)
    participant Sweeper as Background Sweeper

    Note over Order,Domain: Step 1: Idempotent Stock Reservation
    Order->>Domain: ReserveStock(listing_id, variant_id, qty=2, reservation_id="res-abc")
    Domain->>DB: BEGIN Transaction
    Domain->>DB: SELECT EXISTS(SELECT 1 FROM reservations WHERE reservation_id="res-abc")
    alt Already Reserved (Idempotent Retry)
        DB-->>Domain: exists = true
        Domain->>DB: ROLLBACK (No-Op)
        Domain-->>Order: 200 OK (Stock already held)
    else First Reservation Attempt
        DB-->>Domain: exists = false
        Domain->>DB: UPDATE listings SET stock = stock - 2 WHERE id = $1 AND stock >= 2
        alt Stock Available (Rows Affected = 1)
            Domain->>DB: INSERT INTO reservations (reservation_id, listing_id, qty, status='active', expires_at=now() + 15m)
            Domain->>DB: COMMIT Transaction
            Domain-->>Order: 200 OK (Reserved)
        else Insufficient Stock (Rows Affected = 0)
            Domain->>DB: ROLLBACK Transaction
            Domain-->>Order: 409 Conflict (ErrOutOfStock)
        end
    end

    Note over Sweeper,DB: Step 2: Background TTL Expiry Recovery
    loop Every 1 Minute
        Sweeper->>DB: BEGIN Transaction
        Sweeper->>DB: SELECT * FROM reservations WHERE status = 'active' AND expires_at < now() FOR UPDATE SKIP LOCKED
        Sweeper->>DB: UPDATE listings SET stock = stock + res.quantity WHERE id = res.listing_id
        Sweeper->>DB: UPDATE reservations SET status = 'released', released_at = now() WHERE reservation_id = res.id
        Sweeper->>DB: COMMIT Transaction
    end
```

### Workflow 3: Transactional Outbox Pattern & Background Relayer
To guarantee consistency without distributed 2PC transactions, business entity changes and outbox event insertions share a single database transaction. The background relayer continuously polls and publishes pending events to Kafka.

```mermaid
sequenceDiagram
    autonumber
    participant Svc as ListingService
    participant DB as PostgreSQL (outbox_events)
    participant Relayer as Outbox Relayer
    participant Kafka as Kafka (listing.events)
    participant Search as team-search

    Note over Svc,DB: 1. Atomic Write & Enqueue
    Svc->>DB: BEGIN Transaction
    Svc->>DB: UPDATE listings / INSERT listings ...
    Svc->>DB: INSERT INTO outbox_events (event_id, aggregate_id, payload, status='pending', available_at=now())
    Svc->>DB: COMMIT Transaction

    Note over Relayer,Kafka: 2. Asynchronous Polling & Production
    loop Every 1 Second
        Relayer->>DB: SELECT * FROM outbox_events WHERE status = 'pending' AND available_at <= now() ORDER BY available_at FOR UPDATE SKIP LOCKED LIMIT 100
        Relayer->>Kafka: Produce(topic="listing.events", key=listing_id, value=payload)
        alt Produce Succeeded
            Relayer->>DB: UPDATE outbox_events SET status = 'published', published_at = now() WHERE event_id = $1
        else Produce Failed
            Relayer->>DB: UPDATE outbox_events SET attempts = attempts + 1, available_at = now() + backoff, error = $err WHERE event_id = $1
        end
    end

    Note over Kafka,Search: 3. Downstream Read-Model Projection
    Kafka->>Search: Consume EventEnvelope (ListingChanged)
    Search->>Search: Index document into OpenSearch (:9200)
```

---

## 7. gRPC Services & RPC Contracts

### Service Definition: `platform.listing.v1.ListingService`

| Method | Request Payload | Response Payload | Required Scope | Description |
|---|---|---|---|---|
| `GetListing` | `GetListingRequest { id }` | `GetListingResponse { listing }` | `listing.read` | Retrieve listing by ID along with its variants |
| `ListListings` | `ListListingsRequest { page, status }` | `ListListingsResponse { listings, page }` | `listing.read` | Keyset-paginated listings filtered by status |
| `ListMyListings` | `ListMyListingsRequest { page }` | `ListMyListingsResponse { listings, page }` | `listing.write` | Listings owned exclusively by calling seller |
| `CreateListing` | `CreateListingRequest { listing }` | `CreateListingResponse { listing }` | `listing.write` | Create listing, assigns `seller_id`, writes outbox event |
| `UpdateListing` | `UpdateListingRequest { listing }` | `UpdateListingResponse { listing }` | `listing.write` | Updates listing & variants, enforces ownership |
| `DeleteListing` | `DeleteListingRequest { id }` | `DeleteListingResponse {}` | `listing.write` | Soft/hard deletes listing, enforces ownership |
| `GetImageUploadUrl` | `GetImageUploadUrlRequest { content_type, filename }` | `GetImageUploadUrlResponse { upload_url, image_key, public_url }` | `listing.write` | Generates 15-min S3 SigV4 presigned upload URL |
| `ListCategories` | `ListCategoriesRequest { parent_id }` | `ListCategoriesResponse { categories }` | `listing.read` | Lists categories, optionally filtered by `parent_id` |
| `GetCategory` | `GetCategoryRequest { id }` | `GetCategoryResponse { category }` | `listing.read` | Retrieves single category by ID |
| `ReserveStock` | `ReserveStockRequest { listing_id, variant_id, quantity, reservation_id }` | `ReserveStockResponse { success, message }` | *Internal RPC* | Atomically decrements stock if `stock >= quantity` |
| `ReleaseStock` | `ReleaseStockRequest { listing_id, variant_id, quantity, reservation_id }` | `ReleaseStockResponse { success }` | *Internal RPC* | Re-increments stock on cancelled orders / rollbacks |
| `UpsertStorefront` | `UpsertStorefrontRequest { storefront }` | `UpsertStorefrontResponse { storefront }` | `listing.write` | Creates or updates caller's shop profile |
| `GetStorefront` | `GetStorefrontRequest { seller_id, slug }` | `GetStorefrontResponse { storefront }` | `listing.read` | Fetches storefront by seller ID or public slug |
| `CreateBundle` | `CreateBundleRequest { bundle }` | `CreateBundleResponse { bundle }` | `listing.write` | Creates a bundled deal of seller's listings |
| `ListBundlesBySeller`| `ListBundlesBySellerRequest { seller_id }` | `ListBundlesBySellerResponse { bundles }` | `listing.read` | Lists all bundles created by a seller |

---

## 8. Domain Events (Kafka)

All events are published to topic `listing.events` keyed by `listing_id` (guaranteeing in-order per-listing partition processing), wrapped in `platform.events.v1.EventEnvelope`:

```protobuf
message EventEnvelope {
  string event_id = 1;                     // Unique UUID for deduplication
  string type = 2;                         // Fully qualified proto type name
  google.protobuf.Timestamp occurred_at = 3;
  platform.common.v1.Principal principal = 4; // Calling user/seller context
  string traceparent = 5;                  // W3C trace context
  string request_id = 6;                   // Correlation ID
  bytes payload = 7;                       // Serialized proto event message
}
```

### Event Payloads
1. `platform.listing.v1.ListingChanged`: Full listing snapshot with `change_type` (`CREATED`, `UPDATED`, `DELETED`).
2. `platform.listing.v1.ListingBaseInfoChanged`: Metadata changes (title, description, category, images, status).
3. `platform.listing.v1.ListingPricingChanged`: Price & promotional price updates.
4. `platform.listing.v1.ListingStockChanged`: Stock inventory level updates across base listing & variants.
5. `platform.listing.v1.ListingStatusChanged`: Status transitions (`DRAFT`, `PUBLISHED`, `REJECTED`).

---

## 9. Configuration & Environment Variables

| Variable | Type | Default | Description |
|---|---|---|---|
| `ENV` | `string` | `local` | Environment mode (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `string` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `bool` | `true` | Log format in JSON |
| `GRPC_HOST` | `string` | `0.0.0.0` | Bind host for gRPC server |
| `GRPC_PORT` | `int` | `50051` | gRPC server listening port |
| `GRPC_REFLECTION_ENABLED` | `bool` | `true` | Enable gRPC reflection for development/debugging |
| `SHUTDOWN_GRACE_SECONDS` | `float` | `10` | Graceful shutdown timeout |
| `DATABASE_ENABLED` | `bool` | `true` | Enable PostgreSQL database pool |
| `DATABASE_URL` | `string` | `""` | Connection string (`postgresql://listing_svc:listing_pass@localhost:5433/listing_db`) |
| `DB_MAX_CONNS` | `int32` | `10` | Maximum connections in `pgxpool` |
| `KAFKA_ENABLED` | `bool` | `false` | Enable Kafka event publisher & outbox relayer |
| `KAFKA_BROKERS` | `string` | `localhost:9092` | Comma-separated Kafka broker addresses |
| `KAFKA_LISTING_TOPIC` | `string` | `listing.events` | Topic for listing domain events |
| `STORAGE_ENDPOINT` | `string` | `localhost:9000` | S3 / MinIO endpoint |
| `STORAGE_BUCKET` | `string` | `marketplace-listings` | Target bucket for product media |
| `STORAGE_ACCESS_KEY` | `string` | `""` | S3 Access Key ID |
| `STORAGE_SECRET_KEY` | `string` | `""` | S3 Secret Access Key |
| `STORAGE_REGION` | `string` | `us-east-1` | S3 Region |
| `STORAGE_USE_SSL` | `bool` | `false` | Use HTTPS for S3 |
| `STORAGE_PUBLIC_BASE_URL` | `string` | `""` | Public CDN/Gateway base URL for image retrieval |
| `OTEL_ENABLED` | `bool` | `false` | Enable OpenTelemetry tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `string` | `""` | OTLP gRPC endpoint (`localhost:4317`) |
| `OTEL_SERVICE_NAME` | `string` | `team-domain` | Tracing service name |

---

## 10. Local Development, Migrations & Testing

### Running with Docker Infra

```bash
# 1. Start postgres-listing and redpanda from platform-core
cd ../platform-core/infra && docker compose -p platform-core up -d postgres-listing redpanda

# 2. Configure environment
cd ../../team-domain
cp .env.example .env

# 3. Apply database migrations
make migrate

# 4. Run service locally
make run
```

### Verification via grpcurl

```bash
# List categories (listing.read scope)
grpcurl -plaintext -H 'x-principal-id: user-1' -H 'x-principal-type: user' -H 'x-principal-scopes: listing.read' \
  localhost:50051 platform.listing.v1.ListingService/ListCategories

# Create listing (listing.write scope)
grpcurl -plaintext -H 'x-principal-id: seller-1' -H 'x-principal-type: user' -H 'x-principal-scopes: listing.write' \
  -d '{"listing": {"title": "MacBook Pro M3 Max", "description": "16-inch 36GB 1TB", "price": 85000000, "currency": "VND", "status": "LISTING_STATUS_PUBLISHED", "category_id": "cat-electronics", "stock": 10}}' \
  localhost:50051 platform.listing.v1.ListingService/CreateListing

# Idempotent Stock Reservation
grpcurl -plaintext \
  -d '{"listing_id": "<LISTING_ID>", "quantity": 1, "reservation_id": "res-checkout-1001"}' \
  localhost:50051 platform.listing.v1.ListingService/ReserveStock
```

### Quality Gate

```bash
make check      # Runs env drift check, gofmt, go vet, and all test suites
```
