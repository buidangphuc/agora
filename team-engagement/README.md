# team-engagement — Community, Trust & Post-Order Engagement (Go)

Dịch vụ `team-engagement` quản lý toàn bộ các tính năng tương tác người dùng, tín hiệu hành vi, đánh giá sản phẩm (Reviews & Ratings), hỏi đáp cộng đồng (Product Q&A), thống kê lượt xem/yêu thích và khiếu nại đơn hàng (Disputes).

Dịch vụ tuân thủ nghiêm ngặt **Database-per-service (Rule 3)**, sở hữu database `engagement_db` riêng biệt và cung cấp gRPC service `platform.engagement.v1.EngagementService` trên port `:50054`.

---

## 1. Kiến trúc & Tổng quan (Architecture Overview)

```
                     ┌──────────────────┐
                     │   team-gateway   │ (Connect Edge :8080)
                     └────────┬─────────┘
                              │ gRPC (:50054)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        team-engagement                          │
│                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │  Favorites   │  │   Reviews    │  │ Product Q&A  │          │
│  │   & Stats    │  │  & Ratings   │  │   & Replies  │          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
│         │                 │                 │                  │
│         └───────────┬─────┴─────────────────┘                  │
│                     │                                          │
│              ┌──────▼───────┐                                  │
│              │   Disputes   │                                  │
│              │  Resolution  │                                  │
│              └──────┬───────┘                                  │
└─────────────────────┼───────────────────────────────────────────┘
                      ▼
            ┌───────────────────┐
            │   Postgres DB     │ (engagement_db :5436)
            └───────────────────┘
```

### Ranh giới trách nhiệm (Bounded Context)
1. **Favorites & Stats:** Lưu vết danh sách yêu thích, đếm lượt xem (views) và lượt yêu thích làm tín hiệu gợi ý sản phẩm (Recommendation signal).
2. **Reviews & Ratings:** Đánh giá từ 1 đến 5 sao kèm bình luận, liên kết mã đơn hàng (`order_id`) và thống kê phân bổ sao (`RatingBreakdown`).
3. **Product Q&A:** Hỏi đáp công khai trên trang chi tiết sản phẩm giữa người mua và nhà bán hàng (`is_shop_reply`).
4. **Disputes:** Xử lý tranh chấp/khiếu nại đơn hàng giữa người mua (`claimant_id`) và người bán (`defendant_id`) với bằng chứng (`evidence_urls`) và trạng thái giải quyết (`OPEN` -> `INVESTIGATING` -> `RESOLVED` / `REJECTED`).
5. **Bảo mật & Auth:** Đọc `Principal` được giải mã sẵn và chuyển tiếp từ `team-gateway` qua gRPC metadata (`x-principal-*`). Các RPC đọc/ghi tương tác cá nhân yêu cầu scopes `engagement:read` / `engagement:write`. Các RPC xem thống kê/xem review/xem câu hỏi là public.

---

## 2. Cấu trúc thư mục (Repository Structure)

```
team-engagement/
├── cmd/
│   └── server/
│       └── main.go                 # Entrypoint khởi tạo server gRPC & grace shutdown
├── generated/                      # Proto generated code (gitignored / vendored)
│   └── platform/
│       ├── common/v1/              # Common proto (Principal, PageRequest, etc.)
│       └── engagement/v1/          # EngagementService proto & gRPC stubs
├── internal/
│   ├── bootstrap/
│   │   ├── lifecycle.go            # Quản lý khởi động và dọn dẹp tài nguyên
│   │   └── resources.go            # Kết nối PostgreSQL (pgxpool)
│   ├── config/
│   │   ├── config.go               # Struct cấu hình nạp từ ENV
│   │   └── config_test.go          # Test kiểm tra nạp biến môi trường
│   ├── grpcserver/
│   │   ├── server.go               # Cấu hình gRPC Server & Interceptors
│   │   └── server_test.go          # Test khởi động server gRPC trên ephemeral port
│   ├── handler/
│   │   ├── engagement.go           # Adapter chuyển đổi RPC gRPC sang Service domain
│   │   └── engagement_test.go      # Integration test cho tất cả các RPC handler
│   ├── interceptor/
│   │   ├── auth.go                 # Trích xuất Principal và kiểm tra Scope (RequireScopes)
│   │   └── tracing.go              # OpenTelemetry trace interceptor
│   ├── repository/
│   │   ├── engagement.go           # Interface & Data models cho Favorites/Stats
│   │   ├── engagement_pg.go        # PostgreSQL driver cho Favorites/Stats
│   │   ├── review.go               # Interface, Postgres & In-Memory Review repository
│   │   ├── qa.go                   # Interface, Postgres & In-Memory Q&A repository
│   │   ├── qa_test.go              # Unit tests cho Q&A repository
│   │   ├── dispute.go              # Interface, Postgres & In-Memory Dispute repository
│   │   └── dispute_test.go         # Unit tests cho Dispute repository
│   └── service/
│       ├── review.go               # Business logic cho Reviews & Ratings
│       ├── qa.go                   # Business logic cho Q&A
│       ├── qa_test.go              # Unit tests cho Q&A service
│       ├── dispute.go              # Business logic cho Disputes
│       └── dispute_test.go         # Unit tests cho Dispute service
└── migrations/
    ├── 0001_engagement.up.sql      # Schema bảng favorites & listing_stats
    ├── 0002_reviews.up.sql         # Schema bảng reviews
    └── 0003_qa_and_disputes.up.sql # Schema bảng product_questions, product_answers, disputes
```

---

## 3. Database Schema & Migrations

### Bảng dữ liệu:
1. `favorites`
   - `user_id` (`TEXT NOT NULL`): ID người dùng yêu thích.
   - `listing_id` (`TEXT NOT NULL`): ID sản phẩm được yêu thích.
   - `created_at` (`TIMESTAMPTZ NOT NULL DEFAULT now()`): Thời điểm bấm yêu thích.
   - **Primary Key:** `(user_id, listing_id)`
   - **Index:** `favorites_user_idx (user_id)`

2. `listing_stats`
   - `listing_id` (`TEXT PRIMARY KEY`): ID sản phẩm.
   - `view_count` (`BIGINT NOT NULL DEFAULT 0`): Lượt xem tích lũy.
   - `favorite_count` (`BIGINT NOT NULL DEFAULT 0`): Lượt yêu thích hiện tại.

3. `reviews`
   - `id` (`VARCHAR(64) PRIMARY KEY`): UUID review.
   - `listing_id` (`VARCHAR(64) NOT NULL`): ID sản phẩm được đánh giá.
   - `user_id` (`VARCHAR(64) NOT NULL`): ID người đánh giá.
   - `user_name` (`VARCHAR(128) NOT NULL DEFAULT ''`): Tên hiển thị người đánh giá.
   - `order_id` (`VARCHAR(64) NOT NULL DEFAULT ''`): Mã đơn hàng mua sản phẩm.
   - `rating` (`INT NOT NULL CHECK (rating >= 1 AND rating <= 5)`): Điểm đánh giá (1 đến 5 sao).
   - `comment` (`TEXT NOT NULL DEFAULT ''`): Nội dung nhận xét.
   - `created_at` (`TIMESTAMPTZ NOT NULL DEFAULT NOW()`): Thời gian đánh giá.
   - **Indexes:** `idx_reviews_listing_id (listing_id, created_at DESC)`, `idx_reviews_user_id (user_id)`

4. `product_questions`
   - `id` (`VARCHAR(64) PRIMARY KEY`): UUID câu hỏi.
   - `listing_id` (`VARCHAR(64) NOT NULL`): ID sản phẩm.
   - `user_id` (`VARCHAR(64) NOT NULL`): ID người hỏi.
   - `question_text` (`TEXT NOT NULL`): Nội dung câu hỏi.
   - `created_at` (`TIMESTAMPTZ NOT NULL DEFAULT NOW()`): Thời gian tạo câu hỏi.
   - **Indexes:** `idx_product_questions_listing_id (listing_id, created_at DESC)`, `idx_product_questions_user_id (user_id)`

5. `product_answers`
   - `id` (`VARCHAR(64) PRIMARY KEY`): UUID câu trả lời.
   - `question_id` (`VARCHAR(64) NOT NULL REFERENCES product_questions(id) ON DELETE CASCADE`): Khóa ngoại liên kết câu hỏi.
   - `listing_id` (`VARCHAR(64) NOT NULL DEFAULT ''`): ID sản phẩm.
   - `user_id` (`VARCHAR(64) NOT NULL`): ID người trả lời.
   - `answer_text` (`TEXT NOT NULL`): Nội dung câu trả lời.
   - `is_shop_reply` (`BOOLEAN NOT NULL DEFAULT FALSE`): Đánh dấu có phải phản hồi chính thức từ Shop hay không.
   - `created_at` (`TIMESTAMPTZ NOT NULL DEFAULT NOW()`): Thời gian trả lời.
   - **Indexes:** `idx_product_answers_question_id (question_id, created_at ASC)`, `idx_product_answers_user_id (user_id)`

6. `disputes`
   - `id` (`VARCHAR(64) PRIMARY KEY`): UUID tranh chấp.
   - `order_id` (`VARCHAR(64) NOT NULL`): Mã đơn hàng có tranh chấp.
   - `claimant_id` (`VARCHAR(64) NOT NULL`): ID bên khiếu nại (người mua).
   - `defendant_id` (`VARCHAR(64) NOT NULL`): ID bên bị khiếu nại (người bán).
   - `reason` (`TEXT NOT NULL`): Lý do khiếu nại / tranh chấp.
   - `evidence_urls` (`TEXT[] NOT NULL DEFAULT '{}'`): Danh sách URL hình ảnh/bằng chứng.
   - `status` (`VARCHAR(32) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'REJECTED'))`): Trạng thái xử lý.
   - `resolution` (`TEXT NOT NULL DEFAULT ''`): Quyết định / kết quả giải quyết tranh chấp.
   - `created_at` (`TIMESTAMPTZ NOT NULL DEFAULT NOW()`): Thời gian tạo.
   - `updated_at` (`TIMESTAMPTZ NOT NULL DEFAULT NOW()`): Thời gian cập nhật gần nhất.
   - **Indexes:** `idx_disputes_order_id`, `idx_disputes_claimant_id`, `idx_disputes_defendant_id`, `idx_disputes_status`

---

## 4. Đặc tả API gRPC (`EngagementService`)

Tất cả RPCs định nghĩa trong package `platform.engagement.v1`.

### 4.1. Favorites & Stats

| RPC Method | Request Payload | Response Payload | Auth / Scopes | Mô tả |
|---|---|---|---|---|
| `AddFavorite` | `listing_id` (string) | `{}` | `engagement:write` | Thêm sản phẩm vào danh sách yêu thích, tăng `favorite_count` |
| `RemoveFavorite` | `listing_id` (string) | `{}` | `engagement:write` | Bỏ yêu thích sản phẩm, giảm `favorite_count` |
| `IsFavorite` | `listing_id` (string) | `favorite` (bool) | `engagement:read` | Kiểm tra xem user hiện tại đã yêu thích sản phẩm chưa |
| `ListFavorites` | `page` (PageRequest: cursor, page_size) | `listing_ids` (repeated string), `page` (PageResponse) | `engagement:read` | Lấy danh sách ID các sản phẩm user đã yêu thích |
| `RecordView` | `listing_id` (string) | `view_count` (int64) | Public | Tăng số lượt xem của sản phẩm |
| `GetListingStats` | `listing_id` (string) | `view_count` (int64), `favorite_count` (int64) | Public | Lấy số lượt xem và số lượt yêu thích của sản phẩm |

### 4.2. Reviews & Ratings

| RPC Method | Request Payload | Response Payload | Auth / Scopes | Mô tả |
|---|---|---|---|---|
| `CreateReview` | `listing_id`, `rating` (1-5), `comment`, `order_id` | `review` (Review object) | `engagement:write` | Tạo đánh giá sản phẩm sau khi mua |
| `ListReviews` | `listing_id`, `rating_filter` (0=all, 1..5), `page` (PageRequest) | `reviews` (repeated Review), `page` (PageResponse) | Public | Lấy danh sách đánh giá của sản phẩm kèm lọc theo số sao |
| `GetListingRatingSummary` | `listing_id` | `listing_id`, `average_rating` (double), `review_count` (int64), `breakdown` (RatingBreakdown) | Public | Thống kê điểm trung bình và số lượng từng loại sao (1..5) |

### 4.3. Product Q&A

| RPC Method | Request Payload | Response Payload | Auth / Scopes | Mô tả |
|---|---|---|---|---|
| `AskQuestion` | `listing_id`, `question_text` | `question` (ProductQuestion) | `engagement:write` | Người mua đặt câu hỏi về sản phẩm |
| `AnswerQuestion` | `question_id`, `answer_text`, `is_shop_reply` (bool) | `answer` (ProductAnswer) | `engagement:write` | Trả lời câu hỏi (người dùng hoặc chủ shop) |
| `ListQuestionsByListing` | `listing_id`, `page` (PageRequest) | `questions` (repeated ProductQuestion gồm danh sách câu trả lời lồng nhau), `page` (PageResponse) | Public | Lấy toàn bộ câu hỏi và câu trả lời tương ứng của sản phẩm |

### 4.4. Disputes

| RPC Method | Request Payload | Response Payload | Auth / Scopes | Mô tả |
|---|---|---|---|---|
| `CreateDispute` | `order_id`, `defendant_id`, `reason`, `evidence_urls` (repeated string) | `dispute` (Dispute) | `engagement:write` | Mở khiếu nại đơn hàng (mặc định trạng thái `OPEN`) |
| `GetDispute` | `dispute_id` | `dispute` (Dispute) | `engagement:read` | Xem chi tiết tiến độ khiếu nại |
| `ResolveDispute` | `dispute_id`, `status` (`INVESTIGATING`/`RESOLVED`/`REJECTED`), `resolution` | `dispute` (Dispute) | `engagement:write` | Cập nhật kết quả giải quyết khiếu nại (Admin/Seller) |

---

## 5. Biến môi trường (Environment Variables)

| Biến môi trường | Mặc định | Ý nghĩa |
|---|---|---|
| `ENV` | `local` | Môi trường triển khai (`local`, `dev`, `prod`) |
| `LOG_LEVEL` | `info` | Mức log (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `true` | Xuất log dưới dạng JSON có cấu trúc |
| `GRPC_HOST` | `0.0.0.0` | Host lắng nghe gRPC |
| `GRPC_PORT` | `50054` | Cổng gRPC của service |
| `GRPC_REFLECTION_ENABLED` | `true` | Bật gRPC server reflection cho `grpcurl` debug |
| `SHUTDOWN_GRACE_SECONDS` | `10` | Thời gian chờ tối đa khi shutdown service |
| `DATABASE_ENABLED` | `true` | Kích hoạt kết nối Postgres |
| `DATABASE_URL` | `""` | Connection string tới database `engagement_db` |
| `DB_MAX_CONNS` | `10` | Kích thước connection pool |
| `OTEL_ENABLED` | `false` | Bật xuất trace OpenTelemetry |
| `OTEL_EXPORTER_OTLP_ENDPOINT`| `""` | Địa chỉ OTLP collector (ví dụ `localhost:4317`) |
| `OTEL_SERVICE_NAME` | `team-engagement` | Tên service hiển thị trên distributed trace |

---

## 6. Hướng dẫn Chạy & Kiểm thử (Local Run & Testing)

```bash
# 1. Khởi động hạ tầng Postgres (từ platform-core/infra)
docker compose -p platform-core up -d postgres-engagement

# 2. Cấu hình file .env
cp .env.example .env

# 3. Chạy test suite toàn diện
go test -v -race ./...

# 4. Chạy service cục bộ
go run cmd/server/main.go
```

