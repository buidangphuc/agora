# team-ai — Marketplace AI Engine (FastAPI & RAG)

`team-ai` là dịch vụ trí tuệ nhân tạo (AI Microservice) độc lập chạy trên nền tảng **FastAPI (Python 3.12)** tại cổng `:8000`. Dịch vụ cung cấp các năng lực AI chuyên sâu cho thương mại điện tử: Trợ lý mua sắm RAG thông minh, Công cụ tạo tin đăng chuẩn SEO (Magic Listing) cho người bán và Trợ lý trả lời tin nhắn 1-click (Chat Copilot).

---

## 1. Các Endpoints AI Chính

Dịch vụ cung cấp 3 endpoint AI phục vụ trực tiếp cho người mua (Buyer), người bán (Seller) và hệ thống Chat:

```
                      ┌───────────────────────────────────────┐
                      │        team-ai (FastAPI :8000)        │
                      └──────────────────┬────────────────────┘
                                         │
       ┌─────────────────────────────────┼─────────────────────────────────┐
       ▼                                 ▼                                 ▼
POST /api/v1/ai/assistant        POST /api/v1/ai/magic-listing     POST /api/v1/ai/chat-copilot
(Shopping Assistant RAG)         (SEO & Category Auto-tagging)     (1-Click Seller Smart Replies)
```

---

### 1. `POST /api/v1/ai/assistant` — RAG Shopping Assistant
Nhận câu hỏi bằng ngôn ngữ tự nhiên từ người mua, truy vấn catalog sản phẩm thông qua RAG / semantic keyword matching, và trả về lời tư vấn cá nhân hóa, các thẻ sản phẩm gợi ý và câu hỏi tiếp theo.

- **Request Body (`ShoppingAssistantRequest`)**:
  ```json
  {
    "message": "Tìm cho mình áo thun cotton form rộng giá dưới 200k",
    "user_id": "usr-buyer-001",
    "previous_context": ["Chào shop"],
    "top_k": 4
  }
  ```
- **Response Body (`ShoppingAssistantResponse`)**:
  ```json
  {
    "reply_text": "Dạ chào bạn! Dựa trên tìm kiếm \"Tìm cho mình áo thun cotton...\", AI Assistant gợi ý cho bạn sản phẩm nổi bật Áo Thun Cotton 100% Unisex Form Rộng Oversize Thoáng Mát với giá chỉ 149,000đ...",
    "product_cards": [
      {
        "listing_id": "prod-101",
        "title": "Áo Thun Cotton 100% Unisex Form Rộng Oversize Thoáng Mát",
        "price": 149000,
        "currency": "VND",
        "image_url": "https://images.unsplash.com/photo-1521572267360-ee0c2909d518?w=500",
        "discount_rate": 20,
        "rating": 4.9,
        "rating_text": "4.9/5 (1.2k đánh giá)"
      }
    ],
    "suggested_followups": [
      "Sản phẩm này có những size và màu nào?",
      "Có mã freeship hoặc giảm giá thêm không shop?",
      "Chính sách đổi trả size như thế nào?"
    ]
  }
  ```

---

### 2. `POST /api/v1/ai/magic-listing` — Magic Listing Generator
Giúp người bán tạo nhanh nội dung đăng bán chuẩn SEO chỉ từ một từ khóa gợi ý hoặc tên sơ bộ. AI tự động sinh Tiêu đề chuẩn SEO, Bản mô tả chi tiết Markdown, Phân loại ngành hàng, Ước lượng khoảng giá tối ưu và Gắn thẻ hashtag thịnh hành.

- **Request Body (`MagicListingRequest`)**:
  ```json
  {
    "title_hint": "tai nghe bluetooth chống ồn",
    "category_hint": "",
    "image_url": "https://images.unsplash.com/photo-1590658268037-6bf12165a8df?w=500"
  }
  ```
- **Response Body (`MagicListingResponse`)**:
  ```json
  {
    "generated_title": "Tai Nghe Bluetooth Chống Ồn Chính Hãng - Bảo Hành 12 Tháng - Thiết Kế Hiện Đại",
    "generated_description": "### 🌟 **Tai Nghe Bluetooth Chống Ồn Chính Hãng...**\n\n#### 📌 **ĐẶC ĐIỂM NỔI BẬT**\n- ✅ Chất lượng âm thanh vượt trội...\n- ✅ Chống ồn chủ động ANC...",
    "suggested_category_id": "cat-electronics",
    "suggested_price_min": 250000,
    "suggested_price_max": 490000,
    "highlight_tags": ["tai", "nghe", "bluetooth", "chống", "ồn", "chính hãng", "cao cấp", "freeship"]
  }
  ```

---

### 3. `POST /api/v1/ai/chat-copilot` — Chat Copilot Smart Replies
Phân tích tin nhắn của người mua trong cửa sổ chat để nhận diện ý định (Hỏi tồn kho hàng, Hỏi thời gian giao hàng, Xin giảm giá/voucher, Hỏi chính sách bảo hành/đổi trả, Tư vấn size số) và tạo ngay **3 câu trả lời nhanh phù hợp nhất** để người bán chỉ cần bấm 1-click để gửi.

- **Request Body (`ChatCopilotRequest`)**:
  ```json
  {
    "buyer_message": "Sản phẩm này còn sẵn hàng và có size L không shop?",
    "seller_id": "usr-seller-001",
    "listing_id": "prod-101"
  }
  ```
- **Response Body (`ChatCopilotResponse`)**:
  ```json
  {
    "quick_replies": [
      "Dạ chào bạn, sản phẩm bên shop hiện vẫn còn sẵn hàng ạ! Bạn đặt sớm để shop đóng gói gửi đi ngay trong ngày nhé.",
      "Chào bạn, hàng luôn có sẵn tại kho và được kiểm tra kỹ trước khi gửi. Bạn cần shop tư vấn thêm về size hay màu sắc không ạ?",
      "Dạ shop còn bạn nhé! Mọi đơn đặt trước 16h hôm nay sẽ được giao cho đơn vị vận chuyển ngay ạ."
    ]
  }
  ```

---

## 2. Kiến trúc & Cấu trúc Thư mục

```text
app/
  api/
    v1/
      ai/                      # AI API Surface (assistant, magic-listing, chat-copilot)
        dependencies.py        # Dependency injection cho AIAssistantService
        router.py              # APIRouter definitions
      completions/             # LangChain completions transport (stream/sync)
  bootstrap/                   # FastAPI app factory & lifecycle hooks
  core/                        # Config, database, logging, error handling
  modules/
    ai/
      llm/                     # LangChain Chat Model & Langfuse Tracker
      rag/                     # LlamaIndex knowledge retrieval
    business/
      ai_assistant/            # Domain logic, rich product catalog & heuristic AI engines
        schemas.py             # Pydantic schemas (ProductCard, MagicListing, ChatCopilot)
        service.py             # AIAssistantService implementation
```

---

## 3. Cấu hình Biến môi trường (`.env`)

| Biến môi trường | Mặc định | Ý nghĩa |
|---|---|---|
| `HOST` | `0.0.0.0` | Địa chỉ bind của server FastAPI |
| `PORT` | `8000` | Cổng HTTP |
| `AUTH_BEARER_TOKEN` | `change-me-local-bearer-token` | Static Bearer token cho các endpoint nội bộ |
| `CORS_ALLOW_ORIGINS` | `*` | Danh sách CORS allowed origins |
| `LANGFUSE_ENABLED` | `false` | Bật/tắt observability Langfuse |
| `LANGFUSE_PUBLIC_KEY` | `""` | Public API Key của Langfuse |
| `LANGFUSE_SECRET_KEY` | `""` | Secret API Key của Langfuse |
| `CHAT_MODEL` | `""` | Tên model LangChain (vd: `openai:gpt-4.1-mini`) |

---

## 4. Hướng dẫn chạy và Kiểm thử

```bash
# 1. Cài đặt môi trường với uv
cp .env.example .env
uv sync --dev

# 2. Chạy unit tests
make test

# 3. Khởi chạy server ở chế độ dev (Auto-reload)
make dev
# Server lắng nghe tại http://localhost:8000 (Swagger docs tại http://localhost:8000/docs)
```

### Kiểm tra nhanh bằng cURL

```bash
# Test Shopping Assistant
curl -X POST http://localhost:8000/api/v1/ai/assistant \
  -H "Content-Type: application/json" \
  -d '{"message": "Gợi ý cho mình tai nghe chống ồn tốt"}'

# Test Magic Listing
curl -X POST http://localhost:8000/api/v1/ai/magic-listing \
  -H "Content-Type: application/json" \
  -d '{"title_hint": "áo khoác bomber kaki"}'

# Test Chat Copilot
curl -X POST http://localhost:8000/api/v1/ai/chat-copilot \
  -H "Content-Type: application/json" \
  -d '{"buyer_message": "Có được xem hàng trước khi nhận không shop?"}'
```
