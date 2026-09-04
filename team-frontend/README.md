# team-frontend — Next.js 14 SSR Marketplace Frontend

`team-frontend` là giao diện người dùng chính của hệ thống Marketplace Polyrepo, được xây dựng trên **Next.js 14 App Router** (React Server Components, SSR) kết hợp với **Tailwind CSS** và **Connect-ES**.

Theo quy tắc kiến trúc **ARCHITECTURE Rule 1**:
- **Frontend chỉ giao tiếp duy nhất với `team-gateway`** (`:8080`) thông qua Connect RPC (HTTP/1.1 JSON/gRPC).
- Không bao giờ gọi trực tiếp database hay microservice gRPC phía sau.
- Không chứa business logic xử lý giao dịch — chỉ đảm nhiệm việc render UI, định dạng dữ liệu hiển thị (UI shaping) và quản lý session cookie `httpOnly`.

---

## 1. Công nghệ & Kiến trúc

- **Framework**: Next.js 14.2+ (App Router, Server Actions, React Server Components).
- **Styling**: Tailwind CSS với theme Shopee signature gradient (`#f53d2d` -> `#f63`).
- **Giao tiếp RPC**: `@connectrpc/connect`, `@connectrpc/connect-node` kết nối tới `team-gateway`.
- **Bảo mật & Session**: Token JWT được lưu trữ an toàn trong Cookie `session` (`httpOnly`, `sameSite: lax`). Server Component tự động trích xuất token để đính kèm vào header `Authorization: Bearer <token>` khi gửi request sang Gateway.

---

## 2. Toàn bộ 25 Routes trong App Router

### Nhóm 1: Trải nghiệm Mua hàng & Khám phá (Buyer Experience)
1. **`/` (Trang chủ Marketplace)**:
   - Header tìm kiếm với thanh gợi ý từ khóa hot (iPhone 15, MacBook Pro, Nike...).
   - Banner chương trình khuyến mãi & Danh mục sản phẩm nổi bật (Thời trang, Điện tử, Nhà cửa, Sắc đẹp...).
   - Flash Sale Section với đồng hồ đếm ngược (Live Countdown Timer) và thanh trạng thái số lượng bán.
   - Lưới gợi ý sản phẩm cá nhân hóa (Personalized Recommendations).
   - Lối tắt truy cập nhanh vào **AI Shopping Assistant**.
2. **`/search` (Tìm kiếm & Bộ lọc nâng cao)**:
   - Tìm kiếm toàn văn (OpenSearch backend) theo từ khóa `?q=`.
   - Bộ lọc đa chiều: Theo danh mục, khoảng giá (Min - Max VND), đánh giá sao (1-5 sao).
   - Sắp xếp linh hoạt: Phổ biến, Mới nhất, Bán chạy, Giá tăng/giảm dần.
   - Gợi ý từ khóa tự động (Autocomplete Suggestion).
3. **`/listing/[id]` (Chi tiết sản phẩm & Tương tác)**:
   - Gallery hình ảnh sản phẩm với thumbnail tương tác.
   - Thông tin giá bán, tỷ lệ giảm giá, trạng thái tồn kho thực tế.
   - Nút **Thêm Vào Giỏ Hàng** và **Mua Ngay**.
   - Khối thông tin gian hàng (Mini Shop Card) kèm nút **Chat Ngay với Người Bán**.
   - Bảng thống kê đánh giá sao (Rating Breakdown 1-5 sao) và danh sách nhận xét thực tế kèm bình luận.
   - Nút Thả tim (Yêu thích / Wishlist) cập nhật số lượt like theo thời gian thực.
   - Khu vực Hỏi Đáp (Q&A) giữa người mua và người bán.
4. **`/cart` (Quản lý Giỏ hàng)**:
   - Danh sách sản phẩm trong giỏ phân nhóm theo Shop.
   - Chọn tất cả / chọn từng món đồ để thanh toán.
   - Tăng/giảm số lượng món hàng, xóa sản phẩm khỏi giỏ.
   - Tự động tính toán tổng tiền tạm tính, số lượng hàng được chọn.
5. **`/checkout` (Thanh toán & Áp dụng Voucher)**:
   - Lựa chọn địa chỉ giao hàng mặc định hoặc thêm địa chỉ mới.
   - Áp dụng Mã giảm giá sàn / Voucher freeship.
   - Lựa chọn phương thức vận chuyển: **SPX Express**, Giao hàng nhanh, Tiết kiệm.
   - Lựa chọn phương thức thanh toán: **VietQR**, MoMo, Thẻ Quốc tế (Visa/Master), COD (Thanh toán khi nhận hàng).
   - Nút **Đặt Hàng** kích hoạt Distributed Saga Purchase flow.
6. **`/checkout/pay/[id]` (Cổng thanh toán Mock)**:
   - Màn hình hiển thị mã QR VietQR động và thông tin chuyển khoản.
   - Nút giả lập thanh toán: "Giả lập Thành Công (Success)" và "Giả lập Thất Bại (Fail)" để kiểm thử luồng Saga.
   - Tự động redirect về trang đơn hàng khi thanh toán hoàn tất.
7. **`/account/orders` (Danh sách Đơn mua của tôi)**:
   - Tab phân loại trạng thái: Tất cả, Chờ thanh toán, Đang xử lý, Đang giao hàng, Đã giao, Đã hủy, Trả hàng/Hoàn tiền.
   - Các nút thao tác nhanh: Xem chi tiết đơn, Thanh toán ngay, Hủy đơn hàng, Mua lại.
8. **`/account/orders/[id]` (Chi tiết Đơn hàng & Tra cứu Vận đơn SPX)**:
   - Visual Stepper thể hiện tiến trình: *Đã đặt hàng -> Đã xác nhận -> Đang vận chuyển -> Đang phát hàng -> Giao thành công*.
   - Mã vận đơn SPX Express và nhật ký hành trình chi tiết (Timestamped timeline).
   - Chi tiết danh sách sản phẩm, phí ship, giảm giá voucher, tổng thanh toán.
   - Nút **Yêu Cầu Trả Hàng / Hoàn Tiền (RMA)** và **Hủy Đơn Hàng**.
9. **`/account/addresses` (Sổ địa chỉ nhận hàng)**:
   - Danh sách địa chỉ đã lưu của người mua.
   - Đặt địa chỉ làm mặc định.
   - Modal thêm mới / sửa địa chỉ (Tỉnh/Thành, Quận/Huyện, Phường/Xã, Tên đường & Số nhà).
10. **`/favorites` (Danh sách Yêu thích / Wishlist)**:
    - Danh sách các sản phẩm đã thả tim.
    - Nút bỏ thích hoặc 1-click chuyển nhanh vào giỏ hàng.
11. **`/notifications` (Trung tâm Thông báo)**:
    - Phân loại tab: Cập nhật đơn hàng, Khuyến mãi hot, Tin tức hệ thống.
    - Đánh dấu đã đọc tất cả, click chuyển hướng trực tiếp đến đơn hàng liên quan.
12. **`/shop/[id]` (Trang Gian hàng Người Bán công khai)**:
    - Header Shop: Avatar, Tên Shop, Tỷ lệ phản hồi chat, Đánh giá trung bình, Số lượng người theo dõi, Số lượng sản phẩm.
    - Kho Voucher độc quyền của Shop.
    - Tab danh mục sản phẩm của shop: Tất cả sản phẩm, Bán chạy nhất, Hàng mới về.

---

### Nhóm 2: Kênh Người Bán & Quản Trị (Seller Portal & Analytics)
13. **`/seller` (Trang tổng quan Kênh Người Bán)**:
    - Thống kê KPI ngày: Đơn hàng mới hôm nay, Doanh thu tạm tính, Đơn chờ giao cho SPX, Sản phẩm sắp hết hàng.
    - Lối tắt: Đăng sản phẩm mới, Xử lý đơn hàng, Xem báo cáo doanh thu.
    - Bảng danh sách đơn hàng cần xử lý gấp.
14. **`/seller/new` (Đăng bán Sản phẩm mới + Magic Listing AI)**:
    - Form nhập thông tin sản phẩm (Tên, Ảnh, Giá, Tồn kho, Danh mục).
    - **Nút "Magic Listing AI"**: Tự động tạo Tiêu đề chuẩn SEO, Bản mô tả chi tiết Markdown, Tự động phân loại danh mục, Gợi ý dải giá thị trường và Tags thịnh hành thông qua `team-ai`.
15. **`/seller/[id]/edit` (Chỉnh sửa thông tin Sản phẩm)**:
    - Cập nhật giá bán, số lượng tồn kho trong kho hàng, tiêu đề và mô tả.
16. **`/seller/orders` (Quản lý Đơn hàng của Shop)**:
    - Lọc đơn theo trạng thái (Chờ xác nhận, Đang xử lý, Đang giao, Đã hoàn thành).
    - Thao tác xác nhận đơn hàng hàng loạt.
17. **`/seller/orders/[id]` (Chi tiết Đơn bán & Bàn giao Vận chuyển)**:
    - Thông tin người mua và địa chỉ nhận hàng.
    - Nút thao tác: "Xác nhận đơn", "Bàn giao cho đơn vị vận chuyển SPX Express".
18. **`/seller/analytics` (Báo cáo Doanh thu & Rút tiền Ví Người Bán)**:
    - Biểu đồ phân tích doanh thu theo ngày/tuần/tháng.
    - Tỷ lệ chuyển đổi đơn hàng, Giá trị trung bình mỗi đơn (AOV).
    - Số dư ví người bán hiện tại (Seller Wallet Balance).
    - Modal yêu cầu rút tiền / thanh toán doanh thu (Payout Request Mock).

---

### Nhóm 3: AI Assistant, Real-time Chat & Telemetry Cockpit
19. **`/assistant` (Trợ Lý Mua Sắm Thông Minh AI Assistant)**:
    - Giao diện đàm thoại AI toàn màn hình.
    - Tìm kiếm catalog sản phẩm thông minh qua RAG (Retrieval-Augmented Generation).
    - Trả lời tư vấn kèm các thẻ sản phẩm (Product Cards) có thể bấm xem chi tiết hoặc mua ngay.
    - Gợi ý sẵn các câu hỏi tiếp theo (Follow-up suggestions chip).
20. **`/chat` (Hộp thư Tin nhắn Trực tiếp)**:
    - Danh sách các cuộc trò chuyện giữa Buyer và Seller.
    - Badge đếm số tin nhắn chưa đọc, thời gian gửi tin cuối.
21. **`/chat/[id]` (Cửa sổ Đàm thoại Real-time)**:
    - Khung chat trực tiếp kết nối qua Server-Sent Events (SSE).
    - Đính kèm thẻ xem trước sản phẩm đang được trao đổi trong cuộc chat.
    - **Chat Copilot (Smart Replies)**: Gợi ý 3 câu trả lời nhanh tự động (1-Click Quick Replies) do `team-ai` phân tích tin nhắn người mua để người bán phản hồi ngay lập tức.
22. **`/admin/cockpit` (Admin Observability Cockpit HUD)**:
    - Bảng điều khiển thời gian thực dành cho Quản trị viên & Kỹ sư hệ thống.
    - Đồng hồ đo Gateway RPS, Độ trễ trung bình (Avg Latency), P95 và P99 Latency.
    - Bảng ma trận sức khỏe (Health Matrix) của toàn bộ 10 microservices trong Polyrepo.
    - Danh sách Distributed Traces mẫu tích hợp link trực tiếp tới Jaeger UI.
    - Live Order Ticker cập nhật các đơn hàng mới phát sinh theo thời gian thực.
23. **`/sell`**: Lối tắt chuyển hướng thông minh đến Kênh người bán `/seller`.
24. **`/login` (Đăng nhập)**:
    - Form đăng nhập chuẩn bằng Email / Mật khẩu.
    - Các nút đăng nhập nhanh 1-click dành cho Demo/Test:
      - 👤 **Buyer Demo**: `buyer@marketplace.local`
      - 🏪 **Seller Demo**: `seller@marketplace.local`
      - ⚡ **Admin Demo**: `admin@marketplace.local`
25. **`/register` (Đăng ký tài khoản mới)**:
    - Đăng ký tài khoản với lựa chọn vai trò (Người mua / Người bán).

---

## 3. Các thành phần Giao diện Toàn cục (Global Components)

1. **Banner Tuyên Bố Học Thuật & Nghiên Cứu (Educational Disclaimer Banner)**:
   - Hiển thị cố định ở đầu tất cả các trang web (`layout.tsx`):
   - *"Tuyên bố đồ án & nghiên cứu kiến trúc: Toàn bộ giao diện và các luồng trải nghiệm này được xây dựng hoàn toàn vì mục đích học tập, nghiên cứu kiến trúc Polyrepo, Microservices gRPC, Event-Driven Kafka, CQRS & Saga Pattern, phi thương mại và hoàn toàn không có ý định sao chép/clone thương hiệu."*
2. **Floating Chat Bubble (`FloatingChatBubble.tsx`)**:
   - Nút bong bóng chat nổi cố định góc dưới bên phải màn hình giúp người mua mở nhanh cuộc trò chuyện với người bán hoặc truy cập AI Assistant từ bất kỳ trang nào.
3. **Toast Notification System (`ToastProvider.tsx`)**:
   - Hệ thống thông báo toast nổi tự động biến mất khi người dùng thêm hàng vào giỏ, áp mã voucher, copy mã đơn hàng hoặc cập nhật trạng thái thành công.

---

## 4. Biến môi trường Cấu hình (`.env`)

```bash
# URL của Edge Gateway (bắt buộc)
NEXT_PUBLIC_GATEWAY_URL=http://localhost:8080
GATEWAY_URL=http://localhost:8080

# URL của FastAPI AI Service (tùy chọn / fallback)
AI_SERVICE_URL=http://localhost:8000

# Secret giải mã JWT (để decode payload session phía client/SSR)
JWT_SECRET=secret
```

---

## 5. Hướng dẫn chạy và Kiểm thử

```bash
# 1. Cài đặt dependencies
npm install

# 2. Sinh mã nguồn TypeScript từ Proto contracts
npm run proto

# 3. Chạy môi trường phát triển (Dev Server)
npm run dev

# Ứng dụng sẽ hoạt động tại http://localhost:3000
```
