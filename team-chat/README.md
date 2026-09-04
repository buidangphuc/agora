# team-chat — Buyer ↔ Seller 1:1 Messaging & Real-Time Event Streaming (Go)

Dịch vụ `team-chat` chịu trách nhiệm toàn bộ tính năng nhắn tin 1:1 trực tiếp giữa Người mua (Buyer) và Nhà bán hàng (Seller), quản lý hội thoại gắn với sản phẩm (Listing Context), đồng bộ số tin nhắn chưa đọc (Unread Counters) và bắn sự kiện thời gian thực (Real-time Event Streaming) qua Kafka / Redpanda cho Server-Sent Events (SSE) tại API Gateway.

Dịch vụ tuân thủ nguyên tắc **Database-per-service (Rule 3)**, sở hữu database `chat_db` riêng biệt và cung cấp gRPC service `platform.chat.v1.ChatService` trên port `:50057`.

---

## 1. Kiến trúc & Luồng dữ liệu (Architecture & Data Flow)

```
  Browser (Buyer / Seller)
             │  (Connect-ES / SSE Stream)
             ▼
     ┌───────────────┐
     │ team-gateway  │ (Connect Edge :8080)
     └───────┬───────┘
             │ gRPC (:50057)
             ▼
┌─────────────────────────────────────────────────────────────────┐
│                           team-chat                             │
│                                                                 │
│  ┌───────────────────────┐         ┌─────────────────────────┐  │
│  │     ChatService       │         │    ChatPublisher        │  │
│  │ (GetOrCreateThread,   │────────▶│ (Wrap EventEnvelope     │  │
│  │  SendMessage, etc.)   │         │  Key = thread_id)       │  │
│  └───────────┬───────────┘         └────────────┬────────────┘  │
└──────────────┼──────────────────────────────────┼───────────────┘
               │                                  │
               ▼                                  ▼
     ┌───────────────────┐              ┌───────────────────┐
     │    Postgres DB    │              │  Kafka / Redpanda │
     │  (chat_db :5432)  │              │   (chat.events)   │
     └───────────────────┘              └─────────┬─────────┘
                                                  │
                                                  ▼ (SSE Push to Clients)
                                        ┌───────────────────┐
                                        │   team-gateway    │
                                        └───────────────────┘
```

### Ranh giới trách nhiệm & Đặc điểm thiết kế (Bounded Context)
1. **Per-Thread Ordering & Aggregate Key:** Mỗi cuộc trò chuyện gắn với `(buyer_id, seller_id, listing_id)`. Khi phát sinh tin nhắn mới, sự kiện được publish lên Kafka topic `chat.events` với message key = `thread_id`, đảm bảo thứ tự tin nhắn tuyệt đối theo từng thread.
2. **Event Envelope (ADR-0002):** Tất cả sự kiện domain `platform.chat.v1.ChatMessage` được bọc trong `platform.events.v1.EventEnvelope` mang đầy đủ context: `event_id`, `type`, `occurred_at`, `principal`, `traceparent`, `request_id`, và `payload` dạng bytes.
3. **Phân quyền truy cập hội thoại (Authorization Guard):** Người gọi chỉ có thể đọc tin nhắn hoặc gửi tin nhắn vào thread nếu chính họ là `buyer_id` hoặc `seller_id` của thread đó (tránh lộ thông tin giữa các người dùng).
4. **Chống tự nhắn tin (Self-Chat Prevention):** Ngăn chặn người dùng tự tạo thread nhắn tin với chính mình (`buyer_id == seller_id`).
5. **Đồng bộ trạng thái đã đọc (Read Status Management):** Phân tách 2 bộ đếm `unread_count_buyer` và `unread_count_seller` riêng biệt. Khi có tin nhắn từ Buyer -> `unread_count_seller` tăng và ngược lại. Khi gọi `MarkThreadRead`, hệ thống chỉ reset biến đếm của bên đang gọi.

---

## 2. Cấu trúc thư mục (Repository Structure)

```
team-chat/
├── cmd/
│   └── server/
│       └── main.go                 # Khởi động server gRPC, kết nối PG & Kafka
├── generated/                      # Proto stubs sinh ra từ platform-core/proto
│   └── platform/
│       ├── chat/v1/                # ChatService proto & gRPC stubs
│       ├── common/v1/              # Common models (Principal, PageRequest, etc.)
│       └── events/v1/              # EventEnvelope proto
├── internal/
│   ├── bootstrap/
│   │   └── resources.go            # Khởi tạo kết nối DB & Kafka client
│   ├── config/
│   │   └── config.go               # Struct cấu hình nạp từ ENV (Server, Postgres, Kafka, OTEL)
│   ├── events/
│   │   ├── publisher.go            # KafkaPublisher emit EventEnvelope & NoopPublisher
│   │   └── publisher_test.go       # Unit test cho Kafka publisher
│   ├── grpcserver/
│   │   ├── server.go               # Cấu hình gRPC Server & Interceptors
│   │   └── server_test.go          # Test khởi động gRPC server
│   ├── handler/
│   │   ├── chat.go                 # gRPC Handler chuyển tiếp & publish Kafka event
│   │   └── chat_test.go            # Test toàn diện các trường hợp (Auth, Self-chat, Send, Event, Read)
│   ├── interceptor/
│   │   ├── auth.go                 # Trích xuất Principal từ Metadata
│   │   └── tracing.go              # OpenTelemetry trace context propagation
│   ├── observability/
│   │   └── tracer.go               # Tracer provider OpenTelemetry
│   ├── repository/
│   │   ├── chat.go                 # Interface, Postgres & In-Memory chat repository
│   │   └── chat_test.go            # Unit test logic lưu trữ thread/tin nhắn
│   └── service/
│       ├── chat.go                 # Business rules (SelfChat, Unauthorized, Validation)
│       └── chat_test.go            # Unit test tầng service
└── migrations/
    └── 0001_initial.up.sql         # Schema khởi tạo chat_threads & chat_messages
```

---

## 3. Database Schema & Migrations

### Bảng dữ liệu:
1. `chat_threads`
   - `id` (`VARCHAR(64) PRIMARY KEY`): UUID cuộc trò chuyện.
   - `buyer_id` (`VARCHAR(64) NOT NULL`): ID người mua.
   - `seller_id` (`VARCHAR(64) NOT NULL`): ID nhà bán hàng.
   - `listing_id` (`VARCHAR(64) NOT NULL DEFAULT ''`): ID sản phẩm đang trao đổi.
   - `listing_title` (`VARCHAR(255) NOT NULL DEFAULT ''`): Tên sản phẩm tóm tắt.
   - `listing_image_url` (`TEXT NOT NULL DEFAULT ''`): Link ảnh đại diện sản phẩm.
   - `last_message_text` (`TEXT NOT NULL DEFAULT ''`): Nội dung tin nhắn cuối cùng để hiển thị preview.
   - `last_message_at` (`TIMESTAMPTZ`): Thời điểm tin nhắn cuối cùng phát sinh.
   - `unread_count_buyer` (`INT NOT NULL DEFAULT 0`): Số tin nhắn người mua chưa đọc.
   - `unread_count_seller` (`INT NOT NULL DEFAULT 0`): Số tin nhắn người bán chưa đọc.
   - `created_at` (`TIMESTAMPTZ NOT NULL DEFAULT NOW()`): Thời gian tạo thread.
   - `updated_at` (`TIMESTAMPTZ NOT NULL DEFAULT NOW()`): Thời gian cập nhật gần nhất.
   - **Unique Constraint:** `uq_chat_threads_parties UNIQUE (buyer_id, seller_id, listing_id)`
   - **Indexes:** `idx_chat_threads_buyer (buyer_id, updated_at DESC)`, `idx_chat_threads_seller (seller_id, updated_at DESC)`

2. `chat_messages`
   - `id` (`VARCHAR(64) PRIMARY KEY`): UUID tin nhắn.
   - `thread_id` (`VARCHAR(64) NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE`): Khóa ngoại liên kết thread.
   - `sender_id` (`VARCHAR(64) NOT NULL`): ID người gửi tin nhắn.
   - `sender_name` (`VARCHAR(128) NOT NULL DEFAULT ''`): Tên hiển thị người gửi.
   - `content` (`TEXT NOT NULL`): Nội dung văn bản tin nhắn.
   - `created_at` (`TIMESTAMPTZ NOT NULL DEFAULT NOW()`): Thời điểm gửi tin nhắn.
   - **Index:** `idx_chat_messages_thread (thread_id, created_at ASC)`

---

## 4. Đặc tả API gRPC (`ChatService`)

Tất cả RPCs định nghĩa trong package `platform.chat.v1`.

| RPC Method | Request Payload | Response Payload | Yêu cầu Auth & Quyền | Mô tả |
|---|---|---|---|---|
| `GetOrCreateThread` | `seller_id` (string), `listing_id` (string) | `thread` (ChatThread) | Authenticated (User) | Tạo hoặc lấy thread hiện có giữa Buyer & Seller cho sản phẩm cụ thể. Chặn self-chat. |
| `ListThreads` | `page` (PageRequest: cursor, page_size) | `threads` (repeated ChatThread), `page` (PageResponse) | Authenticated (User) | Lấy danh sách hội thoại của user (dù đóng vai trò buyer hay seller), sắp xếp theo `updated_at DESC`. |
| `GetThreadMessages` | `thread_id` (string), `page` (PageRequest) | `messages` (repeated ChatMessage), `page` (PageResponse) | Authenticated (Chỉ thành viên thread) | Lấy lịch sử tin nhắn trong thread theo thứ tự thời gian tăng dần (`ASC`). Chặn user ngoài cuộc. |
| `SendMessage` | `thread_id` (string), `content` (string) | `message` (ChatMessage) | Authenticated (Chỉ thành viên thread) | Gửi tin nhắn mới, cập nhật `last_message` & `unread_count`, đồng thời phát sinh Kafka Event `ChatMessage` trên `chat.events`. |
| `MarkThreadRead` | `thread_id` (string) | `{}` | Authenticated (Chỉ thành viên thread) | Đánh dấu đã đọc toàn bộ tin nhắn trong thread, reset `unread_count` của người gọi về 0. |
| `StreamChat` | `session_id`, `message` | `stream StreamChatResponse` (delta, done) | Optional | Seed RPC cho tính năng trợ lý AI / streaming token. |

---

## 5. Event-Driven Architecture & Kafka Integration

Khi có tin nhắn gửi thành công qua `SendMessage`, `team-chat` phát đi sự kiện:
- **Topic:** `chat.events` (hoặc cấu hình qua `KAFKA_CHAT_TOPIC`)
- **Key:** `thread_id` (đảm bảo partition ordering)
- **Envelope:** `platform.events.v1.EventEnvelope`
  - `event_id`: UUID mới cho mỗi event.
  - `type`: `"platform.chat.v1.ChatMessage"`
  - `occurred_at`: Thời gian phát sinh.
  - `principal`: Principal của người gửi.
  - `request_id`: X-Request-ID phục vụ truy vết.
  - `payload`: Serialized protobuf bytes của `ChatMessage`.

Edge Gateway (`team-gateway`) lắng nghe topic này để đẩy sự kiện real-time xuống client qua SSE `/api/chat/stream`.

---

## 6. Biến môi trường (Environment Variables)

| Biến môi trường | Mặc định | Ý nghĩa |
|---|---|---|
| `SERVER_HOST` | `0.0.0.0` | Địa chỉ lắng nghe gRPC |
| `SERVER_PORT` | `50057` | Cổng gRPC |
| `SERVER_SHUTDOWN_GRACE_SECONDS` | `5.0` | Thời gian chờ khi tắt server |
| `SERVER_REFLECTION_ENABLED` | `true` | Bật gRPC Reflection |
| `RUNTIME_LOG_LEVEL` | `info` | Mức log |
| `RUNTIME_LOG_JSON` | `false` | Log format JSON |
| `POSTGRES_HOST` | `postgres-chat` | Host PostgreSQL |
| `POSTGRES_PORT` | `5432` | Cổng PostgreSQL |
| `POSTGRES_DB` | `chat_db` | Tên Database |
| `POSTGRES_USER` | `chat_svc` | DB User |
| `POSTGRES_PASSWORD` | `chat_pass` | DB Password |
| `POSTGRES_SSLMODE` | `disable` | SSL Mode |
| `KAFKA_ENABLED` | `false` | Kích hoạt gửi sự kiện qua Kafka |
| `KAFKA_BROKERS` | `localhost:9092` | Danh sách broker Kafka / Redpanda |
| `KAFKA_CHAT_TOPIC` | `chat.events` | Tên topic Kafka |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `""` | Địa chỉ OTLP Collector |
| `OTEL_SERVICE_NAME` | `team-chat` | Tên Service cho tracing |

---

## 7. Hướng dẫn Chạy & Kiểm thử (Local Run & Testing)

```bash
# 1. Khởi động hạ tầng Postgres & Kafka từ platform-core
docker compose -p platform-core up -d postgres-chat redpanda

# 2. Cấu hình file .env
cp .env.example .env

# 3. Chạy toàn bộ unit & integration tests
go test -v -race ./...

# 4. Chạy service cục bộ
go run cmd/server/main.go
```
