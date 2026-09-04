# team-notification — In-App Notification Center (Go)

Dịch vụ `team-notification` quản lý trung tâm thông báo người dùng (In-App Notification Center), lưu trữ lịch sử thông báo liên quan đến đơn hàng (Orders), khuyến mãi (Promotions), hệ thống (System) và hội thoại (Chat), đồng thời cung cấp các thao tác quản lý số thông báo chưa đọc (Unread Badge Counter) và đánh dấu đã đọc (Mark as Read).

Dịch vụ tuân thủ nguyên tắc **Database-per-service (Rule 3)**, sở hữu database `notification_db` riêng biệt và cung cấp gRPC service `platform.notification.v1.NotificationService` trên port `:50058`.

---

## 1. Kiến trúc & Tổng quan (Architecture Overview)

```
                    ┌──────────────────┐
                    │   team-gateway   │ (Connect Edge :8080)
                    └────────┬─────────┘
                             │ gRPC (:50058)
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                       team-notification                         │
│                                                                 │
│  ┌───────────────────────┐         ┌─────────────────────────┐  │
│  │  NotificationService  │         │   Event Consumer /      │  │
│  │ (ListNotifications,   │         │   Notification Creator  │  │
│  │  MarkAsRead,          │         │ (Order, Chat, Promo)    │  │
│  │  GetUnreadCount)      │         │                         │  │
│  └───────────┬───────────┘         └────────────▲────────────┘  │
└──────────────┼──────────────────────────────────┼───────────────┘
               │                                  │
               ▼                                  │ (Async Kafka Topics)
     ┌───────────────────┐                        │
     │    Postgres DB    │              ┌─────────┴─────────┐
     │(notification_db   │              │  Kafka / Redpanda │
     │      :5440)       │              └───────────────────┘
     └───────────────────┘
```

### Ranh giới trách nhiệm & Đặc điểm (Bounded Context)
1. **Quản lý danh sách thông báo (Notification Inbox):** Lưu trữ thông báo theo `user_id` với phân loại `NotificationType` (Order, Promotion, System, Chat).
2. **Bộ đếm Badge (Unread Count):** Truy vấn nhanh số lượng thông báo chưa đọc (`is_read = false`) để hiển thị badge chuông thông báo trên Header giao diện người dùng.
3. **Đánh dấu đã đọc linh hoạt (Flexible Mark as Read):** Hỗ trợ đánh dấu 1 thông báo cụ thể theo `id` hoặc đánh dấu tất cả thông báo của user thành đã đọc khi `id` để trống.
4. **Mở rộng tương lai (Event-Driven Ingestion):** Sẵn sàng lắng nghe các event từ `order.events` (đổi trạng thái đơn hàng), `chat.events` (tin nhắn mới) để tự động sinh thông báo tương ứng.

---

## 2. Cấu trúc thư mục (Repository Structure)

```
team-notification/
├── cmd/
│   └── server/
│       └── main.go                 # Entrypoint khởi tạo server gRPC & grace shutdown
├── generated/                      # Proto stubs từ platform-core/proto
│   └── platform/
│       ├── common/v1/              # Common proto (Principal, PageRequest, etc.)
│       └── notification/v1/        # NotificationService proto & gRPC stubs
├── internal/
│   ├── config/
│   │   ├── config.go               # Struct cấu hình nạp từ ENV (GRPC_PORT, DATABASE_URL, KAFKA_BROKER)
│   │   └── config_test.go          # Test kiểm tra load cấu hình
│   ├── grpcserver/
│   │   └── server.go               # Cấu hình gRPC Server
│   ├── handler/
│   │   ├── notification.go         # gRPC Handler cho NotificationService
│   │   └── notification_test.go    # Unit test handler
│   └── repository/
│       └── notification.go         # Interface & PostgreSQL implementation
└── migrations/
    └── 001_create_notifications.up.sql # Schema bảng notifications & index user_id
```

---

## 3. Database Schema & Migrations

### Bảng dữ liệu `notifications`:
- `id` (`VARCHAR(64) PRIMARY KEY`): ID thông báo (ví dụ `noti_a1b2c3d4`).
- `user_id` (`VARCHAR(64) NOT NULL`): ID người nhận thông báo.
- `title` (`VARCHAR(255) NOT NULL`): Tiêu đề thông báo.
- `body` (`TEXT NOT NULL`): Nội dung chi tiết thông báo.
- `type` (`INT NOT NULL DEFAULT 0`): Phân loại loại thông báo (`0: UNSPECIFIED`, `1: ORDER`, `2: PROMOTION`, `3: SYSTEM`, `4: CHAT`).
- `link_url` (`VARCHAR(512) NOT NULL DEFAULT ''`): Đường dẫn điều hướng khi người dùng nhấn vào thông báo (Deep-link / Web URL).
- `is_read` (`BOOLEAN NOT NULL DEFAULT FALSE`): Trạng thái đã đọc (`true` / `false`).
- `created_at` (`TIMESTAMPTZ NOT NULL DEFAULT NOW()`): Thời điểm tạo thông báo.

### Indexes:
- `idx_notifications_user_id ON notifications (user_id, created_at DESC)`: Tối ưu truy vấn danh sách thông báo mới nhất theo từng user.

---

## 4. Đặc tả API gRPC (`NotificationService`)

Tất cả RPCs định nghĩa trong package `platform.notification.v1`.

| RPC Method | Request Payload | Response Payload | Mô tả |
|---|---|---|---|
| `ListNotifications` | `page_size` (int32), `page_number` (int32) | `notifications` (repeated Notification), `total_unread` (int32) | Lấy danh sách thông báo phân trang theo user kèm tổng số thông báo chưa đọc. |
| `MarkAsRead` | `id` (string - để trống nếu muốn mark all) | `success` (bool) | Đánh dấu đã đọc cho 1 thông báo cụ thể hoặc toàn bộ thông báo của user. |
| `GetUnreadCount` | `{}` | `unread_count` (int32) | Lấy số lượng thông báo chưa đọc của user hiện tại. |

### Enum NotificationType:
- `NOTIFICATION_TYPE_UNSPECIFIED = 0`
- `NOTIFICATION_TYPE_ORDER = 1` (Thông báo cập nhật đơn hàng: xác nhận, giao hàng, hoàn tất)
- `NOTIFICATION_TYPE_PROMOTION = 2` (Voucher giảm giá, ưu đãi khuyến mãi)
- `NOTIFICATION_TYPE_SYSTEM = 3` (Bảo trì hệ thống, cảnh báo tài khoản)
- `NOTIFICATION_TYPE_CHAT = 4` (Tin nhắn mới từ shop/khách hàng)

---

## 5. Biến môi trường (Environment Variables)

| Biến môi trường | Mặc định | Ý nghĩa |
|---|---|---|
| `GRPC_PORT` | `50058` | Cổng gRPC của service |
| `DATABASE_URL` | `postgres://notification_svc:notification_pass@localhost:5440/notification_db?sslmode=disable` | Connection string PostgreSQL (`notification_db`) |
| `KAFKA_BROKER` | `localhost:19092` | Địa chỉ Kafka/Redpanda broker |

---

## 6. Hướng dẫn Chạy & Kiểm thử (Local Run & Testing)

```bash
# 1. Khởi động hạ tầng Postgres (từ platform-core/infra)
docker compose -p platform-core up -d postgres-notification

# 2. Chạy migrate dữ liệu
# Tạo bảng notifications theo migrations/001_create_notifications.up.sql

# 3. Chạy test suite
go test -v ./...

# 4. Chạy service cục bộ
go run cmd/server/main.go
```
