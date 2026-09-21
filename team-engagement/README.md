# team-engagement — Community, Trust & Post-Order Engagement (Go)

The `team-engagement` microservice manages all user interaction, social proof, behavioral trust signals, product reviews and ratings, community Q&A, dispute resolution workflows, wishlist collections, seller follow graphs, and loyalty check-in streaks across the Agora marketplace.

The service strictly adheres to **Database-per-service (Rule 3)**, owning its dedicated `engagement_db` database on PostgreSQL (port `:5436`), and exposes gRPC service `platform.engagement.v1.EngagementService` on port `:50054`.

---

## 1. Service Overview & Responsibilities

`team-engagement` serves as the core behavioral and trust engine of the marketplace. Key responsibilities include:

1. **Favorites, Views & Behavioral Signals:**
   - Real-time product view tracking (`RecordView`) and per-user recently viewed browsing history (`view_history`).
   - Favorite / Wishlist toggling (`AddFavorite`, `RemoveFavorite`, `IsFavorite`) and aggregate counter updates (`listing_stats`).
   - Signal generation consumed by recommendation pipelines and search ranking engines.
2. **Reviews & Ratings with Verified Purchase Check:**
   - 1-to-5 star rating and comment submissions linked with specific orders (`order_id`) and media attachments (`media_urls`).
   - Automated gRPC verification against `team-order` (`GetOrder`) to validate completed purchases and mark reviews with `verified_purchase = true`.
   - Denormalized `seller_id` capture enabling shop-level rating aggregation (`GetShopRatingSummary`) without cross-database joins.
   - Per-user helpfulness upvoting (`MarkReviewHelpful`) backed by idempotent vote tracking (`review_helpful_votes`).
3. **Wishlist Collections:**
   - Multi-tier curation enabling buyers to create custom named wishlist folders (`collections`) and organize saved items (`collection_items`).
4. **Community Product Q&A:**
   - Threaded pre-purchase Q&A on product detail pages (`product_questions` and `product_answers`).
   - Official shop reply identification (`is_shop_reply`) to distinguish verified merchant answers.
5. **Dispute Resolution & Escalation:**
   - Order-level dispute filing by claimants against defendants with structured evidence tracking (`evidence_urls`).
   - Lifecycle state machine transitions: `OPEN` -> `INVESTIGATING` -> `RESOLVED` / `REJECTED`.
6. **Seller Follow Graph & Feed Generation:**
   - Social graph tracking follower relationships (`follows`).
   - Dynamic timeline feed queries (`ListFollowedListings`) that resolve recent listings from followed merchants.
7. **Loyalty & Daily Check-in Streak:**
   - Daily gamified check-in tracking (`checkins`) with consecutive streak calculation and marketplace coin balance rewards (`loyalty_accounts`).

---

## 2. Technology Stack & Key Libraries

- **Language & Runtime:** Go 1.22 (Pinnned standard library & toolchain)
- **RPC & Interface Contract:** gRPC Go (`google.golang.org/grpc`), Protocol Buffers v2 (`google.golang.org/protobuf`), Buf CLI managed generation.
- **Database & Storage:** PostgreSQL 16 via `github.com/jackc/pgx/v5` (`pgxpool` connection pool).
- **Security & Authorization:** Tokenless downstream architecture (ADR-0006); decodes forwarded `Principal` from Gateway metadata headers (`x-principal-id`, `x-principal-type`, `x-principal-scopes`) via `interceptor.RequireScopes`.
- **Cross-Service Communication:** gRPC Client dialing `team-order` over internal network (`UPSTREAM_ORDER_ADDR`).
- **Observability:** OpenTelemetry Go SDK (`go.opentelemetry.io/otel`), structured logging via standard library `log/slog`.
- **Testing:** Native Go `testing`, `testify` assertions/suites, in-process ephemeral gRPC listeners.

---

## 3. Detailed Architecture Diagram

```mermaid
flowchart TD
    subgraph Clients["Clients Layer"]
        Browser["Web / Mobile Browser"]
    end

    subgraph Edge["Edge Layer"]
        Gateway["team-gateway (Connect Edge :8080)"]
    end

    subgraph Service["team-engagement Service (:50054)"]
        GRPCServer["gRPC Server & Interceptors (Auth / Tracing)"]
        Handler["EngagementHandler (Wire Mappers & RPC Endpoints)"]
        
        subgraph Domains["Domain Services"]
            ReviewSvc["ReviewService"]
            QASvc["QAService"]
            DisputeSvc["DisputeService"]
            WishlistSvc["WishlistService"]
        end

        subgraph Repositories["Data Access Layer"]
            EngagementPG["PostgresEngagementRepo (Favorites, Stats, History, Follows, Loyalty)"]
            ReviewPG["PostgresReviewRepo (Reviews, Breakdown, Helpful Votes)"]
            QAPG["PostgresQARepo (Questions & Answers)"]
            DisputePG["PostgresDisputeRepo (Dispute Tickets)"]
            WishlistPG["PostgresWishlistRepo (Collections & Items)"]
        end

        UpstreamOrder["Upstream OrderClient (gRPC)"]
    end

    subgraph Siblings["Sibling Services"]
        TeamOrder["team-order (:50055)"]
    end

    subgraph Storage["Persistence Layer"]
        PostgresDB[("PostgreSQL: engagement_db (:5436)")]
    end

    Browser -->|HTTP / Connect RPC| Gateway
    Gateway -->|gRPC + x-principal metadata| GRPCServer
    GRPCServer --> Handler
    Handler --> ReviewSvc
    Handler --> QASvc
    Handler --> DisputeSvc
    Handler --> WishlistSvc
    Handler --> EngagementPG

    ReviewSvc --> UpstreamOrder
    ReviewSvc --> ReviewPG
    QASvc --> QAPG
    DisputeSvc --> DisputePG
    WishlistSvc --> WishlistPG

    UpstreamOrder -->|gRPC GetOrder| TeamOrder

    EngagementPG --> PostgresDB
    ReviewPG --> PostgresDB
    QAPG --> PostgresDB
    DisputePG --> PostgresDB
    WishlistPG --> PostgresDB
```

---

## 4. Internal Package Structure & Data Schemas

### 4.1. Internal Package Structure

```
team-engagement/
├── cmd/
│   └── server/
│       └── main.go                 # Service entrypoint, lifecycle orchestration & signal handling
├── generated/                      # Buf-generated gRPC stubs (gitignored / vendored)
│   └── platform/
│       ├── common/v1/              # Common models (Principal, PageRequest, PageResponse)
│       ├── engagement/v1/          # EngagementService proto definitions & stubs
│       └── order/v1/               # Upstream OrderService proto stubs
├── internal/
│   ├── bootstrap/
│   │   ├── lifecycle.go            # Resource lifecycle and clean shutdown coordinator
│   │   └── resources.go            # Connection pool initialization (PostgreSQL pgxpool)
│   ├── config/
│   │   ├── config.go               # Environment configuration struct & validation
│   │   ├── config_test.go          # Configuration loading unit tests
│   │   └── envcheck.go             # Environment drift validation against .env.example
│   ├── grpcserver/
│   │   ├── server.go               # gRPC server factory with auth & tracing interceptors
│   │   └── server_test.go          # In-process ephemeral server bootstrap tests
│   ├── handler/
│   │   ├── engagement.go           # Core engagement RPC handlers & wire conversions
│   │   ├── engagement_test.go      # Integration tests covering all RPC flows
│   │   ├── reviews.go              # Review and rating summary endpoints
│   │   └── wishlist.go             # Wishlist collection and item endpoints
│   ├── interceptor/
│   │   ├── auth.go                 # Principal extraction & RequireScopes authorization
│   │   └── tracing.go              # OpenTelemetry span propagation interceptor
│   ├── observability/
│   │   └── tracer.go               # OpenTelemetry tracer provider initialization
│   ├── repository/
│   │   ├── dispute.go              # Dispute repository interface & Postgres implementation
│   │   ├── engagement.go           # Engagement repository interface & domain entities
│   │   ├── engagement_pg.go        # PostgreSQL implementation for favorites, stats, follows & loyalty
│   │   ├── qa.go                   # Q&A repository interface & PostgreSQL queries
│   │   ├── review.go               # Review repository interface, SQL queries & in-memory mocks
│   │   └── wishlist.go             # Wishlist collection repository interface & implementation
│   ├── service/
│   │   ├── dispute.go              # Dispute business rules & validation logic
│   │   ├── qa.go                   # Q&A moderation & reply authorization rules
│   │   ├── review.go               # Review workflows & verified purchase orchestration
│   │   └── wishlist.go             # Collection naming & item membership logic
│   └── upstream/
│       └── order.go                # Outbound gRPC client for team-order verification
└── migrations/
    ├── 0001_engagement.up.sql      # favorites & listing_stats tables
    ├── 0002_reviews.up.sql         # reviews table & indexes
    ├── 0003_qa_and_disputes.up.sql # product_questions, product_answers & disputes tables
    ├── 0004_wishlist_collections.up.sql # collections & collection_items tables
    ├── 0005_review_enrichment.up.sql   # media_urls, helpful_votes & seller_id denormalization
    ├── 0006_view_history.up.sql    # view_history table
    ├── 0007_follows.up.sql         # follows & seller_listings tables
    └── 0008_loyalty.up.sql         # loyalty_accounts & checkins tables
```

### 4.2. Database Entity-Relationship Diagram

```mermaid
erDiagram
    FAVORITES {
        text user_id PK
        text listing_id PK
        timestamptz created_at
    }

    LISTING_STATS {
        text listing_id PK
        bigint view_count
        bigint favorite_count
    }

    VIEW_HISTORY {
        text user_id PK
        text listing_id PK
        timestamptz viewed_at
    }

    REVIEWS {
        varchar id PK
        varchar listing_id
        varchar user_id
        varchar user_name
        varchar order_id
        varchar seller_id
        int rating
        text comment
        text_array media_urls
        bigint helpful_count
        boolean verified_purchase
        timestamptz created_at
    }

    REVIEW_HELPFUL_VOTES {
        varchar review_id PK, FK
        varchar user_id PK
        timestamptz created_at
    }

    COLLECTIONS {
        varchar id PK
        varchar user_id
        varchar name
        timestamptz created_at
    }

    COLLECTION_ITEMS {
        varchar collection_id PK, FK
        varchar listing_id PK
        timestamptz created_at
    }

    PRODUCT_QUESTIONS {
        varchar id PK
        varchar listing_id
        varchar user_id
        text question_text
        timestamptz created_at
    }

    PRODUCT_ANSWERS {
        varchar id PK
        varchar question_id FK
        varchar listing_id
        varchar user_id
        text answer_text
        boolean is_shop_reply
        timestamptz created_at
    }

    DISPUTES {
        varchar id PK
        varchar order_id
        varchar claimant_id
        varchar defendant_id
        text reason
        text_array evidence_urls
        varchar status
        text resolution
        timestamptz created_at
        timestamptz updated_at
    }

    FOLLOWS {
        text user_id PK
        text seller_id PK
        timestamptz created_at
    }

    SELLER_LISTINGS {
        text seller_id PK
        text listing_id PK
        timestamptz created_at
    }

    LOYALTY_ACCOUNTS {
        text user_id PK
        bigint coin_balance
        int streak
        date last_checkin
    }

    CHECKINS {
        text user_id PK
        date day PK
        timestamptz created_at
    }

    REVIEWS ||--o{ REVIEW_HELPFUL_VOTES : "receives votes"
    COLLECTIONS ||--o{ COLLECTION_ITEMS : "contains"
    PRODUCT_QUESTIONS ||--o{ PRODUCT_ANSWERS : "answered by"
    FOLLOWS }o--o{ SELLER_LISTINGS : "generates feed"
    LOYALTY_ACCOUNTS ||--o{ CHECKINS : "tracked by"
```

---

## 5. Core Workflows

### 5.1. Reviews & Ratings with Verified Purchase Check

When a buyer submits a review:
1. The handler extracts `Principal{id, scopes}` and ensures the caller has `engagement:write` scope.
2. `ReviewService` checks if an `order_id` is supplied. If present, it invokes `upstream.OrderClient.VerifyPurchase(ctx, buyerID, listingID, orderID)` over gRPC calling `team-order.GetOrder`.
3. `team-order` verifies that:
   - The order exists and belongs to the buyer (`buyer_id == principal.id`).
   - The order status is `ORDER_STATUS_COMPLETED`.
   - The order contains the target `listing_id`.
4. If verified, `verified_purchase` is set to `true` and the order's `seller_id` is denormalized onto the review record.
5. The review is persisted in `reviews` table. Future queries (`GetShopRatingSummary`) aggregate ratings by `seller_id` directly without cross-service calls.
6. Other buyers can upvote reviews via `MarkReviewHelpful`. Idempotency is enforced by `review_helpful_votes` primary key `(review_id, user_id)`.

```mermaid
sequenceDiagram
    autonumber
    actor Buyer
    participant Gateway as team-gateway
    participant Engagement as team-engagement
    participant Order as team-order
    participant DB as Postgres (engagement_db)

    Buyer->>Gateway: POST /platform.engagement.v1.EngagementService/CreateReview
    Gateway->>Engagement: gRPC CreateReview(listing_id, rating, comment, order_id)
    Engagement->>Order: gRPC GetOrder(order_id)
    Order-->>Engagement: Order{buyer_id, status: COMPLETED, items: [listing_id], seller_id}
    Note over Engagement: Check buyer_id match & status == COMPLETED<br/>verified_purchase = true, capture seller_id
    Engagement->>DB: INSERT INTO reviews (..., verified_purchase, seller_id)
    DB-->>Engagement: review_id
    Engagement-->>Gateway: CreateReviewResponse(Review)
    Gateway-->>Buyer: 200 OK (Review with Verified Badge)
```

### 5.2. Favorites, Wishlists & Browsing History

- **Favorites & Stats:** `AddFavorite` inserts into `favorites` and atomically increments `listing_stats.favorite_count`. `RecordView` increments `listing_stats.view_count` and, if authenticated, upserts into `view_history` with updated `viewed_at = now()`.
- **Wishlist Collections:** Buyers create custom collections (e.g., "Tech Wishlist", "Living Room Decor") via `CreateCollection`. Items are added/removed via `AddToCollection` / `RemoveFromCollection` with automatic item count aggregation.

```mermaid
sequenceDiagram
    autonumber
    actor Buyer
    participant Gateway as team-gateway
    participant Engagement as team-engagement
    participant DB as Postgres (engagement_db)

    Buyer->>Gateway: POST /platform.engagement.v1.EngagementService/AddFavorite
    Gateway->>Engagement: gRPC AddFavorite(listing_id)
    Engagement->>DB: INSERT INTO favorites (user_id, listing_id) ON CONFLICT DO NOTHING
    Engagement->>DB: INSERT INTO listing_stats (listing_id, favorite_count) VALUES (listing_id, 1) ON CONFLICT DO UPDATE SET favorite_count = favorite_count + 1
    DB-->>Engagement: OK
    Engagement-->>Gateway: AddFavoriteResponse
    Gateway-->>Buyer: 200 OK
```

### 5.3. Community Product Q&A Threads

- Buyers submit questions on product pages via `AskQuestion`.
- Anyone or the merchant can submit answers via `AnswerQuestion`. If the answering user is the merchant, `is_shop_reply` is marked `true`.
- Listing detail pages fetch nested question/answer threads via `ListQuestionsByListing` with pagination.

```mermaid
sequenceDiagram
    autonumber
    actor Buyer
    actor Merchant
    participant Gateway as team-gateway
    participant Engagement as team-engagement
    participant DB as Postgres (engagement_db)

    Buyer->>Gateway: POST /AskQuestion(listing_id, "Does it have warranty?")
    Gateway->>Engagement: gRPC AskQuestion
    Engagement->>DB: INSERT INTO product_questions(...)
    DB-->>Engagement: question_id
    Engagement-->>Buyer: ProductQuestion created

    Merchant->>Gateway: POST /AnswerQuestion(question_id, "12 months official warranty", is_shop_reply: true)
    Gateway->>Engagement: gRPC AnswerQuestion
    Engagement->>DB: INSERT INTO product_answers(question_id, is_shop_reply: true, ...)
    DB-->>Engagement: answer_id
    Engagement-->>Merchant: ProductAnswer created
```

### 5.4. Dispute Resolution Tickets

- A buyer initiates a dispute via `CreateDispute` specifying `order_id`, `defendant_id`, `reason`, and `evidence_urls`. The service validates `claimant_id != defendant_id` and records status `OPEN`.
- Support admins and sellers retrieve tickets via `GetDispute`.
- Disputes are progressed to `INVESTIGATING`, and finalized to `RESOLVED` or `REJECTED` via `ResolveDispute` along with a formal `resolution` justification.

```mermaid
stateDiagram-v2
    [*] --> OPEN: CreateDispute (Buyer)
    OPEN --> INVESTIGATING: Admin Review
    INVESTIGATING --> RESOLVED: Resolution Accepted
    INVESTIGATING --> REJECTED: Evidence Insufficient
    RESOLVED --> [*]
    REJECTED --> [*]
```

### 5.5. Seller Follower Graph & Personalized Feed

- Buyers follow favorite shops via `FollowSeller(seller_id)`.
- The `follows` table maintains follower mappings `(user_id, seller_id)`.
- `ListFollowedListings` performs an indexed join between `follows` and `seller_listings` to assemble a real-time chronological product feed of newly published listings from followed sellers.

```mermaid
sequenceDiagram
    autonumber
    actor Buyer
    participant Gateway as team-gateway
    participant Engagement as team-engagement
    participant DB as Postgres (engagement_db)

    Buyer->>Gateway: POST /FollowSeller(seller_id: "shop_123")
    Gateway->>Engagement: gRPC FollowSeller
    Engagement->>DB: INSERT INTO follows (user_id, seller_id) ON CONFLICT DO NOTHING
    DB-->>Engagement: OK
    Engagement-->>Buyer: 200 OK

    Buyer->>Gateway: GET /ListFollowedListings
    Gateway->>Engagement: gRPC ListFollowedListings
    Engagement->>DB: SELECT sl.listing_id FROM follows f JOIN seller_listings sl ON f.seller_id = sl.seller_id WHERE f.user_id = $1 ORDER BY sl.created_at DESC
    DB-->>Engagement: [listing_id_1, listing_id_2, ...]
    Engagement-->>Buyer: ListFollowedListingsResponse(listing_ids)
```

---

## 6. gRPC API Specification (`EngagementService`)

Defined in package `platform.engagement.v1`.

### 6.1. Favorites, Stats & History
| RPC Method | Request Payload | Response Payload | Auth & Scopes | Description |
|---|---|---|---|---|
| `AddFavorite` | `listing_id` | `{}` | `engagement:write` | Adds item to favorites and increments favorite counter. |
| `RemoveFavorite` | `listing_id` | `{}` | `engagement:write` | Removes item from favorites and decrements counter. |
| `IsFavorite` | `listing_id` | `favorite` (bool) | `engagement:read` | Checks if current user has favorited the item. |
| `ListFavorites` | `page` (PageRequest) | `listing_ids`, `page` | `engagement:read` | Lists all listing IDs favorited by the current user. |
| `RecordView` | `listing_id` | `view_count` | Public / Optional Auth | Increments view counter and updates user's recent history. |
| `GetListingStats` | `listing_id` | `view_count`, `favorite_count` | Public | Retrieves aggregated view and favorite metrics. |
| `GetRecentlyViewed`| `page` (PageRequest) | `listing_ids`, `page` | `engagement:read` | Retrieves user's chronological recently viewed history. |

### 6.2. Reviews & Ratings
| RPC Method | Request Payload | Response Payload | Auth & Scopes | Description |
|---|---|---|---|---|
| `CreateReview` | `listing_id`, `rating`, `comment`, `order_id`, `media_urls` | `review` (Review) | `engagement:write` | Creates a product review; verifies purchase via `team-order`. |
| `ListReviews` | `listing_id`, `rating_filter`, `page` | `reviews`, `page` | Public | Fetches paginated reviews for a listing with star filter. |
| `GetListingRatingSummary` | `listing_id` | `average_rating`, `review_count`, `breakdown` | Public | Computes average rating and star breakdown for a listing. |
| `MarkReviewHelpful` | `review_id` | `helpful_count` | `engagement:write` | Records helpful vote (idempotent per user). |
| `GetShopRatingSummary` | `seller_id` | `average_rating`, `review_count`, `breakdown` | Public | Aggregates all ratings across a seller's catalog. |

### 6.3. Wishlist Collections
| RPC Method | Request Payload | Response Payload | Auth & Scopes | Description |
|---|---|---|---|---|
| `CreateCollection` | `name` | `collection` (Collection) | `engagement:write` | Creates a named wishlist folder. |
| `ListCollections` | `{}` | `collections` (repeated) | `engagement:read` | Lists user's wishlist collections with item counts. |
| `AddToCollection` | `collection_id`, `listing_id` | `{}` | `engagement:write` | Adds an item to a specific collection. |
| `RemoveFromCollection` | `collection_id`, `listing_id` | `{}` | `engagement:write` | Removes an item from a collection. |
| `ListCollectionItems` | `collection_id`, `page` | `listing_ids`, `page` | `engagement:read` | Lists listing IDs contained inside a collection. |

### 6.4. Product Q&A & Disputes
| RPC Method | Request Payload | Response Payload | Auth & Scopes | Description |
|---|---|---|---|---|
| `AskQuestion` | `listing_id`, `question_text` | `question` (ProductQuestion) | `engagement:write` | Submits a pre-purchase inquiry on a listing. |
| `AnswerQuestion` | `question_id`, `answer_text`, `is_shop_reply` | `answer` (ProductAnswer) | `engagement:write` | Answers an existing question. |
| `ListQuestionsByListing` | `listing_id`, `page` | `questions` (with answers), `page` | Public | Lists all questions and nested answers for a listing. |
| `CreateDispute` | `order_id`, `defendant_id`, `reason`, `evidence_urls` | `dispute` (Dispute) | `engagement:write` | Opens a dispute ticket for an order. |
| `GetDispute` | `dispute_id` | `dispute` (Dispute) | `engagement:read` | Retrieves dispute status and history. |
| `ResolveDispute` | `dispute_id`, `status`, `resolution` | `dispute` (Dispute) | `engagement:write` | Resolves or rejects an active dispute. |

### 6.5. Social Follows & Loyalty
| RPC Method | Request Payload | Response Payload | Auth & Scopes | Description |
|---|---|---|---|---|
| `FollowSeller` | `seller_id` | `{}` | `engagement:write` | Follows a shop. |
| `UnfollowSeller` | `seller_id` | `{}` | `engagement:write` | Unfollows a shop. |
| `ListFollowedSellers` | `page` | `seller_ids`, `page` | `engagement:read` | Lists seller IDs followed by user. |
| `IsFollowing` | `seller_id` | `following` (bool) | `engagement:read` | Checks follow status for a seller. |
| `ListFollowedListings` | `page` | `listing_ids`, `page` | `engagement:read` | Resolves feed of listings from followed sellers. |
| `CheckIn` | `{}` | `streak`, `coins_earned`, `coin_balance` | `engagement:write` | Idempotent daily check-in rewarding coins. |
| `GetLoyalty` | `{}` | `streak`, `coin_balance`, `last_checkin` | `engagement:read` | Fetches loyalty status and coin balance. |

---

## 7. Environment Variables

| Variable | Default | Description |
|---|---|---|
| `ENV` | `local` | Runtime environment (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `info` | Structured log severity level (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `true` | Format log outputs as JSON |
| `GRPC_HOST` | `0.0.0.0` | gRPC server bind host |
| `GRPC_PORT` | `50054` | gRPC server listening port |
| `GRPC_REFLECTION_ENABLED` | `true` | Enable gRPC Server Reflection |
| `SHUTDOWN_GRACE_SECONDS` | `10` | Maximum graceful shutdown drain timeout |
| `DATABASE_ENABLED` | `true` | Enable PostgreSQL database connection pool |
| `DATABASE_URL` | `""` | PostgreSQL connection string (`postgres://user:pass@host:5436/engagement_db`) |
| `DB_MAX_CONNS` | `10` | Maximum connection pool size |
| `UPSTREAM_ORDER_ADDR` | `""` | gRPC host:port for `team-order` (e.g. `localhost:50055`) |
| `OTEL_ENABLED` | `false` | Enable OpenTelemetry tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT`| `""` | OpenTelemetry OTLP collector endpoint (e.g. `localhost:4317`) |
| `OTEL_SERVICE_NAME` | `team-engagement` | Service name identifier in traces |

---

## 8. Local Setup & Testing

```bash
# 1. Start Postgres infrastructure container
docker compose -p platform-core up -d postgres-engagement

# 2. Configure environment variables
cp .env.example .env

# 3. Run all unit and integration tests
go test -v -race ./...

# 4. Launch service locally
go run cmd/server/main.go
```
