# 00-spec — "Killer Showcase" Fullstack Marketplace

> Copied into EVERY subagent's context. Self-contained + short. Follow it; do not re-decide.

## Goal
Biến AI-first marketplace (polyrepo microservices) thành bản phô diễn tư duy Fullstack/Staff:
một **golden demo path** xuyên toàn stack (Reactive FE ↔ backend phân tán ↔ SRE ↔ AI),
xây theo **MVP-mỗi-trụ-cột**, chạy song song tối đa (mỗi service = 1 write-set tách biệt).

**Golden demo path:** Buyer hỏi AI Assistant → search → xem Flash Sale (tồn kho nhảy real-time
khi người khác mua) → add-to-cart → checkout **saga** (timeline 4 bước, nút test-fail →
compensation trả kho) → mock pay → chat với seller (seller có AI Copilot) → notification bật
chuông → Admin mở `/admin/cockpit` (đơn chảy real-time + p95/p99 + Inspect Trace → Jaeger).

## User stories (chỉ cái đổi quyết định)
- **Buyer** thêm địa chỉ, bỏ hàng vào giỏ, checkout tạo đơn với stock được reserve, theo dõi đơn.
- **Buyer** xem 1 sản phẩm flash-sale và thấy `🔥 ĐÃ BÁN xx%` **tự nhảy** khi buyer khác mua (không F5).
- **Buyer** hỏi AI Assistant bằng ngôn ngữ tự nhiên → nhận **product card** bấm được trong khung chat.
- **Buyer↔Seller** chat realtime; **Seller** nhận gợi ý trả lời 1-click từ AI Copilot.
- **Seller** đăng tin: bấm "✨ AI tạo mô tả & gợi ý giá" → form tự điền.
- **Seller** thấy đơn của shop, advance trạng thái.
- **Admin/SRE** mở cockpit: order ticker realtime, RPS + p95/p99, deep-link trace sang Jaeger.
- **Edge case demo:** checkout ép payment fail → saga **compensation** trả stock nguyên vẹn + order CANCELLED.

## Non-goals
Tiền thật/ví/boost (payment = **mock only**); variant/category (Phase 3); admin CRUD/voucher
(Phase 7); đổi cơ chế auth (giữ JWT verify-once ở gateway, ADR-0003); **Visual Search** (hoãn);
tinh chỉnh ranking AI. Không refactor ngoài scope; không thêm dependency ngoài spec này.

## Architecture decisions (làm NOW — sub-agent theo, không tự quyết lại)
- **Polyrepo, mỗi service owns its DB**; cross-service = gRPC, không join DB (AGENTS §3, Rule 3).
- **Contract là source of truth**: RPC/message chỉ định ở `platform-core/packages/proto`, additive
  (buf lint + buf breaking pass), re-vendor + `buf generate` trong repo sở hữu. Không sửa code sinh.
- **Broker đúng luồng** (ADR-0002): state-change → Kafka `<domain>.events` (key = aggregate id,
  bọc `platform.events.v1.EventEnvelope`); background job → RabbitMQ.
- **1 real-time edge dùng chung**: cầu **Kafka → SSE/WS** ở `team-gateway`, multiplex theo room
  (`listing:{id}`, `chat:{thread}`, `user:{id}`, `ops:orders`). Transport-only — KHÔNG business
  logic ở gateway (Rule 2). Flash-sale, chat, notification bell, cockpit ticker đều dùng edge này.
- **team-ai = 1 sidecar nhiều endpoint** (assistant / magic-listing / chat-copilot / semantic-text).
  **Model/LLM provider do team-ai tự lo** — spec chỉ khoá schema I/O; stub JSON hợp lệ là đạt MVP.
- **Observability**: OTel spans mỗi service (fold vào từng task); bật otel compose profile; exporter
  → Jaeger + Prometheus; cockpit đọc Prometheus **qua gateway proxy** (browser không gọi thẳng Prom).
- **Flash-sale = atomic stock**: reserve/trừ kho phải row-lock/atomic, bám saga — không oversell.
- **Payment mock**: chọn method → callback "paid" → advance order; tuyệt đối không chuyển tiền thật.

## Contracts (bề mặt additive — field number chốt khi mở proto ở Wave 0/R0)
**order proto (team-order sở hữu)**
- `Cart`, `CartItem{listing_id, qty, unit_price_snapshot}`, `Order`, `OrderItem`.
- `enum OrderStatus { PENDING_PAYMENT, PAID, SHIPPED, COMPLETED, CANCELLED }`.
- RPC: AddToCart / UpdateCartItem / RemoveCartItem / GetCart / Checkout / GetOrder /
  ListBuyerOrders / ListShopOrders / AdvanceOrderStatus / **GetSagaState** / **ForceFail**(test-only).
- Kafka `order.events`: `OrderCreated`, `OrderStatusChanged`.

**identity proto (team-identity)** — `Address{id,buyer_id,name,phone,line,ward,district,city,is_default}`;
RPC AddAddress / UpdateAddress / DeleteAddress / ListAddresses / SetDefaultAddress.

**domain proto (team-domain)** — stock reserve mirror promotion:
- RPC `ReserveStock(items)→{reservationId}`, `CommitStock(reservationId)` (idempotent),
  `ReleaseStock(reservationId)`; product-level stock + cờ `is_flash_sale`, `sold`, `total`.
- Kafka `ListingStockChanged{listing_id, sold, total}` mỗi lần stock đổi.

**ai proto/HTTP (team-ai)** — I/O schema (provider-agnostic):
- `Assistant(query)→{reply, product_cards[]}` (streaming); `MagicListing(title, image_key)→
  {description, suggested_price, category}`; `ChatCopilot(thread_ctx)→{suggestions[]}`;
  `SemanticSearch(query)→{listing_ids[]}`.

**chat proto (team-chat)** — `Message{id,thread_id,sender,body,ts}`; RPC SendMessage / GetThread /
ListThreads; Kafka `chat.events{MessageSent}`.

**notification proto (team-notification)** — `Notification{id,user_id,kind,payload,read,ts}`;
RPC ListNotifications / MarkRead; consumes `order.events` + `chat.events` (idempotent by event id).

## Conventions
- Go services mirror FastAPI reference template (config env tags + .env.example drift gate;
  pgxpool health; gRPC transport + interceptors; in-process gRPC tests on ephemeral ports).
  `gofmt` + `go vet` + `go test` sạch. Go/buf chạy **trong Docker** (host không có Go).
- Frontend: Next.js 14 App Router + Tailwind, Connect-ES v1, server-only gateway module,
  per-request client từ httpOnly `session` cookie. `next build` + `tsc` sạch.
- Test location: cạnh code trong repo sở hữu. Commit per task; không push trừ khi được yêu cầu.
- Commit author `Bùi Đăng phúc <phuc.buidang@batdongsan.com.vn>`.
- Ports: gateway :8080, frontend :3000, domain :50051, search :50052, identity :50053,
  engagement :50054, **order :50055**. Infra: Jaeger :16686, Prometheus :9090, Grafana :3001,
  MinIO :9000/9001, Qdrant :6333, kafka-ui :8088.

## Work-rules cho mọi sub-agent
Chỉ create/edit file trong **Write-set** của packet. Muốn đụng file khác → **STOP & báo lý do**,
không sửa. Explore bằng codegraph → grep hẹp → Read có mục tiêu (≤5 file). Theo pattern sẵn có;
**không thêm dependency** (deps quyết ở spec này); không refactor ngoài scope; chỉ viết test mà
acceptance yêu cầu. Báo cáo ngắn gọn: đã đổi gì.

## Giả định đang khoá (verify ở bước đầu mỗi task — KHÔNG đọc code khi lập plan)
`team-order/payment/chat` build dở → mỗi task xác minh cái đã có, chỉ làm phần thiếu.
`team-notification` chưa có repo → tạo mới (copy shape 1 Go service). Stock product-level.
Cách deploy/CI từng service (ngoài compose local) — chốt trước prod, ngoài scope demo.

## Locked
2026-09-01 — Chốt bởi user: (1) mục tiêu showcase fullstack 6 trụ cột theo golden demo path;
(2) Phase A saga là nền, làm trước; (3) LLM/kết quả AI do team-ai tự lo (spec chỉ khoá schema);
(4) Visual Search hoãn; (5) chiến lược MVP-mỗi-trụ-cột. Decomposition & fan-out được phép tiến hành.
