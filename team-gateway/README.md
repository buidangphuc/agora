# team-gateway — Connect Edge & Reverse Proxy

`team-gateway` là Edge Gateway duy nhất của nền tảng Marketplace Polyrepo. Nó phục vụ toàn bộ các contracts dịch vụ qua giao thức **Connect RPC** (hỗ trợ đồng thời gRPC, gRPC-Web và REST/JSON qua HTTP/1.1 & HTTP/2 h2c).

Theo **ARCHITECTURE Rule 1-3 & ADR-0003**:
- **Không chứa database và không chứa business logic**: Gateway chỉ đóng vai trò phân giải danh tính, rate-limit, gán timeout/retry và forward cuộc gọi tới đúng microservice gRPC phía sau.
- **Single Verification Point**: Gateway là thành phần duy nhất giải mã & xác thực JWT Bearer token, sau đó đính kèm thông tin Principal tin cậy (`x-principal-id`, `x-principal-type`, `x-principal-scopes`) vào gRPC metadata gửi cho các upstream service.
- **Frontend / Client Boundary**: Mọi truy vấn từ Web frontend, mobile client hoặc bên ngoài bắt buộc phải đi qua Gateway (cổng `:8080`).

---

## 1. Kiến trúc & Chức năng cốt lõi

```
[ Browser / Mobile Client / E2E Test Platform ]
                       │
                       │ HTTP/JSON / gRPC / Connect (Port :8080)
                       ▼
             ┌───────────────────┐
             │   team-gateway    │
             │   (Connect Edge)  │
             └─────────┬─────────┘
                       │
  ┌────────────────────┼────────────────────────┬─────────────────────┐
  │ :50053             │ :50051                 │ :50052              │ :50054
  ▼                    ▼                        ▼                     ▼
team-identity        team-domain              team-search           team-engagement
(Auth, Address)      (Listing, Inventory)     (OpenSearch Read)     (Fav, Review, Stats)
  │                    │                        │                     │
  ├────────────────────┼────────────────────────┴─────────────────────┘
  │ :50055             │ :50056                 │ :50057
  ▼                    ▼                        ▼
team-order           team-payment             team-chat
(Cart, Order Saga)   (Transactions, Payout)   (Real-time Threads)
```

### Pipeline Interceptors (Thứ tự thực thi)
1. **Request ID (`e.requestIDInterceptor`)**: Đảm bảo mọi request đều có `X-Request-Id` (tạo mới UUID nếu client không gửi), đồng thời echo lại ở response header.
2. **Edge Auth Resolution (`e.authInterceptor`)**: Giải mã HS256 JWT token từ header `Authorization: Bearer <token>`. Nếu hợp lệ -> gán danh tính Principal. Nếu không có hoặc token sai -> gán `anonymous` với các scope công khai (`PUBLIC_SCOPES`).
3. **Structured Logging (`e.loggingInterceptor`)**: Ghi log structured JSON cho mỗi request (method, principal, HTTP/gRPC status code, latency ms, request id).
4. **Token Bucket Rate Limiting (`e.rateLimitInterceptor`)**: Rate limit trên từng Principal ID (đối với user đã đăng nhập) hoặc trên từng Client IP (đối với anonymous). Vượt ngưỡng sẽ trả về HTTP 429 (`ResourceExhausted`).

### Metadata Forwarding (`x-principal-*`)
Gateway xây mới toàn bộ gRPC Outgoing Context trước khi gọi sang upstream, ngăn chặn việc client giả lập header:
- `x-principal-id`: ID của user/subject (hoặc `anonymous`).
- `x-principal-type`: Loại principal (`user`, `seller`, `admin`, `anonymous`).
- `x-principal-scopes`: Danh sách quyền phân tách bằng dấu phẩy (vd: `listing:read,listing:write,order:create`).
- `x-request-id`: Trace request id duy nhất xuyên suốt hệ thống.

---

## 2. Bảng định tuyến 8 Upstream Services

Gateway định tuyến và chuyển tiếp các RPC call đến 8 Go gRPC Services nội bộ:

| Service gRPC Contract | Upstream Target | Default Port | Các RPC Endpoints chính |
|---|---|---|---|
| `platform.identity.v1.AuthService` | `team-identity` | `:50053` | `Register`, `Login` |
| `platform.identity.v1.AddressService` | `team-identity` | `:50053` | `ListAddresses`, `CreateAddress`, `UpdateAddress`, `DeleteAddress`, `SetDefaultAddress` |
| `platform.listing.v1.ListingService` | `team-domain` | `:50051` | `GetListing`, `ListListings`, `ListMyListings`, `CreateListing`, `UpdateListing`, `DeleteListing`, `GetImageUploadUrl`, `ListCategories`, `GetCategory`, `ReserveStock`, `ReleaseStock` |
| `platform.search.v1.SearchService` | `team-search` | `:50052` | `SearchListings` (OpenSearch multi-filter), `Suggest` (autocomplete) |
| `platform.engagement.v1.EngagementService` | `team-engagement` | `:50054` | `AddFavorite`, `RemoveFavorite`, `IsFavorite`, `ListFavorites`, `RecordView`, `GetListingStats`, `CreateReview`, `ListReviews`, `GetListingRatingSummary` |
| `platform.order.v1.CartService` | `team-order` | `:50055` | `GetCart`, `AddToCart`, `UpdateCartItem`, `RemoveFromCart`, `ClearCart` |
| `platform.order.v1.OrderService` | `team-order` | `:50055` | `CreateOrder`, `GetOrder`, `ListBuyerOrders`, `ListSellerOrders`, `UpdateOrderStatus`, `CancelOrder`, `GetSagaState`, `ForceFailSaga` |
| `platform.payment.v1.PaymentService` | `team-payment` | `:50056` | `CreatePayment`, `GetPayment`, `ProcessMockPayment` |
| `platform.chat.v1.ChatService` | `team-chat` | `:50057` | `GetOrCreateThread`, `ListThreads`, `GetThreadMessages`, `SendMessage`, `MarkThreadRead` |

---

## 3. Real-time SSE & Cockpit Telemetry

Bên cạnh Connect RPC handlers, Gateway cung cấp 3 endpoint HTTP đặc thù:

### 1. Healthcheck: `GET /healthz`
- Trả về `200 OK` ("ok") cho container liveness & readiness probe.

### 2. Live SSE Multiplexer: `GET /api/events/live?room=<room_name>`
- Cung cấp Server-Sent Events (SSE) theo cơ chế phòng (room-based PubSub `RealtimeBroker`):
  - `listing:<id>`: Flash sale stock update, giá thay đổi theo thời gian thực.
  - `chat:<thread>`: Tin nhắn trực tiếp giữa Buyer và Seller.
  - `user:<id>`: Thông báo đơn hàng, khuyến mãi, chuông thông báo.
  - `ops:orders`: Live order ticker cho Admin Dashboard.
- Hỗ trợ auto-heartbeat định kỳ mỗi 15 giây.

### 3. Cockpit Metrics HUD: `GET /api/admin/metrics`
- Cung cấp tổng hợp số liệu telemtry trực tiếp phục vụ trang **Admin Cockpit HUD**:
  - `total_rps`, `avg_latency_ms`, `total_orders_24h`, `total_revenue_24h`.
  - Matrix trạng thái chi tiết của 10 services (team-gateway, team-domain, team-search, team-identity, team-engagement, team-order, team-payment, team-chat, team-notification, team-ai) với RPS, P95/P99 latency, Error rate.
  - Danh sách distributed traces mẫu đính kèm liên kết Jaeger UI.

---

## 4. Cấu hình biến môi trường (`.env`)

| Biến môi trường | Mặc định | Ý nghĩa |
|---|---|---|
| `HTTP_HOST` | `0.0.0.0` | Địa chỉ bind của HTTP Server |
| `HTTP_PORT` | `8080` | Cổng lắng nghe Connect Gateway |
| `JWT_SECRET` | `secret` | Khóa bí mật HMAC-SHA256 (phải khớp với `team-identity`) |
| `PUBLIC_SCOPES` | `listing.read,search:read` | Scopes mặc định gán cho anonymous caller |
| `RATE_LIMIT_RPS` | `20` | Số lượng request tối đa mỗi giây cho mỗi client |
| `RATE_LIMIT_BURST` | `40` | Dung lượng burst tối đa của Token Bucket |
| `CALL_TIMEOUT_SECONDS` | `5` | Deadline tối đa cho mỗi upstream call |
| `RETRY_MAX` | `2` | Số lần thử lại tối đa đối với các thao tác đọc idempotent khi gặp lỗi `Unavailable` |
| `CORS_ORIGINS` | `http://localhost:3000` | Whitelist danh sách CORS origins cho phép |
| `UPSTREAM_SEARCH_ADDR` | `localhost:50052` | Địa chỉ gRPC của `team-search` |
| `UPSTREAM_LISTING_ADDR` | `localhost:50051` | Địa chỉ gRPC của `team-domain` |
| `UPSTREAM_IDENTITY_ADDR` | `localhost:50053` | Địa chỉ gRPC của `team-identity` |
| `UPSTREAM_ENGAGEMENT_ADDR` | `localhost:50054` | Địa chỉ gRPC của `team-engagement` |
| `UPSTREAM_ORDER_ADDR` | `localhost:50055` | Địa chỉ gRPC của `team-order` |
| `UPSTREAM_PAYMENT_ADDR` | `localhost:50056` | Địa chỉ gRPC của `team-payment` |
| `UPSTREAM_CHAT_ADDR` | `localhost:50057` | Địa chỉ gRPC của `team-chat` |

---

## 5. Hướng dẫn chạy và Kiểm thử

### Chạy trực tiếp (Local Go)
```bash
# 1. Chuẩn bị file môi trường
cp .env.example .env

# 2. Sinh mã nguồn từ proto contracts
make proto

# 3. Kiểm tra drift biến môi trường, định dạng code và unit test
make check

# 4. Chạy service Gateway
make run
```

### Ví dụ gọi API qua REST/JSON (Connect protocol)

**Tìm kiếm sản phẩm:**
```bash
curl -X POST http://localhost:8080/platform.search.v1.SearchService/SearchListings \
  -H "Content-Type: application/json" \
  -d '{"query":"áo thun","filters":{"status":"published"},"pageSize":10}'
```

**Thêm vào giỏ hàng (kèm JWT Token):**
```bash
curl -X POST http://localhost:8080/platform.order.v1.CartService/AddToCart \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <YOUR_BUYER_JWT_TOKEN>" \
  -d '{"listingId":"prod-101","quantity":2,"unitPrice":149000}'
```

**Lấy dữ liệu Cockpit HUD:**
```bash
curl -X GET http://localhost:8080/api/admin/metrics
```
