# team-order — Go Order & Shopping Cart Microservice

Microservice quản lý Giỏ hàng (Shopping Cart), Đặt hàng (Purchase Saga Orchestrator), Vận chuyển (Logistics & SPX/GHN/GHTK Tracking), và Đổi trả hoàn tiền (RMA Returns) trong kiến trúc Polyrepo E-Commerce.

Dịch vụ chạy trên Go 1.22, cung cấp gRPC API (:50055), quản lý cơ sở dữ liệu riêng biệt `order_db` qua PostgreSQL, và tuân thủ chặt chẽ các nguyên tắc kiến trúc platform (Rule 3: DB-per-service, Saga Orchestration, Zero-Trust Principal Forwarding).

---

## 1. Vai trò & Kiến trúc Tổng quan

`team-order` giữ vai trò trọng tâm trong luồng mua sắm và sau mua sắm:
- **CartService**: Quản lý giỏ hàng của từng buyer theo nguyên tắc Real-time Upsert & Subtotal calculation.
- **OrderService (4-Step Purchase Saga)**: Điều phối đặt hàng nhiều người bán (multi-vendor split), gọi gRPC `ReserveStock` sang `team-domain`, ghi nhận đơn hàng, và sẵn sàng bù trừ hoàn tác tồn kho (`ReleaseStock`) khi thất bại.
- **RMA Service (Return & Refund Management)**: Cho phép người mua yêu cầu trả hàng/hoàn tiền và người bán xét duyệt.
- **Shipment & Logistics Service**: Mô phỏng đơn vị vận chuyển (SPX, GHN, GHTK), cấp mã vận đơn, quản lý checkpoint lộ trình di chuyển.

```
                    ┌─────────────────────────┐
                    │ team-gateway (:8080)    │
                    └───────────┬─────────────┘
                                │ gRPC (:50055)
                                ▼
                    ┌─────────────────────────┐
                    │ team-order Microservice │
                    └───────┬─────────┬───────┘
          gRPC ReserveStock │         │ gRPC Address Lookup
                            ▼         ▼
┌─────────────────────────────┐     ┌───────────────────────────────┐
│ team-domain (:50051)        │     │ team-identity (:50053)        │
└─────────────────────────────┘     └───────────────────────────────┘
```

---

## 2. Database Schema & Migrations

Dịch vụ sử dụng PostgreSQL (`order_db`) với 3 migration up/down hoàn chỉnh:

### Danh sách Migrations (`migrations/`)
1. **`0001_initial.up.sql`**: Khởi tạo bảng `cart_items`, `orders`, `order_items`.
2. **`0002_shipping_and_payment.up.sql`**: Thêm cột `shipping_fee`, `items_subtotal`, `payment_method` vào bảng `orders`.
3. **`0003_returns_and_shipments.up.sql`**: Khởi tạo bảng `order_returns`, `shipments`, `shipment_checkpoints`.

### Cấu trúc Bảng & Chỉ mục

#### Bảng `cart_items`
Lưu trữ các sản phẩm trong giỏ hàng của buyer:
| Cột | Kiểu | Ràng buộc | Mô tả |
|---|---|---|---|
| `id` | `TEXT` | `PRIMARY KEY` | Định danh mục giỏ hàng (UUID) |
| `user_id` | `TEXT` | `NOT NULL` | ID người dùng sở hữu |
| `listing_id` | `TEXT` | `NOT NULL` | ID sản phẩm niêm yết |
| `variant_id` | `TEXT` | `NOT NULL DEFAULT ''` | ID biến thể sản phẩm |
| `quantity` | `INT` | `NOT NULL DEFAULT 1` | Số lượng chọn mua |
| `unit_price` | `BIGINT` | `NOT NULL` | Đơn giá tại thời điểm thêm |
| `title` | `TEXT` | `NOT NULL` | Tiêu đề sản phẩm |
| `variant_name` | `TEXT` | `NOT NULL DEFAULT ''` | Tên biến thể (màu sắc/kích cỡ) |
| `image_url` | `TEXT` | `NOT NULL DEFAULT ''` | Ảnh đại diện |
| `seller_id` | `TEXT` | `NOT NULL` | ID người bán |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT now()` | Thời gian tạo và cập nhật |
- *Unique Constraint*: `UNIQUE (user_id, listing_id, variant_id)`
- *Index*: `idx_cart_items_user` trên `(user_id)`

#### Bảng `orders`
Lưu trữ đơn đặt hàng (1 đơn hàng tương ứng 1 seller):
| Cột | Kiểu | Ràng buộc | Mô tả |
|---|---|---|---|
| `id` | `TEXT` | `PRIMARY KEY` | Mã đơn hàng (UUID) |
| `buyer_id` | `TEXT` | `NOT NULL` | ID người mua |
| `seller_id` | `TEXT` | `NOT NULL` | ID người bán |
| `status` | `INT` | `NOT NULL DEFAULT 1` | 1: PENDING, 2: PAID, 3: SHIPPED, 4: COMPLETED, 5: CANCELLED |
| `total_amount` | `BIGINT` | `NOT NULL` | Tổng tiền thanh toán (items + shipping) |
| `items_subtotal` | `BIGINT` | `NOT NULL DEFAULT 0` | Tổng giá trị hàng hóa |
| `shipping_fee` | `BIGINT` | `NOT NULL DEFAULT 0` | Phí giao hàng |
| `payment_method` | `INT` | `NOT NULL DEFAULT 1` | 1: COD, 2: MOMO, 3: BANK, 4: CARD |
| `currency` | `TEXT` | `NOT NULL DEFAULT 'VND'` | Đơn vị tiền tệ |
| `shipping_address` | `JSONB` | `NOT NULL DEFAULT '{}'` | Địa chỉ giao hàng snapshot |
| `tracking_number` | `TEXT` | `NOT NULL DEFAULT ''` | Mã vận đơn giao vận |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT now()` | Thời gian tạo và cập nhật |
- *Indexes*: `orders_buyer_idx (buyer_id)`, `orders_seller_idx (seller_id)`

#### Bảng `order_items`
Chi tiết sản phẩm nằm trong từng đơn hàng:
| Cột | Kiểu | Ràng buộc | Mô tả |
|---|---|---|---|
| `id` | `TEXT` | `PRIMARY KEY` | ID mục đơn hàng |
| `order_id` | `TEXT` | `NOT NULL REFERENCES orders(id) ON DELETE CASCADE` | Thuộc đơn hàng nào |
| `listing_id` | `TEXT` | `NOT NULL` | Mã listing |
| `variant_id` | `TEXT` | `NOT NULL DEFAULT ''` | Mã phân loại hàng |
| `title` | `TEXT` | `NOT NULL` | Tên sản phẩm |
| `variant_name` | `TEXT` | `NOT NULL DEFAULT ''` | Tên phân loại |
| `quantity` | `INT` | `NOT NULL` | Số lượng mua |
| `unit_price` | `BIGINT` | `NOT NULL` | Đơn giá mua |
| `image_url` | `TEXT` | `NOT NULL DEFAULT ''` | Hình ảnh snapshot |
- *Index*: `order_items_order_idx (order_id)`

#### Bảng `order_returns` (RMA)
| Cột | Kiểu | Ràng buộc | Mô tả |
|---|---|---|---|
| `id` | `TEXT` | `PRIMARY KEY` | Mã yêu cầu trả hàng (UUID) |
| `order_id` | `TEXT` | `NOT NULL REFERENCES orders(id) ON DELETE CASCADE` | Mã đơn hàng cần hoàn |
| `buyer_id` | `TEXT` | `NOT NULL` | Người mua khiếu nại |
| `seller_id` | `TEXT` | `NOT NULL` | Người bán thụ lý |
| `reason` | `TEXT` | `NOT NULL` | Lý do trả hàng/hoàn tiền |
| `refund_amount`| `BIGINT` | `NOT NULL` | Số tiền đề nghị hoàn trả |
| `status` | `INT` | `NOT NULL DEFAULT 1` | 1: PENDING, 2: APPROVED, 3: REJECTED, 4: REFUNDED |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT now()` | Thời gian tạo và cập nhật |
- *Indexes*: `order_returns_order_idx`, `order_returns_buyer_idx`, `order_returns_seller_idx`

#### Bảng `shipments` & `shipment_checkpoints` (Logistics Tracking)
- **`shipments`**: `id`, `order_id` (FK), `carrier` (SPX, GHN, GHTK), `tracking_code` (UNIQUE), `status` (1: PENDING, 2: PICKED_UP, 3: IN_TRANSIT, 4: DELIVERED, 5: FAILED), `created_at`, `updated_at`.
- **`shipment_checkpoints`**: `id`, `shipment_id` (FK), `timestamp`, `location`, `description`, `created_at`.

---

## 3. Danh mục gRPC Services & RPC Methods

### A. `platform.order.v1.CartService`
1. **`GetCart(GetCartRequest) returns (GetCartResponse)`**
   - Payload vào: Trống (Lấy `buyer_id` từ caller principal).
   - Payload ra: `Cart` object gồm danh sách `items` và `subtotal`.
2. **`AddToCart(AddToCartRequest) returns (AddToCartResponse)`**
   - Payload: `listing_id` (bắt buộc), `variant_id` (tùy chọn), `quantity`.
   - Hành vi: Gọi `team-domain.GetListing` lấy thông tin chính xác, snapshot giá/ảnh, lưu vào giỏ hàng.
3. **`UpdateCartItem(UpdateCartItemRequest) returns (UpdateCartItemResponse)`**
   - Payload: `item_id`, `quantity`. Nếu `quantity <= 0`, mục tự động bị xóa.
4. **`RemoveFromCart(RemoveFromCartRequest) returns (RemoveFromCartResponse)`**
   - Payload: `item_id`.
5. **`ClearCart(ClearCartRequest) returns (ClearCartResponse)`**
   - Payload: Trống. Làm trống toàn bộ giỏ hàng của user.

---

### B. `platform.order.v1.OrderService`

#### 1. Đặt hàng & Vòng đời Đơn hàng
- **`CreateOrder(CreateOrderRequest) returns (CreateOrderResponse)`**
  - Payload vào: `address_id` (hoặc rỗng để lấy địa chỉ mặc định), `item_ids` (chọn lọc hoặc toàn bộ giỏ), `payment_method` (COD, MOMO, BANK, CARD).
  - Luồng 4-Step Saga:
    1. Lấy thông tin giỏ hàng & địa chỉ giao hàng (`team-identity`).
    2. Tách đơn hàng theo từng `seller_id` (Multi-vendor Split).
    3. Gọi gRPC `ReserveStock` đồng thời sang `team-domain` cho tất cả mặt hàng. Nếu hết hàng -> Rollback nhả lại toàn bộ hàng đã khóa và trả về lỗi `ResourceExhausted`.
    4. Tạo bản ghi đơn hàng với trạng thái `ORDER_STATUS_PENDING`, tính phí vận chuyển theo khu vực, dọn sạch giỏ hàng.
- **`GetOrder(GetOrderRequest) returns (GetOrderResponse)`**: Xem chi tiết đơn hàng (chỉ Buyer, Seller hoặc Admin).
- **`ListBuyerOrders(ListBuyerOrdersRequest) returns (ListBuyerOrdersResponse)`**: Danh sách đơn mua của Buyer (hỗ trợ `status_filter`).
- **`ListSellerOrders(ListSellerOrdersRequest) returns (ListSellerOrdersResponse)`**: Danh sách đơn bán của Seller (hỗ trợ `status_filter`).
- **`UpdateOrderStatus(UpdateOrderStatusRequest) returns (UpdateOrderStatusResponse)`**: Cập nhật trạng thái (`PAID`, `SHIPPED`, `COMPLETED`, `CANCELLED`) và `tracking_number`.
- **`CancelOrder(CancelOrderRequest) returns (CancelOrderResponse)`**: Hủy đơn hàng và tự động phát lệnh `ReleaseStock` bù trừ tồn kho sang `team-domain`.
- **`CalculateShippingFee(CalculateShippingFeeRequest) returns (CalculateShippingFeeResponse)`**:
  - Miễn phí vận chuyển cho đơn hàng $\ge$ 500.000 VND.
  - Nội thành HN / HCM: 20.000 VND.
  - Toàn quốc / Liên tỉnh: 35.000 VND.

#### 2. Saga Orchestration & Testability (P3 Pillar)
- **`GetSagaState(GetSagaStateRequest) returns (GetSagaStateResponse)`**:
  - Trả về tiến trình trực quan của Saga: `1. Order Created` -> `2. Stock Reserved` -> `3. Payment Charged` -> `4. Order Confirmed` hoặc `Compensation Executed (ReleaseStock)`.
- **`ForceFailSaga(ForceFailSagaRequest) returns (ForceFailSagaResponse)`**:
  - Giả lập lỗi ở bước thanh toán / giao vận để kích hoạt giao dịch bù trừ (Compensating Transaction) phục vụ test E2E độ tin cậy.

#### 3. RMA (Return & Refund Management)
- **`CreateReturnRequest(CreateReturnRequestRequest) returns (CreateReturnRequestResponse)`**:
  - Người mua gửi yêu cầu trả hàng/hoàn tiền (`order_id`, `reason`, `refund_amount`).
  - Ràng buộc: Đơn hàng không ở trạng thái Pending/Cancelled, số tiền hoàn $\le$ tổng đơn.
- **`GetReturnRequest(GetReturnRequestRequest) returns (GetReturnRequestResponse)`**: Lấy thông tin RMA.
- **`UpdateReturnStatus(UpdateReturnStatusRequest) returns (UpdateReturnStatusResponse)`**:
  - Người bán hoặc Admin duyệt (`APPROVED`), từ chối (`REJECTED`), hoặc xác nhận hoàn tiền (`REFUNDED`).

#### 4. Shipment & Logistics Tracking
- **`CreateShipment(CreateShipmentRequest) returns (CreateShipmentResponse)`**:
  - Tạo vận đơn cho đơn vị vận chuyển (SPX, GHN, GHTK), sinh `tracking_code`, khởi tạo checkpoint đầu tiên tại trung tâm phân loại, tự động chuyển đơn sang `ORDER_STATUS_SHIPPED`.
- **`GetShipmentTracking(GetShipmentTrackingRequest) returns (GetShipmentTrackingResponse)`**:
  - Tra cứu vận đơn bằng `tracking_code`, `order_id`, hoặc `shipment_id`, bao gồm toàn bộ danh sách `checkpoints`.

---

## 4. Cấu hình Môi trường (`internal/config`)

| Biến Môi Trường | Mặc định | Ý nghĩa |
|---|---|---|
| `ENV` | `local` | Môi trường chạy (`local`, `prod`) |
| `LOG_LEVEL` | `info` | Mức ghi log (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `true` | Định dạng log JSON hoặc text |
| `GRPC_HOST` | `0.0.0.0` | Địa chỉ lắng nghe gRPC |
| `GRPC_PORT` | `50055` | Cổng gRPC của team-order |
| `GRPC_REFLECTION_ENABLED`| `true` | Bật gRPC Reflection |
| `SHUTDOWN_GRACE_SECONDS` | `10` | Thời gian chờ dừng dịu |
| `DATABASE_ENABLED` | `true` | Bật kết nối PostgreSQL |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5433/order_db?sslmode=disable` | Chuỗi kết nối DB |
| `DB_MAX_CONNS` | `10` | Số lượng connection tối đa trong pool |
| `UPSTREAM_DOMAIN_ADDR` | `localhost:50051` | Địa chỉ gRPC của `team-domain` (để Reserve/Release Stock) |
| `UPSTREAM_IDENTITY_ADDR`| `localhost:50053` | Địa chỉ gRPC của `team-identity` (để lấy danh sách địa chỉ) |
| `OTEL_ENABLED` | `false` | Kích hoạt OpenTelemetry tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `""` | OTLP Collector endpoint |

---

## 5. Danh mục Kiểm thử (Test Suites)

Tất cả các tầng đều có unit test và in-memory fake test đầy đủ:
1. **`internal/config/config_test.go`**: Kiểm tra parsing biến môi trường, validation, drift check.
2. **`internal/repository/order_test.go`**: Kiểm thử CRUD `OrderRepository` trên PostgreSQL & InMemory.
3. **`internal/repository/cart_test.go`**: Kiểm thử Upsert, Delete, Clear giỏ hàng.
4. **`internal/repository/return_test.go`**: Kiểm thử khởi tạo và chuyển đổi trạng thái RMA request.
5. **`internal/repository/shipment_test.go`**: Kiểm thử cấp mã vận đơn, ghi nhận checkpoints lộ trình giao vận.
6. **`internal/service/order_test.go`**: Kiểm thử nghiệp vụ tính phí ship, 4-step Saga stock rollback, RMA flow.
7. **`internal/handler/order_test.go`**: Kiểm thử gRPC handlers (CreateOrder, CalculateShippingFee, GetSagaState, ForceFailSaga, RMA RPCs, Shipment RPCs).
8. **`internal/grpcserver/server_test.go`**: In-process gRPC E2E test với ephemeral port và health check.
