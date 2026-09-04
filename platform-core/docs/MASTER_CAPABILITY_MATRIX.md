# 🌟 MASTER CAPABILITY MATRIX — Marketplace Polyrepo

Tài liệu tổng hợp toàn diện năng lực hệ thống (All Services, RPCs, Data Models, UI Routes & E2E Test Cases) được trích xuất từ `README.md` của từng repository trong Polyrepo.

---

## 🗺️ 1. Bản Đồ Dịch Vụ & Bounded Contexts

| Tầng | Repository | Port | Vai Trò & Bounded Context | Bảng Dữ Liệu (DB) |
|---|---|---|---|---|
| **Contract & Infra** | `platform-core` | — | Single source of truth (Proto v2), Docker infra, OTel telemetry | — |
| **E2E Automation** | `platform-e2e` | — | Pytest-bdd + Playwright E2E testing platform | — |
| **Edge Gateway** | `team-gateway` | :8080 | Connect Edge, JWT Verification, Rate Limit, CORS, SSE Multiplexer | In-memory token bucket |
| **Frontend SSR** | `team-frontend` | :3000 | Next.js 14 App Router, 25 routes, Storefront, Seller, Admin | Session Cookies |
| **Identity & Auth** | `team-identity` | :50053 | User accounts, bcrypt, HS256 JWT, scopes, password reset tokens | `users`, `addresses`, `password_reset_tokens` |
| **Domain (Write)** | `team-domain` | :50051 | Listing write-model, multi-variant stock holds, Kafka publisher | `listings`, `listing_variants`, `outbox_events` |
| **Search (Read)** | `team-search` | :50052 | OpenSearch CQRS consumer, multi-attribute filter & auto-suggest | OpenSearch `listings` index |
| **Order & Saga** | `team-order` | :50055 | Cart, 4-step Purchase Saga, RMA Returns, SPX Shipment tracking | `cart_items`, `orders`, `order_items`, `order_returns`, `shipments` |
| **Fintech & Payment**| `team-payment` | :50056 | MockPay transactions, refunds, seller wallet balance & payouts | `payment_transactions`, `seller_wallets`, `payout_requests`, `wallet_transactions` |
| **Community** | `team-engagement`| :50054 | Favorites, verified star reviews + photos, product Q&A, disputes| `favorites`, `listing_stats`, `reviews`, `questions`, `answers`, `disputes` |
| **Realtime Chat** | `team-chat` | :50057 | Buyer-Seller real-time chat threads & message history | `chat_threads`, `chat_messages` |
| **Notifications** | `team-notification`| :50058| In-app notification center & unread badges | `notifications` |
| **AI Intelligence** | `team-ai` | :8000 | FastAPI AI: Shopping RAG advice, Magic Listing SEO, Chat Copilot | In-memory semantic catalog |

---

## ⚡ 2. Toàn Bộ 10 Phase Nghiệp Vụ Cốt Lõi

### Phase 1: Authentication, Authorization & Identity
- **RPCs**: `Register`, `Login`, `ChangePassword`, `RequestPasswordReset`, `ResetPassword`.
- **Logic**: Bcrypt hash ($\text{cost}=10$), JWT HS256 với role-based scopes (`buyer`, `seller`, `admin`), reset token expire 15m.
- **Frontend**: `/login`, `/register`, `/account/profile`.

### Phase 2: Listing Management & Multi-Variant Catalog
- **RPCs**: `CreateListing`, `GetListing`, `UpdateListing`, `DeleteListing`, `ReserveStock`, `ReleaseStock`.
- **Logic**: Multi-variant SKU, inventory reservation atomically, outbox event publishing to Kafka `listing.events`.
- **Frontend**: `/seller/new`, `/seller/[id]/edit`, `/seller`.

### Phase 3: Search Engine & CQRS Synchronization
- **RPCs**: `SearchListings`, `SuggestKeywords`, `IndexListing`, `DeleteListingIndex`.
- **Logic**: Lexical match, multi-attribute filters (price range, category, brand, rating), sorting (price asc/desc, newest, top-rated).
- **Frontend**: `/search`, `/` (Home search bar with auto-suggest dropdown).

### Phase 4: Social Engagement, Reviews & Community Q&A
- **RPCs**: `ToggleFavorite`, `ListFavorites`, `AddReview`, `ListReviewsByListing`, `AskQuestion`, `AnswerQuestion`, `CreateDispute`, `GetDispute`.
- **Logic**: Star rating breakdown (1-5★), photo attachments, verified purchase badges, buyer-seller Q&A threads, dispute resolution.
- **Frontend**: `/listing/[id]` (Review and Q&A accordions), `/favorites`.

### Phase 5: Shopping Cart & Dynamic Pricing
- **RPCs**: `AddToCart`, `GetCart`, `UpdateCartItem`, `RemoveFromCart`, `ClearCart`.
- **Logic**: Item grouping by seller, snapshot prices, stock availability verification, voucher discount deductions.
- **Frontend**: `/cart`, Floating Cart counter.

### Phase 6: Distributed Purchase Saga & Compensation
- **RPCs**: `CreateOrder`, `GetOrder`, `ListBuyerOrders`, `ListSellerOrders`, `CancelOrder`, `GetSagaState`, `ForceFailSaga`.
- **Logic**:
  - `Step 1`: Order Created (`ORDER_STATUS_PENDING`)
  - `Step 2`: Stock Reserved via gRPC `team-domain.ReserveStock`
  - `Step 3`: Payment Charged via gRPC `team-payment.CreatePayment`
  - `Step 4`: Order Confirmed (`ORDER_STATUS_PAID`)
  - `Rollback Flow`: Khi lỗi thanh toán hoặc hủy đơn $\to$ Tự động gọi `team-domain.ReleaseStock` và chuyển trạng thái `CANCELLED`.
- **Frontend**: `/checkout`, `/account/orders`, `/checkout/pay/[id]`.

### Phase 7: Logistics & Multi-Stop SPX Shipment Tracking
- **RPCs**: `CreateShipment`, `GetShipmentTracking`, `AddShipmentCheckpoint`.
- **Logic**: Tự động sinh mã vận đơn `SPX_VN_<id>`, cập nhật timeline 5 bước: `Đặt hàng` $\to$ `Đã thanh toán` $\to$ `Đang đóng gói` $\to$ `Đang vận chuyển` $\to$ `Giao hàng thành công`.
- **Frontend**: `/account/orders/[id]`, `/seller/orders/[id]`.

### Phase 8: RMA (Return Merchandise Authorization) & Automated Refund
- **RPCs**: `CreateReturnRequest`, `GetReturnRequest`, `UpdateReturnStatus`, `RefundPayment`.
- **Logic**: Cho phép người mua gửi yêu cầu hoàn tiền $\to$ Người bán duyệt $\to$ Kích hoạt `team-payment.RefundPayment` và hoàn trả số lượng tồn kho cho người bán.
- **Frontend**: `/account/orders/[id]` (RMA Modal & Reason selector).

### Phase 9: Fintech, Seller Wallet & Bank Payout Settlement
- **RPCs**: `GetSellerWallet`, `RequestPayout`, `ListPayoutHistory`.
- **Logic**: Ví người bán lưu trữ doanh thu từ các đơn hàng thành công, trừ số dư nguyên tử (atomic decrement) và ghi nhận giao dịch sổ cái `wallet_transactions`.
- **Frontend**: `/seller/analytics` (Biểu đồ doanh thu 7 ngày, số dư ví, rút tiền ngân hàng).

### Phase 10: AI Intelligence & Admin Observability Cockpit
- **Endpoints**: `POST /api/v1/ai/assistant`, `POST /api/v1/ai/magic-listing`, `POST /api/v1/ai/chat-copilot`, `/admin/cockpit`.
- **Logic**: Trợ lý mua sắm AI RAG, tự động sinh tiêu đề SEO & mô tả sản phẩm, gợi ý câu trả lời nhanh cho shop, Dashboard đo lường trạng thái 8 services và OTel distributed traces.
- **Frontend**: `/assistant`, Floating Chat Bubble, `/admin/cockpit`.

---

## 🎯 3. Ma Trận Kiểm Thử Tích Hợp E2E (Dành cho `platform-e2e`)

| Test Code | Scenario Tương Tác | Người Dùng / Persona | Services / Routes Tham Gia | Tiêu Chuẩn Pass (Assertion) |
|---|---|---|---|---|
| **E2E-AUTH-01** | Đăng nhập hợp lệ và sinh Session JWT | Buyer / Seller | `team-identity`, `/login` | `session` cookie hợp lệ, chuyển hướng sang `/` hoặc `/seller` |
| **E2E-AUTH-02** | Đổi mật khẩu và Xác thực Reset Token | Buyer | `team-identity`, `AuthService` | Hash mật khẩu mới khớp, token hết hạn đúng chu kỳ |
| **E2E-CAT-01** | Người bán đăng sản phẩm có phân loại (Variants) | Seller | `team-domain`, `/seller/new` | Listing hiển thị trên `/seller`, event phát ra Kafka |
| **E2E-SRCH-01** | Tìm kiếm sản phẩm & Lọc theo Category/Giá | Buyer | `team-search`, `/search` | Kết quả tìm kiếm trả về sản phẩm khớp từ khóa |
| **E2E-ENG-01** | Thêm yêu thích & Đặt câu hỏi Q&A cho Shop | Buyer | `team-engagement`, `/listing/[id]` | Heart icon bật sáng, câu hỏi xuất hiện trong Q&A list |
| **E2E-CART-01** | Thêm vào giỏ & Áp dụng Voucher Giảm giá | Buyer | `team-order`, `/cart`, `/checkout` | Subtotal trừ đúng giá voucher `MARKET50K` |
| **E2E-SAGA-01** | Luồng mua hàng Saga 4 bước (Golden Path) | Buyer | `team-order`, `team-payment`, `/checkout` | Đơn hàng `PAID`, tồn kho bị giữ chính xác |
| **E2E-SAGA-02** | Rollback Saga & Bù trừ tồn kho khi Hủy đơn | Buyer | `team-order`, `team-domain` | Tồn kho được nhả (`ReleaseStock`), đơn `CANCELLED` |
| **E2E-LOG-01** | Seller giao hàng & Buyer xem Timeline SPX | Seller / Buyer | `team-order`, `/account/orders/[id]` | 5 mốc vận chuyển và mã vận đơn SPX hiển thị đầy đủ |
| **E2E-RMA-01** | Yêu cầu trả hàng hoàn tiền (RMA) | Buyer / Seller | `team-order`, `team-payment` | Trạng thái RMA `REFUNDED`, giao dịch thanh toán hoàn tiền |
| **E2E-FIN-01** | Seller xem doanh thu và rút tiền từ ví | Seller | `team-payment`, `/seller/analytics` | Số dư ví trừ đúng, giao dịch rút tiền `PROCESSING` |
| **E2E-AI-01** | Hỏi đáp AI RAG & Giám sát Admin Cockpit | Admin / Buyer | `team-ai`, `team-gateway`, `/admin/cockpit` | AI gợi ý thẻ SP đúng, HUD hiển thị 8 services SERVING |
