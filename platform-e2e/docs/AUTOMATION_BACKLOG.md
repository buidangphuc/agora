# Automation Backlog — 14 feature còn lại

Snapshot: **20/34 automated (59%)**. Còn **14** (11 planned + 3 manual). File này là
công thức để làm nốt. Số liệu API/selector bên dưới đã probe thật trên stack local
(:3000 / :8080) nên khỏi phải tự mò lại.

## Vòng lặp chuẩn cho mỗi feature
1. `.feature` trong `tests/e2e/features/<area>/` (gắn tag = marker; marker mới phải thêm ở `pyproject.toml`).
2. Steps: **reuse `common_steps` trước** (`the "<page>" page is open`, `I navigate to the "<page>" page`, `I am logged in as a buyer/seller via API`). Steps mới cho vào file riêng `<area>_steps.py` để tránh đụng file người khác.
3. Page object mới → thêm `src/pages/<x>_page.py` + export ở `src/pages/__init__.py` + đăng ký `src/core/page_factory.py` + `PageName` ở `src/constants/pages.py`.
4. Binder `test_<x>.py`: `scenarios("<area>/<file>.feature")` + `from ..._steps import *`.
5. Chạy: `HEADLESS=true pytest tests/e2e/step_definitions/test_<x>.py`. Xanh mới đi tiếp.
6. Flip manifest `<repo>/FEATURES.yaml`: `status: automated` + `covered_by: "<file>.feature::<Scenario>"`.
7. `make features-check` (gate) phải exit 0.

## Dữ liệu tham chiếu đã probe (gateway :8080, JSON `POST /<pkg>.<Service>/<Method>`, header `Authorization: bearer <jwt>`)
- **CreateAddress** (flat, KHÔNG bọc): `{"recipientName","phone","street","ward","district","city","isDefault":true}` → `{"address":{"id":...}}`. Thiếu → 400 "recipient_name, phone, street, and city are required".
- **AddToCart**: `{"listingId","quantity":1}`. **CreateOrder**: `{"paymentMethod":"PAYMENT_METHOD_COD"}` (tự dùng default address) → `{"orders":[{"id","status":"ORDER_STATUS_PENDING",...}]}`. Cart rỗng → 500 "cart is empty".
- **CreateListing** (đã dùng): bọc `{"listing":{...,"status":"LISTING_STATUS_PUBLISHED"}}` → `{"listing":{"id"}}`. Category id thật: `cat-electronics`, `cat-laptop`, `cat-fashion-men`, `cat-beauty`…
- Saga: `OrderService/GetSagaState {"orderId"}`, `OrderService/ForceFailSaga {"orderId"}`. Payment: `PaymentService/ProcessMockPayment`. Shipment: `OrderService/CreateShipment`, `GetShipmentTracking`. RMA: `OrderService/CreateReturnRequest`, `UpdateReturnStatus`, `PaymentService/RefundPayment`. Wallet: `PaymentService/{GetSellerWallet,RequestPayout,ListPayoutHistory}`.
- Selector UI đã biết: register `#username/#password/#role` + "Đăng ký"; edit submit **"Lưu thay đổi"**; favorite heart `role:button` aria-label **"Yêu thích"/"Bỏ thích"**; checkout đặt hàng **"Xác nhận đặt hàng"** / mock pay **"Tiến hành thanh toán Demo"**; chat input placeholder **"Nhập nội dung tin nhắn..."** + nút **"Gửi"**; payout **"Rút Tiền Về Ngân Hàng"**; seller delete control text **"Delete"** ở `team-frontend/src/app/seller/page.tsx:180`.

---

## Group A — Unlock Address + Order (6 feature) ★ROI cao nhất
`order.purchase-saga`, `order.saga-compensation`, `payment.mock-pay`, `order.shipment-tracking`, `order.rma-return`, `payment.refund`. Đồng thời fix luôn scenario đỏ `buyer/order_tracking_and_rma.feature`.

**Hạ tầng làm 1 lần:**
1. Thêm marker `needsAddress`, `needsOrder` vào `pyproject.toml`.
2. `config/tags.py`: không cần payload; xử lý trong hook.
3. `tests/conftest.py` — mở rộng `seed_by_tags`:
   - `needsAddress`: đảm bảo có buyer (needsBuyer) → `service_factory.get(...)` gọi `CreateAddress` (payload flat ở trên) bằng token buyer → lưu `world.state.extra["address_id"]`.
   - `needsOrder`: needsBuyer + needsListing + needsAddress → `AddToCart(seeded listing.listing_id)` → `CreateOrder({paymentMethod:PAYMENT_METHOD_COD})` → lưu `world.state.extra["order_id"]` (PENDING).
4. Thêm service: `src/api/services/` — `IdentityAddressService` (CreateAddress/ListAddresses) và bổ sung `OrderService.create_order(payload)` cho khớp (hiện `tests/e2e/flows/order_flow.py:20` gọi `create_order(items=...)` → **sửa signature** này, đây là nguyên nhân test_order đỏ).

**Rồi từng feature:**
- `payment.mock-pay` (@needsOrder): navigate `/checkout/pay/{order_id}` → nút "Tiến hành thanh toán Demo" → assert order PAID (UI badge hoặc API `GetOrder`).
- `order.purchase-saga` (@needsOrder): sau mock-pay → `GetSagaState(order_id)` = PAID (assert qua API service, hoặc UI `/account/orders` trạng thái "Đã thanh toán").
- `order.saga-compensation` (@needsOrder): `ForceFailSaga(order_id)` → `GetSagaState` = CANCELLED; (tùy chọn) verify tồn kho hoàn qua `GetListing`.
- `order.shipment-tracking` (@needsOrder + đã pay): `CreateShipment(order_id)` → `/account/orders/{order_id}` hiện timeline SPX (assert 1 mốc, vd "Đang vận chuyển").
- `order.rma-return` (order đã delivered): `CreateReturnRequest` → seller `UpdateReturnStatus` (login seller) → assert trạng thái return. Có thể để phần seller-approve ở mức API.
- `payment.refund`: sau khi return approved → `RefundPayment` → assert qua API.

Manifest flip: 6 feature ở `team-order`/`team-payment`.

---

## Group B — Fix routing gateway (2 feature) ★cần sửa backend Go
`engagement.qa`, `engagement.disputes` — **BLOCKED**: gateway trả 404 cho
`AskQuestion`/`AnswerQuestion`/`CreateDispute` (trong khi `ListReviews`→200,
`CreateReview`→400 reachable). RPC đã có trong generated của team-engagement.

**Việc cần làm (team-gateway):**
1. Trong `team-gateway/internal/edge/server.go` (`NewMux`) — nơi đăng ký Connect handler cho EngagementService — bổ sung các method còn thiếu (AskQuestion, AnswerQuestion, ListQuestionsByListing, CreateDispute, GetDispute, ResolveDispute) giống cách CreateReview/ListReviews được mount. Rebuild container gateway.
2. Xác nhận: `curl ... /platform.engagement.v1.EngagementService/AskQuestion` không còn 404.
3. Automate: buyer `AskQuestion` (API hoặc UI trên `/listing/[id]`), seller `AnswerQuestion`, assert `ListQuestionsByListing`. Dispute tương tự.

Manifest: gỡ note BLOCKED, flip automated. Nếu không sửa backend → giữ nguyên (đã document trung thực).

---

## Group C — Standalone, công sức thấp (6 feature)
- `listing.delete` (@needsSeller + seed listing của chính seller đó qua `seed_listing`): `/seller` → control "Delete" (`team-frontend/src/app/seller/page.tsx:180`, có thể là confirm dialog → `BasePage.performActionOnDialog` hoặc `page.on("dialog")`) → assert listing biến mất.
- `notification.unread-badge`: gọi API tạo notification (hoặc dựa notification có sẵn) → `/` → assert badge số ở header (`GetUnreadCount`). Nếu app không tạo được notification dễ → có thể để `manual`.
- `chat.buyer-seller-thread` (@needsListing): mở chat từ listing/shop → input placeholder "Nhập nội dung tin nhắn..." → "Gửi" → assert tin nhắn hiện trong thread (`SendMessage`/`GetThreadMessages`).
- `ai.chat-copilot` (phụ thuộc chat.buyer-seller-thread): trong thread, nút copilot → assert có reply nháp. team-ai :8000 phải chạy.
- `engagement.reviews`: cần đơn đã mua (verified purchase) → **phụ thuộc Group A** (tạo order PAID trước), rồi `CreateReview` → assert review + rating trên `/listing/[id]`. Hoặc seed review thuần API rồi assert UI.
- `payment.payout` (@needsSeller có số dư ví — **phụ thuộc Group A** để có doanh thu): `/seller/analytics` → "Rút Tiền Về Ngân Hàng" → nhập số tiền → assert xuất hiện trong lịch sử (`RequestPayout`/`ListPayoutHistory`).

---

## Ghi chú
- `listing.stock-reservation` giữ `not-testable` (assert gián tiếp qua `order.saga-compensation`).
- Thứ tự khuyến nghị: **Group A trước** (mở khoá reviews + payout của Group C), rồi Group C, cuối cùng Group B (nếu chịu sửa gateway).
- Marker mới cần khai báo ở `pyproject.toml [tool.pytest.ini_options].markers`: `needsAddress`, `needsOrder` (và bất kỳ tag mới nào) — nếu không `--strict-markers` sẽ fail.

## Verify sau mỗi nhóm
```bash
make features          # coverage tăng dần
make features-check    # gate: manifest valid + covered_by resolve
HEADLESS=true pytest -m "<tag>"        # nhóm vừa làm
HEADLESS=true pytest -q --tb=no        # full; 2 đỏ hiện tại sẽ xanh khi Group A/B xong
```
