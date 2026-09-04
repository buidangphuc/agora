# team-search — Search & Discovery Read-Model Microservice

`team-search` is the buyer-facing **search and discovery read-model** in the marketplace architecture. Following the **CQRS pattern (ADR-0005)**, it consumes product lifecycle events from Kafka (`listing.events`) emitted by `team-domain` and maintains an optimized lexical index in **OpenSearch**.

It serves high-speed full-text queries, multi-attribute filtering (category, price range, status), structured sorting, and type-ahead prefix autocomplete via gRPC.

---

## 1. Responsibilities & Architecture

- **Rebuildable Projection (CQRS)**: The OpenSearch index is a pure projection of Kafka events. Replaying the `listing.events` topic completely rebuilds the read-model index from scratch.
- **Lexical Search Engine**: Multi-match queries over `title^2` and `description`, combined with term filters (`category_id`, `status`, `currency`), range queries (`price`), and sorting (`relevance`, `price_asc`, `price_desc`, `newest`).
- **Type-Ahead Autocomplete**: Uses OpenSearch `search_as_you_type` (`bool_prefix`) fields on `title` to generate instant suggestions with zero external search index dependencies.
- **Two Processes, One Codebase**:
  1. `cmd/server`: gRPC query API serving `platform.search.v1.SearchService` on port `50052`.
  2. `cmd/indexer`: Continuous Kafka consumer (`team-search-indexer` group) projecting events into OpenSearch in real-time.
- **Resource Efficiency**: Fine-grained events (`ListingBaseInfoChanged`, `ListingPricingChanged`, `ListingStatusChanged`) perform OpenSearch **Partial Updates**, avoiding costly full document re-indexing.

---

## 2. Configuration & Environment Variables

| Variable | Type | Default | Description |
|---|---|---|---|
| `ENV` | `string` | `local` | Environment mode (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `string` | `info` | Log verbosity (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `bool` | `true` | Log format in JSON |
| `GRPC_HOST` | `string` | `0.0.0.0` | Bind host for gRPC server |
| `GRPC_PORT` | `int` | `50052` | gRPC server listening port |
| `GRPC_REFLECTION_ENABLED` | `bool` | `true` | Enable gRPC reflection |
| `SHUTDOWN_GRACE_SECONDS` | `float` | `10` | Server shutdown drain timeout |
| `OPENSEARCH_URL` | `string` | `http://localhost:9200` | OpenSearch cluster endpoint |
| `OPENSEARCH_INDEX` | `string` | `listings` | Target index name |
| `KAFKA_ENABLED` | `bool` | `false` | Enable Kafka consumer (required for `cmd/indexer`) |
| `KAFKA_BROKERS` | `string` | `localhost:9092` | Comma-separated Kafka broker seed addresses |
| `KAFKA_CONSUMER_GROUP` | `string` | `team-search-indexer` | Kafka consumer group name |
| `KAFKA_LISTING_TOPIC` | `string` | `listing.events` | Ingest topic name |
| `OTEL_ENABLED` | `bool` | `false` | Enable OpenTelemetry tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `string` | `""` | OTLP gRPC endpoint (`localhost:4317`) |
| `OTEL_SERVICE_NAME` | `string` | `team-search` | Tracing service name |

---

## 3. OpenSearch Index Schema & Mappings

The read-model index `listings` is automatically created on startup via `EnsureIndex`:

```json
{
  "settings": {
    "analysis": {
      "analyzer": {
        "default": { "type": "standard" }
      }
    }
  },
  "mappings": {
    "properties": {
      "id":          { "type": "keyword" },
      "title":       { "type": "search_as_you_type" },
      "description": { "type": "text" },
      "status":      { "type": "keyword" },
      "currency":    { "type": "keyword" },
      "price":       { "type": "long" },
      "category_id": { "type": "keyword" }
    }
  }
}
```

---

## 4. Kafka Event Consumption & Index Projection

The `cmd/indexer` consumes `platform.events.v1.EventEnvelope` and routes by `type`:

| Event Type | Operation on OpenSearch | Details |
|---|---|---|
| `platform.listing.v1.ListingChanged` | `Upsert` / `Delete` | If `change_type == DELETED`, deletes doc; otherwise full upsert of `ListingDoc`. |
| `platform.listing.v1.ListingBaseInfoChanged` | `PartialUpdate` / `Delete` | Updates `title`, `description`, `category_id`, `status` without modifying other fields. |
| `platform.listing.v1.ListingPricingChanged` | `PartialUpdate` | Updates `price` (evaluates `is_on_sale` & `promotional_price`) and `currency`. |
| `platform.listing.v1.ListingStatusChanged` | `PartialUpdate` / `Delete` | Updates `status`; if status is `REJECTED`, removes document from index. |

---

## 5. gRPC Services & RPC Contracts

### `platform.search.v1.SearchService`

#### `SearchListings`
- **Request**:
  ```protobuf
  message SearchListingsRequest {
    string query = 1;                     // Text to match against title^2 and description
    platform.common.v1.PageRequest page = 2; // cursor (offset string), page_size
    map<string, string> filters = 3;      // Optional term filters (e.g. {"status":"published"})
    string category_id = 4;               // Exact category match
    int64 min_price = 5;                  // Min price bound (inclusive)
    int64 max_price = 6;                  // Max price bound (inclusive)
    SortBy sort_by = 7;                   // SORT_BY_RELEVANCE, SORT_BY_PRICE_ASC, SORT_BY_PRICE_DESC, SORT_BY_NEWEST
    int32 min_rating = 8;
  }
  ```
- **Response**:
  ```protobuf
  message SearchListingsResponse {
    repeated SearchHit hits = 1;          // { listing_id, score }
    platform.common.v1.PageResponse page = 2; // next_cursor (offset), total
  }
  ```
- **Required Scope**: `search:read`
- **Pagination**: Converts opaque cursor into OpenSearch `from` / `size` (default page size: 10, max: 50).

#### `Suggest`
- **Request**:
  ```protobuf
  message SuggestRequest {
    string query = 1;                     // Partial prefix string typed by user
    int32 limit = 2;                      // Max items (default: 5, max: 20)
  }
  ```
- **Response**:
  ```protobuf
  message SuggestResponse {
    repeated string suggestions = 1;      // Unique matching listing titles
  }
  ```
- **Required Scope**: `search:read`

---

## 6. How to Run & Test

### Local Development

```bash
# 1. Start OpenSearch and Redpanda from platform-core
cd ../platform-core/infra && docker compose -p platform-core up -d opensearch redpanda

# 2. Configure environment
cd ../../team-search
cp .env.example .env

# 3. Start indexer (consume Kafka -> OpenSearch)
KAFKA_ENABLED=true make indexer

# 4. Start gRPC Search Server (:50052)
make server
```

### Verification via grpcurl

```bash
# Search listings
grpcurl -plaintext -H 'x-principal-scopes: search:read' \
  -d '{"query":"iphone","min_price":10000000,"sort_by":2}' \
  localhost:50052 platform.search.v1.SearchService/SearchListings

# Autocomplete suggestions
grpcurl -plaintext -H 'x-principal-scopes: search:read' \
  -d '{"query":"iph","limit":5}' \
  localhost:50052 platform.search.v1.SearchService/Suggest

# Health check
grpcurl -plaintext localhost:50052 grpc.health.v1.Health/Check
```

### Quality Gate

```bash
make check      # Runs check-env, gofmt, go vet, and unit/integration tests
```
