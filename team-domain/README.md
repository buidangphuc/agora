# team-domain — Catalog & Listing Write-Model Microservice

`team-domain` is the authoritative source-of-truth service for the marketplace product catalog. It manages product listings, multi-level category taxonomy, inventory stock reservations (for order checkout), and presigned S3/MinIO media upload URLs.

In accordance with **CQRS (ADR-0005)** and **Broker Selection (ADR-0002)**, `team-domain` is strictly the write-model. Whenever a listing is created, updated, or deleted, it publishes domain events to Kafka (`listing.events`) wrapped in a standard `platform.events.v1.EventEnvelope` so downstream read-models (like `team-search`) can project them into OpenSearch.

---

## 1. Responsibilities & Architecture

- **Listing Management & Ownership Enforcement**: Create, update, soft-filter, and delete listings. Enforces that only the owning seller (or an `admin`) can modify or delete listings.
- **Category Taxonomy**: Product hierarchical categories with slugs, display orders, and icon metadata.
- **Atomic Stock Reservation & Release**: High-concurrency inventory reservation with SQL-level stock guard (`stock >= quantity`), called during Saga checkouts by `team-order`.
- **Media Upload Pipeline**: Generates AWS S3 / MinIO SigV4 presigned `PUT` URLs so frontend clients upload images directly to object storage.
- **Kafka Event Publishing**: Emits fine-grained & monolithic domain events on topic `listing.events` keyed by `listing_id` for in-order partition processing.
- **Database Ownership (Rule 3)**: Exclusively owns `listing_db` (Postgres port `5433`).

---

## 2. Configuration & Environment Variables

| Variable | Type | Default | Description |
|---|---|---|---|
| `ENV` | `string` | `local` | Environment mode (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `string` | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `bool` | `true` | Log format in JSON |
| `GRPC_HOST` | `string` | `0.0.0.0` | Bind host for gRPC server |
| `GRPC_PORT` | `int` | `50051` | gRPC server listening port |
| `GRPC_REFLECTION_ENABLED` | `bool` | `true` | Enable gRPC reflection |
| `SHUTDOWN_GRACE_SECONDS` | `float` | `10` | Server shutdown grace period |
| `DATABASE_ENABLED` | `bool` | `true` | Enable Postgres database pool |
| `DATABASE_URL` | `string` | `""` | Connection string (`postgresql://listing_svc:listing_pass@localhost:5433/listing_db`) |
| `DB_MAX_CONNS` | `int32` | `10` | Maximum connections in pgxpool |
| `KAFKA_ENABLED` | `bool` | `false` | Enable Kafka event publishing |
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

## 3. Database Schema & Migrations

Database migrations are located in `migrations/` and applied using `golang-migrate`.

### `0001_init.up.sql`
Initial `listings` table holding base listing details.
```sql
CREATE TABLE IF NOT EXISTS listings (
    id          TEXT PRIMARY KEY,
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    price       BIGINT      NOT NULL,
    currency    TEXT        NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'draft',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS listings_status_idx ON listings (status);
```

### `0002_seller.up.sql`
Adds owner binding for the seller center and authorization checks.
```sql
ALTER TABLE listings ADD COLUMN IF NOT EXISTS seller_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS listings_seller_idx ON listings (seller_id);
```

### `0003_images.up.sql`
Stores object storage keys as an array.
```sql
ALTER TABLE listings ADD COLUMN IF NOT EXISTS image_keys TEXT[] NOT NULL DEFAULT '{}';
```

### `0004_categories.up.sql`
Creates category hierarchy and tags listings with `category_id`.
```sql
CREATE TABLE IF NOT EXISTS categories (
    id            TEXT PRIMARY KEY,
    name          TEXT        NOT NULL,
    slug          TEXT        NOT NULL UNIQUE,
    parent_id     TEXT        REFERENCES categories(id) ON DELETE SET NULL,
    display_order INT         NOT NULL DEFAULT 0,
    icon_url      TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS categories_parent_idx ON categories (parent_id);
CREATE INDEX IF NOT EXISTS categories_order_idx ON categories (display_order);

ALTER TABLE listings ADD COLUMN IF NOT EXISTS category_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS listings_category_idx ON listings (category_id);
```

### `0005_variants.up.sql`
Adds inventory stock column and multi-variant SKUs table.
```sql
ALTER TABLE listings ADD COLUMN IF NOT EXISTS stock INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS listing_variants (
    id          TEXT PRIMARY KEY,
    listing_id  TEXT NOT NULL REFERENCES listings(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    sku         TEXT NOT NULL DEFAULT '',
    price       BIGINT NOT NULL DEFAULT 0,
    stock       INT NOT NULL DEFAULT 0,
    image_url   TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS listing_variants_listing_idx ON listing_variants (listing_id);
```

---

## 4. gRPC Services & RPC Contracts

### `platform.listing.v1.ListingService`

| Method | Request Payload | Response Payload | Required Scope | Description |
|---|---|---|---|---|
| `GetListing` | `GetListingRequest { id }` | `GetListingResponse { listing }` | `listing.read` | Retrieve listing by ID along with its variants |
| `ListListings` | `ListListingsRequest { page, status }` | `ListListingsResponse { listings, page }` | `listing.read` | Keyset-paginated listings filtered by status |
| `ListMyListings` | `ListMyListingsRequest { page }` | `ListMyListingsResponse { listings, page }` | `listing.write` | Listings owned by the calling seller |
| `CreateListing` | `CreateListingRequest { listing }` | `CreateListingResponse { listing }` | `listing.write` | Create listing, assigns `seller_id`, emits `ListingChanged(CREATED)` |
| `UpdateListing` | `UpdateListingRequest { listing }` | `UpdateListingResponse { listing }` | `listing.write` | Updates listing & variants, enforces ownership, emits `ListingChanged(UPDATED)` |
| `DeleteListing` | `DeleteListingRequest { id }` | `DeleteListingResponse {}` | `listing.write` | Deletes listing & variants, enforces ownership, emits `ListingChanged(DELETED)` |
| `GetImageUploadUrl` | `GetImageUploadUrlRequest { content_type, filename }` | `GetImageUploadUrlResponse { upload_url, image_key, public_url }` | `listing.write` | Generates 15-min S3 SigV4 presigned upload URL |
| `ListCategories` | `ListCategoriesRequest { parent_id }` | `ListCategoriesResponse { categories }` | `listing.read` | Lists categories, optionally filtered by `parent_id` |
| `GetCategory` | `GetCategoryRequest { id }` | `GetCategoryResponse { category }` | `listing.read` | Retrieves single category by ID |
| `ReserveStock` | `ReserveStockRequest { listing_id, variant_id, quantity, reservation_id }` | `ReserveStockResponse { success, message }` | *Internal RPC* | Atomically decrements stock if `stock >= quantity` |
| `ReleaseStock` | `ReleaseStockRequest { listing_id, variant_id, quantity, reservation_id }` | `ReleaseStockResponse { success }` | *Internal RPC* | Re-increments stock on cancelled orders |

---

## 5. Domain Events (Kafka)

Published to `listing.events` wrapped in `platform.events.v1.EventEnvelope`:

```protobuf
message EventEnvelope {
  string event_id = 1;       // UUID
  string type = 2;           // FQ proto type name (e.g. "platform.listing.v1.ListingChanged")
  google.protobuf.Timestamp occurred_at = 3;
  platform.common.v1.Principal principal = 4;
  string traceparent = 5;    // W3C trace context
  string request_id = 6;
  bytes payload = 7;         // Serialized proto message
}
```

### Event Types
1. `platform.listing.v1.ListingChanged`: Full listing snapshot with `change_type` (`CREATED`, `UPDATED`, `DELETED`).
2. `platform.listing.v1.ListingBaseInfoChanged`: Metadata changes (title, description, category, images, status).
3. `platform.listing.v1.ListingPricingChanged`: Price & flash sale promotional price changes.
4. `platform.listing.v1.ListingStockChanged`: Stock inventory level updates across base listing & variants.
5. `platform.listing.v1.ListingStatusChanged`: Status transitions (`DRAFT`, `PUBLISHED`, `REJECTED`).

---

## 6. How to Run & Test

### Local Development

```bash
# 1. Start postgres-listing and redpanda from platform-core
cd ../platform-core/infra && docker compose -p platform-core up -d postgres-listing redpanda

# 2. Configure environment
cd ../../team-domain
cp .env.example .env

# 3. Apply database migrations
make migrate

# 4. Run service
make run
```

### Verification via grpcurl

```bash
# List categories (Public/Read scope)
grpcurl -plaintext -H 'x-principal-id: user-1' -H 'x-principal-type: user' -H 'x-principal-scopes: listing.read' \
  localhost:50051 platform.listing.v1.ListingService/ListCategories

# Create listing (Seller scope)
grpcurl -plaintext -H 'x-principal-id: seller-1' -H 'x-principal-type: user' -H 'x-principal-scopes: listing.write' \
  -d '{"listing": {"title": "iPhone 15 Pro", "description": "Titan tự nhiên 128GB", "price": 25000000, "currency": "VND", "status": "LISTING_STATUS_PUBLISHED", "category_id": "cat-electronics", "stock": 50}}' \
  localhost:50051 platform.listing.v1.ListingService/CreateListing

# Reserve stock
grpcurl -plaintext \
  -d '{"listing_id": "<LISTING_ID>", "quantity": 2, "reservation_id": "res-1"}' \
  localhost:50051 platform.listing.v1.ListingService/ReserveStock
```

### Quality Gate

```bash
make check      # Runs check-env, gofmt, go vet, and all tests
```
