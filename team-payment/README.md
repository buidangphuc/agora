# team-payment — Go Mock Payment & Seller Wallet Microservice

Microservice quản lý Giao dịch thanh toán giả lập (Mock Payment Transactions) và Ví tiền Người bán (Seller Wallet & Payout System) trong kiến trúc Polyrepo E-Commerce.

Dịch vụ chạy trên Go 1.22, cung cấp gRPC API (:50056), quản lý cơ sở dữ liệu riêng biệt `payment_db` qua PostgreSQL, và tuân thủ chặt chẽ nguyên tắc bảo mật tài chính (Rule: *Financially sensitive => Mock only, zero real money movement*).

---

## 1. Vai trò & Kiến trúc Tổng quan

`team-payment` đóng vai trò hoàn thiện luồng thanh toán và đối soát dòng tiền người bán:
- **Payment Processing**: Tạo giao dịch thanh toán gắn với đơn hàng, hỗ trợ các hình thức Mock Payment (COD, MoMo QR, Bank Transfer, Credit/Debit Card), mô phỏng thành công/thất bại và tự động thăng cấp trạng thái đơn hàng (`team-order` -> `ORDER_STATUS_PAID`).
- **Payment Refund**: Xử lý hoàn tiền giao dịch khi có khiếu nại RMA hoặc hủy đơn.
- **Seller Wallet & Payout**: Quản lý số dư ví người bán, ghi nhận lịch sử biến động số dư (`ORDER_SETTLEMENT`, `PAYOUT`, `REFUND_DEDUCTION`), và xử lý yêu cầu rút tiền về tài khoản ngân hàng.

```
                    ┌───────────────────────────┐
                    │ team-gateway (:8080)      │
                    └─────────────┬─────────────┘
                                  │ gRPC (:50056)
                                  ▼
                    ┌───────────────────────────┐
                    │ team-payment Microservice │
                    └─────────────┬─────────────┘
                                  │ gRPC GetOrder / UpdateOrderStatus
                                  ▼
                    ┌───────────────────────────┐
                    │ team-order (:50055)       │
                    └───────────────────────────┘
```

---

## 2. Database Schema & Migrations

Dịch vụ sử dụng PostgreSQL (`payment_db`) với 2 migration up/down hoàn chỉnh:

### Danh sách Migrations (`migrations/`)
1. **`0001_initial.up.sql`**: Khởi tạo bảng `payment_transactions`.
2. **`0002_seller_wallet.up.sql`**: Khởi tạo các bảng `seller_wallets`, `payout_requests`, `wallet_transactions`.

### Cấu trúc Bảng & Chỉ mục

#### Bảng `payment_transactions`
Lưu trữ toàn bộ giao dịch thanh toán gắn với từng đơn hàng:
| Cột | Kiểu | Ràng buộc | Mô tả |
|---|---|---|---|
| `id` | `VARCHAR(64)` | `PRIMARY KEY` | Mã giao dịch thanh toán (UUID) |
| `order_id` | `VARCHAR(64)` | `NOT NULL` | Mã đơn hàng từ `team-order` |
| `buyer_id` | `VARCHAR(64)` | `NOT NULL` | ID người mua thanh toán |
| `amount` | `BIGINT` | `NOT NULL` | Số tiền thanh toán (VND) |
| `currency` | `VARCHAR(8)` | `NOT NULL DEFAULT 'VND'` | Loại tiền tệ |
| `method` | `INT` | `NOT NULL DEFAULT 1` | 1: COD, 2: MOCK_MOMO, 3: MOCK_BANK, 4: MOCK_CARD |
| `status` | `INT` | `NOT NULL DEFAULT 1` | 1: PENDING, 2: PAID, 3: FAILED, 4: REFUNDED |
| `provider_reference` | `VARCHAR(128)` | `NOT NULL DEFAULT ''` | Mã tham chiếu cổng thanh toán giả lập |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | Thời gian tạo và cập nhật |
- *Indexes*: `idx_payment_order_id (order_id)`, `idx_payment_buyer_id (buyer_id)`

#### Bảng `seller_wallets`
Lưu trữ số dư ví doanh thu của từng nhà bán hàng:
| Cột | Kiểu | Ràng buộc | Mô tả |
|---|---|---|---|
| `id` | `VARCHAR(64)` | `PRIMARY KEY` | ID ví người bán (UUID) |
| `seller_id` | `VARCHAR(64)` | `NOT NULL UNIQUE` | ID người bán sở hữu ví |
| `balance` | `BIGINT` | `NOT NULL DEFAULT 0` | Số dư hiện khả dụng (VND, không âm) |
| `currency` | `VARCHAR(8)` | `NOT NULL DEFAULT 'VND'` | Loại tiền tệ |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | Thời điểm cập nhật số dư |
- *Index*: `idx_seller_wallets_seller_id (seller_id)`

#### Bảng `payout_requests`
Lưu trữ các yêu cầu rút tiền về tài khoản ngân hàng của người bán:
| Cột | Kiểu | Ràng buộc | Mô tả |
|---|---|---|---|
| `id` | `VARCHAR(64)` | `PRIMARY KEY` | Mã yêu cầu rút tiền (UUID) |
| `seller_id` | `VARCHAR(64)` | `NOT NULL` | ID người bán yêu cầu |
| `amount` | `BIGINT` | `NOT NULL` | Số tiền muốn rút |
| `bank_code` | `VARCHAR(32)` | `NOT NULL` | Mã ngân hàng (VCB, TCB, MB, ACB, ...) |
| `account_number` | `VARCHAR(64)` | `NOT NULL` | Số tài khoản nhận tiền |
| `account_name` | `VARCHAR(128)` | `NOT NULL` | Tên chủ tài khoản |
| `status` | `INT` | `NOT NULL DEFAULT 1` | 1: PENDING, 2: PROCESSING, 3: COMPLETED, 4: REJECTED |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | Thời gian lập lệnh rút |
- *Index*: `idx_payout_requests_seller_id (seller_id)`

#### Bảng `wallet_transactions`
Ghi nhận sổ cái biến động số dư ví người bán (Audit Trail):
| Cột | Kiểu | Ràng buộc | Mô tả |
|---|---|---|---|
| `id` | `VARCHAR(64)` | `PRIMARY KEY` | Mã bản ghi biến động |
| `wallet_id` | `VARCHAR(64)` | `NOT NULL` | ID ví tương ứng |
| `amount` | `BIGINT` | `NOT NULL` | Số tiền biến động (+ tiền vào, - tiền rút) |
| `type` | `INT` | `NOT NULL` | 1: ORDER_SETTLEMENT, 2: PAYOUT, 3: REFUND_DEDUCTION |
| `reference_id` | `VARCHAR(128)` | `NOT NULL DEFAULT ''` | ID tham chiếu (mã đơn hàng / mã payout) |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | Thời gian phát sinh |
- *Indexes*: `idx_wallet_transactions_wallet_id (wallet_id)`, `idx_wallet_transactions_reference_id (reference_id)`

---

## 3. Danh mục gRPC Services & RPC Methods

### `platform.payment.v1.PaymentService`

#### 1. Xử lý Thanh toán (Payment Transactions)
- **`CreatePayment(CreatePaymentRequest) returns (CreatePaymentResponse)`**
  - Payload vào: `order_id` (bắt buộc), `method` (COD, MOCK_MOMO, MOCK_BANK, MOCK_CARD).
  - Quy trình:
    1. Gọi sang `team-order.GetOrder(order_id)` kiểm tra đơn có tồn tại và đang ở trạng thái `ORDER_STATUS_PENDING`.
    2. Kiểm tra nếu giao dịch đã tồn tại cho đơn này thì trả về luôn.
    3. Tạo bản ghi `PaymentTransaction` với trạng thái `PAYMENT_STATUS_PENDING`.
    4. Trả về thông tin giao dịch cùng URL thanh toán giả lập `payment_url: "/checkout/pay/<order_id>"`.
- **`GetPayment(GetPaymentRequest) returns (GetPaymentResponse)`**
  - Payload vào: `id` (mã giao dịch) hoặc `order_id` (mã đơn hàng).
  - Payload ra: `PaymentTransaction` chi tiết.
- **`ProcessMockPayment(ProcessMockPaymentRequest) returns (ProcessMockPaymentResponse)`**
  - Payload vào: `transaction_id`, `simulate_success` (boolean: true để mô phỏng thành công, false để mô phỏng bị từ chối).
  - Quy trình khi thành công:
    1. Cập nhật giao dịch sang `PAYMENT_STATUS_PAID`, cấp mã `MOCK-REF-<timestamp>`.
    2. Gọi gRPC sang `team-order.UpdateOrderStatus(order_id, ORDER_STATUS_PAID)` để hoàn tất bước thanh toán của đơn hàng.
  - Quy trình khi thất bại: Cập nhật giao dịch sang `PAYMENT_STATUS_FAILED`, giữ nguyên trạng thái đơn hàng.
- **`RefundPayment(RefundPaymentRequest) returns (RefundPaymentResponse)`**
  - Payload vào: `payment_id`, `amount`, `reason`.
  - Quy trình: Kiểm tra giao dịch đã thanh toán (`PAID`), số tiền hoàn $\le$ tổng tiền giao dịch, cập nhật sang `PAYMENT_STATUS_REFUNDED`.

#### 2. Ví Tiền & Rút Tiền Người Bán (Seller Wallet & Payouts)
- **`GetSellerWallet(GetSellerWalletRequest) returns (GetSellerWalletResponse)`**
  - Payload vào: `seller_id` (tùy chọn; mặc định lấy ID của người bán từ context principal).
  - Payload ra: `SellerWallet` gồm `balance`, `currency`, `updated_at` (tự động khởi tạo ví với số dư 0 nếu chưa có).
- **`RequestPayout(RequestPayoutRequest) returns (RequestPayoutResponse)`**
  - Payload vào: `seller_id`, `amount`, `bank_code`, `account_number`, `account_name`.
  - Quy trình:
    1. Trừ tiền ví `seller_wallets` an toàn bằng atomic update `balance = balance - amount WHERE balance >= amount`. Nếu không đủ số dư -> trả về `FailedPrecondition (insufficient balance)`.
    2. Tạo bản ghi yêu cầu rút tiền `payout_requests` với trạng thái `PAYOUT_STATUS_PENDING`.
    3. Ghi nhận biến động vào sổ cái `wallet_transactions` (`type = WALLET_TRANSACTION_TYPE_PAYOUT`, `amount = -amount`).
- **`ListPayoutHistory(ListPayoutHistoryRequest) returns (ListPayoutHistoryResponse)`**
  - Payload vào: `seller_id`.
  - Payload ra: Danh sách toàn bộ lịch sử yêu cầu rút tiền sắp xếp theo thời gian mới nhất.

---

## 4. Cấu hình Môi trường (`internal/config`)

| Biến Môi Trường | Mặc định | Ý nghĩa |
|---|---|---|
| `ENV` | `local` | Môi trường chạy (`local`, `prod`) |
| `LOG_LEVEL` | `info` | Mức ghi log (`debug`, `info`, `warn`, `error`) |
| `LOG_JSON` | `true` | Định dạng log JSON hoặc text |
| `GRPC_HOST` | `0.0.0.0` | Địa chỉ lắng nghe gRPC |
| `GRPC_PORT` | `50056` | Cổng gRPC của team-payment |
| `GRPC_REFLECTION_ENABLED`| `true` | Bật gRPC Reflection |
| `SHUTDOWN_GRACE_SECONDS` | `10` | Thời gian chờ dừng dịu |
| `DATABASE_ENABLED` | `true` | Bật kết nối PostgreSQL |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5436/payment_db?sslmode=disable` | Chuỗi kết nối DB |
| `DB_MAX_CONNS` | `10` | Số lượng connection tối đa trong pool |
| `UPSTREAM_ORDER_ADDR` | `localhost:50055` | Địa chỉ gRPC của `team-order` (để kiểm tra đơn và cập nhật PAID) |
| `OTEL_ENABLED` | `false` | Kích hoạt OpenTelemetry tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `""` | OTLP Collector endpoint |

---

## 5. Danh mục Kiểm thử (Test Suites)

Tất cả các tầng đều có unit test và in-memory fake test đầy đủ:
1. **`internal/config/config_test.go`**: Kiểm tra parsing cấu hình, default values, env drift.
2. **`internal/interceptor/auth_test.go`**: Kiểm tra trích xuất `x-principal-*` gRPC metadata, xác thực quyền caller.
3. **`internal/repository/payment_test.go`**: Kiểm thử CRUD và cập nhật trạng thái `PaymentTransaction`.
4. **`internal/repository/wallet_test.go`**: Kiểm thử khởi tạo ví, cập nhật số dư atomic, kiểm tra chặn số dư âm, ghi nhận payout và transaction history.
5. **`internal/service/payment_test.go`**: Kiểm thử luồng nghiệp vụ tạo thanh toán, xử lý mock payment đồng bộ sang `team-order`, hoàn tiền giao dịch, rút tiền ví người bán.
6. **`internal/handler/payment_test.go`**: Kiểm thử toàn bộ gRPC handler methods (`CreatePayment`, `GetPayment`, `ProcessMockPayment`, `GetSellerWallet`, `RequestPayout`, `ListPayoutHistory`, `RefundPayment`).
7. **`internal/grpcserver/server_test.go`**: In-process gRPC server tests với health check và lifecycle.
