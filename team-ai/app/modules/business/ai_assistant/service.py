from __future__ import annotations

import re
import unicodedata
from typing import Any

from loguru import logger

from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    ChatCopilotResponse,
    MagicListingRequest,
    MagicListingResponse,
    ProductCard,
    ShoppingAssistantRequest,
    ShoppingAssistantResponse,
    SummarizeReviewsRequest,
    SummarizeReviewsResponse,
)

# Aspect keyword map for deterministic pros/cons extraction from review text.
# Each entry maps a Vietnamese aspect label to accent-stripped trigger keywords.
REVIEW_ASPECTS: list[tuple[str, tuple[str, ...]]] = [
    (
        "Chất lượng sản phẩm",
        (
            "chat luong",
            "chat luong tot",
            "ben",
            "chac chan",
            "xin",
            "tot",
            "te",
            "kem",
            "mau hong",
            "hong",
            "loi",
        ),
    ),
    (
        "Giao hàng & đóng gói",
        (
            "giao hang",
            "giao",
            "ship",
            "van chuyen",
            "dong goi",
            "goi",
            "nhanh",
            "cham",
            "lau",
        ),
    ),
    ("Giá cả", ("gia", "gia ca", "re", "dat", "hop ly", "dang tien", "gia tot")),
    (
        "Dịch vụ & hỗ trợ",
        ("dich vu", "shop", "tu van", "nhiet tinh", "phan hoi", "cham soc", "thai do"),
    ),
    (
        "Mẫu mã & thiết kế",
        ("mau ma", "dep", "mau sac", "thiet ke", "form", "kieu dang", "xau"),
    ),
    (
        "Đúng mô tả",
        (
            "dung mo ta",
            "giong hinh",
            "nhu hinh",
            "chuan",
            "sai mo ta",
            "khac hinh",
            "khong giong",
        ),
    ),
]

# Built-in rich catalog for semantic / keyword matching & RAG fallback
CATALOG: list[dict[str, Any]] = [
    {
        "listing_id": "prod-101",
        "title": "Áo Thun Cotton 100% Unisex Form Rộng Oversize Thoáng Mát",
        "category_id": "cat-fashion",
        "price": 149000,
        "image_url": "https://images.unsplash.com/photo-1521572267360-ee0c2909d518?w=500",
        "discount_rate": 20,
        "rating": 4.9,
        "rating_text": "4.9/5 (1.2k đánh giá)",
        "keywords": [
            "áo thun",
            "ao thun",
            "t-shirt",
            "cotton",
            "unisex",
            "oversize",
            "form rộng",
            "thời trang",
            "nam",
            "nữ",
        ],
    },
    {
        "listing_id": "prod-102",
        "title": "Áo Khoác Bomber Kaki 2 Lớp Chống Gió Chống Nước Phong Cách Hàn Quốc",
        "category_id": "cat-fashion",
        "price": 289000,
        "image_url": "https://images.unsplash.com/photo-1591047139829-d91aecb6caea?w=500",
        "discount_rate": 15,
        "rating": 4.8,
        "rating_text": "4.8/5 (850 đánh giá)",
        "keywords": [
            "áo khoác",
            "ao khoac",
            "jacket",
            "bomber",
            "kaki",
            "hàn quốc",
            "nam",
            "nữ",
            "áo gió",
        ],
    },
    {
        "listing_id": "prod-103",
        "title": "Quần Jean Nam Ống Suông Baggy Denim Co Giãn Trẻ Trung",
        "category_id": "cat-fashion",
        "price": 239000,
        "image_url": "https://images.unsplash.com/photo-1542272604-780c96856592?w=500",
        "discount_rate": 10,
        "rating": 4.7,
        "rating_text": "4.7/5 (620 đánh giá)",
        "keywords": [
            "quần jean",
            "quan jean",
            "denim",
            "baggy",
            "ống suông",
            "quần bò",
            "nam",
        ],
    },
    {
        "listing_id": "prod-104",
        "title": "Giày Sneaker Thể Thao Nam Nữ Đế Êm Chạy Bộ Đi Học Thời Trang",
        "category_id": "cat-fashion",
        "price": 350000,
        "image_url": "https://images.unsplash.com/photo-1549298916-b41d501d3772?w=500",
        "discount_rate": 25,
        "rating": 4.9,
        "rating_text": "4.9/5 (2.5k đánh giá)",
        "keywords": [
            "giày",
            "giay",
            "sneaker",
            "thể thao",
            "chạy bộ",
            "đi học",
            "giày nam",
            "giày nữ",
        ],
    },
    {
        "listing_id": "prod-201",
        "title": "Tai Nghe Bluetooth True Wireless TWS Chống Ồn ENC Pin Trâu 30H",
        "category_id": "cat-electronics",
        "price": 320000,
        "image_url": "https://images.unsplash.com/photo-1590658268037-6bf12165a8df?w=500",
        "discount_rate": 30,
        "rating": 4.8,
        "rating_text": "4.8/5 (3.1k đánh giá)",
        "keywords": [
            "tai nghe",
            "tai nghe bluetooth",
            "tws",
            "chống ồn",
            "headphone",
            "earphone",
            "không dây",
            "âm thanh",
        ],
    },
    {
        "listing_id": "prod-202",
        "title": "Bàn Phím Cơ Không Dây Bluetooth 3 Chế Độ RGB Hot-swap Gasket Mount",
        "category_id": "cat-electronics",
        "price": 650000,
        "image_url": "https://images.unsplash.com/photo-1587829741301-dc798b83add3?w=500",
        "discount_rate": 18,
        "rating": 4.9,
        "rating_text": "4.9/5 (920 đánh giá)",
        "keywords": [
            "bàn phím cơ",
            "ban phim co",
            "keyboard",
            "rgb",
            "hot swap",
            "không dây",
            "gaming",
            "máy tính",
        ],
    },
    {
        "listing_id": "prod-203",
        "title": "Chuột Gaming Không Dây Siêu Nhẹ 59g Cảm Biến Quang Học 26K DPI",
        "category_id": "cat-electronics",
        "price": 420000,
        "image_url": "https://images.unsplash.com/photo-1615663245857-ac93bb7c39e7?w=500",
        "discount_rate": 12,
        "rating": 4.8,
        "rating_text": "4.8/5 (780 đánh giá)",
        "keywords": [
            "chuột",
            "chuot",
            "mouse",
            "gaming",
            "không dây",
            "siêu nhẹ",
            "dpi",
        ],
    },
    {
        "listing_id": "prod-204",
        "title": "Củ Sạc Nhanh GaN 65W 3 Cổng Type-C & USB-A Đa Năng Cho Laptop Điện Thoại",
        "category_id": "cat-electronics",
        "price": 275000,
        "image_url": "https://images.unsplash.com/photo-1583863788434-e58a36330cf0?w=500",
        "discount_rate": 22,
        "rating": 4.9,
        "rating_text": "4.9/5 (1.8k đánh giá)",
        "keywords": [
            "củ sạc",
            "cu sac",
            "sạc nhanh",
            "gan",
            "65w",
            "type c",
            "iphone",
            "samsung",
            "laptop",
        ],
    },
    {
        "listing_id": "prod-301",
        "title": "Bình Giữ Nhiệt Inox 316 Cao Cấp Giữ Nóng Lạnh 24H Dung Tích 800ml",
        "category_id": "cat-home-living",
        "price": 185000,
        "image_url": "https://images.unsplash.com/photo-1602143407151-7111542de6e8?w=500",
        "discount_rate": 20,
        "rating": 4.9,
        "rating_text": "4.9/5 (4.2k đánh giá)",
        "keywords": [
            "bình giữ nhiệt",
            "binh giu nhiet",
            "inox 316",
            "giữ nóng",
            "giữ lạnh",
            "nước",
            "gia dụng",
        ],
    },
    {
        "listing_id": "prod-302",
        "title": "Đèn Ngủ LED Cảm Ứng Để Bàn Điều Chỉnh Độ Sáng 3 Màu Tích Điện",
        "category_id": "cat-home-living",
        "price": 129000,
        "image_url": "https://images.unsplash.com/photo-1507473885765-e6ed057f782c?w=500",
        "discount_rate": 15,
        "rating": 4.7,
        "rating_text": "4.7/5 (1.1k đánh giá)",
        "keywords": [
            "đèn ngủ",
            "den ngu",
            "đèn led",
            "cảm ứng",
            "để bàn",
            "trang trí",
            "phòng ngủ",
        ],
    },
    {
        "listing_id": "prod-401",
        "title": "Kem Chống Nắng Nâng Tông Kiềm Dầu SPF 50+ PA++++ Cho Da Dầu Mụn 50ml",
        "category_id": "cat-beauty",
        "price": 195000,
        "image_url": "https://images.unsplash.com/photo-1556228720-195a672e8a03?w=500",
        "discount_rate": 10,
        "rating": 4.9,
        "rating_text": "4.9/5 (5.6k đánh giá)",
        "keywords": [
            "kem chống nắng",
            "kem chong nang",
            "spf 50",
            "kiềm dầu",
            "da dầu",
            "mỹ phẩm",
            "skincare",
        ],
    },
    {
        "listing_id": "prod-402",
        "title": "Serum Dưỡng Trắng Mờ Thâm Vitamin C & Niacinamide 30ml",
        "category_id": "cat-beauty",
        "price": 245000,
        "image_url": "https://images.unsplash.com/photo-1620916566398-39f1143ab7be?w=500",
        "discount_rate": 20,
        "rating": 4.8,
        "rating_text": "4.8/5 (2.9k đánh giá)",
        "keywords": [
            "serum",
            "vitamin c",
            "niacinamide",
            "dưỡng trắng",
            "mờ thâm",
            "skincare",
            "mỹ phẩm",
        ],
    },
]


def _strip_accents(text: str) -> str:
    nfkd = unicodedata.normalize("NFKD", text)
    return "".join(c for c in nfkd if not unicodedata.combining(c)).lower()


class AIAssistantService:
    def __init__(self, rag_service: Any = None) -> None:
        self.rag_service = rag_service

    async def shopping_assistant(
        self, request: ShoppingAssistantRequest
    ) -> ShoppingAssistantResponse:
        query = request.message.strip()
        logger.info(
            "shopping_assistant.query user_id={} message={}", request.user_id, query
        )

        # 1. Match products from catalog / RAG
        matched_products = self._match_products(query, top_k=request.top_k)

        # 2. Craft personalized reply text
        reply_text = self._build_assistant_reply(query, matched_products)

        # 3. Build suggested followups
        suggested_followups = self._build_suggested_followups(query, matched_products)

        return ShoppingAssistantResponse(
            reply_text=reply_text,
            product_cards=matched_products,
            suggested_followups=suggested_followups,
        )

    async def magic_listing(self, request: MagicListingRequest) -> MagicListingResponse:
        title_hint = request.title_hint.strip()
        logger.info("magic_listing.generate hint={}", title_hint)

        category_id, cat_name = self._infer_category(title_hint, request.category_hint)
        generated_title = self._generate_seo_title(title_hint, cat_name)
        generated_desc = self._generate_seo_description(
            title_hint, cat_name, generated_title
        )
        price_min, price_max = self._estimate_price_range(title_hint, category_id)
        highlight_tags = self._generate_tags(title_hint, cat_name)

        return MagicListingResponse(
            generated_title=generated_title,
            generated_description=generated_desc,
            suggested_category_id=category_id,
            suggested_price_min=price_min,
            suggested_price_max=price_max,
            highlight_tags=highlight_tags,
        )

    async def chat_copilot(self, request: ChatCopilotRequest) -> ChatCopilotResponse:
        buyer_msg = request.buyer_message.strip()
        logger.info("chat_copilot.replies buyer_message={}", buyer_msg)

        quick_replies = self._generate_smart_replies(buyer_msg)
        return ChatCopilotResponse(quick_replies=quick_replies)

    async def summarize_reviews(
        self, request: SummarizeReviewsRequest
    ) -> SummarizeReviewsResponse:
        reviews = request.reviews
        logger.info(
            "summarize_reviews.generate listing_id={} count={}",
            request.listing_id,
            len(reviews),
        )

        if not reviews:
            return SummarizeReviewsResponse(
                summary="Chưa có đánh giá nào cho sản phẩm này.",
                pros=[],
                cons=[],
                sentiment="unknown",
            )

        rated = [r.rating for r in reviews if 1 <= r.rating <= 5]
        avg = sum(rated) / len(rated) if rated else 0.0
        has_pos = any(r.rating >= 4 for r in reviews)
        has_neg = any(1 <= r.rating <= 2 for r in reviews)

        pros = self._extract_review_aspects(reviews, positive=True)
        cons = self._extract_review_aspects(reviews, positive=False)
        if has_pos and not pros:
            pros = ["Được khách hàng đánh giá tích cực"]
        if has_neg and not cons:
            cons = ["Một số khách hàng chưa hài lòng"]

        sentiment = self._review_sentiment(avg, has_pos=has_pos, has_neg=has_neg)
        summary = self._build_review_summary(len(reviews), avg, sentiment, pros, cons)

        return SummarizeReviewsResponse(
            summary=summary,
            pros=pros,
            cons=cons,
            sentiment=sentiment,
        )

    def _extract_review_aspects(
        self, reviews: list[Any], *, positive: bool
    ) -> list[str]:
        # Positive aspects come from >=4 star reviews, negatives from <=2 star.
        matched: set[str] = set()
        for review in reviews:
            if positive and review.rating < 4:
                continue
            if not positive and not (1 <= review.rating <= 2):
                continue
            norm = _strip_accents(review.comment)
            for label, keywords in REVIEW_ASPECTS:
                if any(kw in norm for kw in keywords):
                    matched.add(label)
        # Preserve the stable REVIEW_ASPECTS order for deterministic output.
        return [label for label, _ in REVIEW_ASPECTS if label in matched]

    def _review_sentiment(self, avg: float, *, has_pos: bool, has_neg: bool) -> str:
        if has_pos and has_neg:
            return "mixed"
        if avg >= 3.5:
            return "positive"
        if avg <= 2.5 and avg > 0:
            return "negative"
        return "neutral"

    def _build_review_summary(
        self,
        count: int,
        avg: float,
        sentiment: str,
        pros: list[str],
        cons: list[str],
    ) -> str:
        verdict = {
            "positive": "được đánh giá rất tích cực",
            "negative": "nhận nhiều phản hồi chưa hài lòng",
            "mixed": "nhận ý kiến trái chiều",
            "neutral": "được đánh giá ở mức trung bình",
        }.get(sentiment, "được đánh giá ở mức trung bình")

        parts = [
            f"Dựa trên {count} đánh giá (điểm trung bình {avg:.1f}/5), "
            f"sản phẩm {verdict}."
        ]
        if pros:
            parts.append(f"Điểm được khen nhiều nhất: {pros[0]}.")
        if cons:
            parts.append(f"Điểm cần cải thiện: {cons[0]}.")
        return " ".join(parts)

    def _match_products(self, query: str, top_k: int = 4) -> list[ProductCard]:
        norm_query = _strip_accents(query)
        words = set(re.findall(r"\w+", norm_query))

        # Check for price constraint (e.g. "dưới 300k", "< 500000")
        max_price = self._extract_max_price(norm_query)

        scored: list[tuple[float, dict[str, Any]]] = []
        for item in CATALOG:
            score = 0.0
            item_text = _strip_accents(
                item["title"] + " " + " ".join(item.get("keywords", []))
            )

            for word in words:
                if len(word) >= 2 and word in item_text:
                    score += 2.0
                if word in _strip_accents(item["title"]):
                    score += 3.0

            # Direct substring match boost
            if norm_query in item_text:
                score += 10.0

            # Price constraint filter / penalty
            if max_price is not None:
                if item["price"] <= max_price:
                    score += 4.0
                else:
                    score -= 10.0

            if score > 0:
                scored.append((score, item))

        scored.sort(key=lambda x: x[0], reverse=True)

        if not scored:
            # Fallback to popular recommendations if no direct match
            candidates = CATALOG[:top_k]
        else:
            candidates = [x[1] for x in scored[:top_k]]

        return [
            ProductCard(
                listing_id=c["listing_id"],
                title=c["title"],
                price=c["price"],
                currency=c.get("currency", "VND"),
                image_url=c.get("image_url", ""),
                discount_rate=c.get("discount_rate", 0),
                rating=c.get("rating", 4.8),
                rating_text=c.get("rating_text", "4.8/5"),
            )
            for c in candidates
        ]

    def _extract_max_price(self, norm_query: str) -> int | None:
        # Matches patterns like "duoi 300k", "< 500k", "tam 200k"
        match_k = re.search(
            r"(?:duoi|tam|gia re|khoang|duoi muc|<|<=)\s*(\d+)\s*(?:k|nghin|ngan)",
            norm_query,
        )
        if match_k:
            return int(match_k.group(1)) * 1000
        match_vnd = re.search(r"(?:duoi|tam|<)\s*(\d{4,9})", norm_query)
        if match_vnd:
            return int(match_vnd.group(1))
        return None

    def _build_assistant_reply(self, query: str, products: list[ProductCard]) -> str:
        if not products:
            return (
                f'Dạ chào bạn! Hiện tại mình chưa tìm thấy sản phẩm chính xác cho yêu cầu "{query}". '
                "Bạn có thể thử tìm với các từ khóa phổ biến như 'áo thun', 'tai nghe', 'giày sneaker', hoặc 'bình giữ nhiệt' nha!"
            )

        top_item = products[0]
        count = len(products)
        return (
            f'Dạ chào bạn! Dựa trên tìm kiếm "{query}", AI Assistant gợi ý cho bạn {count} sản phẩm tốt nhất. '
            f"Nổi bật nhất là **{top_item.title}** với giá chỉ **{top_item.price:,.0f}đ** "
            f"({top_item.discount_rate}% giảm giá, đánh giá {top_item.rating_text}). "
            "Bạn tham khảo các thẻ sản phẩm bên dưới để xem chi tiết và đặt hàng nhé!"
        )

    def _build_suggested_followups(
        self, query: str, products: list[ProductCard]
    ) -> list[str]:
        norm = _strip_accents(query)
        if any(k in norm for k in ["ao", "quan", "vay", "giay", "thoi trang"]):
            return [
                "Sản phẩm này có những size và màu nào?",
                "Có mã freeship hoặc giảm giá thêm không shop?",
                "Chính sách đổi trả size như thế nào?",
            ]
        if any(
            k in norm
            for k in ["tai nghe", "ban phim", "chuot", "sac", "dien thoai", "cong nghe"]
        ):
            return [
                "Sản phẩm có được bảo hành chính hãng bao lâu?",
                "Có hỗ trợ kết nối cả iOS và Android không?",
                "Giao hàng nhanh trong ngày được không shop?",
            ]
        if any(k in norm for k in ["my pham", "kem", "serum", "da"]):
            return [
                "Sản phẩm này phù hợp cho loại da nào?",
                "Cách sử dụng mang lại hiệu quả tốt nhất?",
                "Cam kết hàng chính hãng 100% không shop?",
            ]
        return [
            "Shop có sẵn hàng để giao ngay không?",
            "Có được kiểm tra hàng trước khi thanh toán không?",
            "Tư vấn thêm cho mình các sản phẩm tương tự nhé!",
        ]

    def _infer_category(
        self, title_hint: str, category_hint: str = ""
    ) -> tuple[str, str]:
        if category_hint:
            return category_hint, category_hint

        norm = _strip_accents(title_hint)
        if any(
            k in norm
            for k in [
                "ao",
                "quan",
                "vay",
                "dam",
                "giay",
                "dep",
                "tui",
                "balo",
                "mu",
                "non",
            ]
        ):
            return "cat-fashion", "Thời Trang & Phụ Kiện"
        if any(
            k in norm
            for k in [
                "tai nghe",
                "chuot",
                "ban phim",
                "sac",
                "cap",
                "pin",
                "loa",
                "may tinh",
                "dien thoai",
            ]
        ):
            return "cat-electronics", "Thiết Bị Điện Tử & Phụ Kiện"
        if any(
            k in norm
            for k in [
                "binh",
                "noi",
                "chao",
                "den",
                "hop",
                "chan",
                "ga",
                "goi",
                "ke",
                "ghe",
                "ban",
            ]
        ):
            return "cat-home-living", "Nhà Cửa & Đời Sống"
        if any(
            k in norm
            for k in ["kem", "serum", "son", "phan", "sua rua mat", "nuoc hoa", "tam"]
        ):
            return "cat-beauty", "Sức Khỏe & Sắc Đẹp"
        if any(k in norm for k in ["vot", "bong", "tham", "xe dap", "ta", "chay bo"]):
            return "cat-sports", "Thể Thao & Dã Ngoại"
        return "cat-general", "Sản Phẩm Tổng Hợp"

    def _generate_seo_title(self, title_hint: str, cat_name: str) -> str:
        clean = title_hint.strip()
        # Capitalize nicely
        clean = " ".join(w.capitalize() for w in clean.split())
        if "Thời Trang" in cat_name:
            return f"[{clean}] Form Rộng Unisex Cao Cấp Thoáng Mát Chuẩn Xu Hướng"
        if "Điện Tử" in cat_name:
            return f"{clean} Chính Hãng - Bảo Hành 12 Tháng - Thiết Kế Hiện Đại"
        if "Nhà Cửa" in cat_name:
            return f"{clean} Đa Năng Tiện Lợi Chất Liệu Cao Cấp Độ Bền Cao"
        if "Sắc Đẹp" in cat_name:
            return f"{clean} Dưỡng Ẩm Chuyên Sâu Chính Hãng An Toàn Mọi Loại Da"
        return f"{clean} Cao Cấp Chính Hãng - Tiện Lợi Đa Năng Giá Tốt"

    def _generate_seo_description(
        self, title_hint: str, cat_name: str, generated_title: str
    ) -> str:
        return f"""### 🌟 **{generated_title}**

Chào mừng bạn đến với gian hàng chính hãng! Sản phẩm **{title_hint}** được thiết kế hiện đại, chất lượng vượt trội nhằm mang lại trải nghiệm tốt nhất cho người dùng.

---

#### 📌 **ĐẶC ĐIỂM NỔI BẬT**
- ✅ **Chất lượng cao cấp:** Gia công tỉ mỉ, kiểm định chất lượng nghiêm ngặt trước khi xuất xưởng.
- ✅ **Thiết kế thông minh:** Tối ưu hóa tính thẩm mỹ và công năng sử dụng hàng ngày.
- ✅ **Độ bền vượt trội:** Sử dụng vật liệu an toàn, thân thiện và bền đẹp theo thời gian.
- ✅ **Dễ dàng sử dụng & vệ sinh:** Phù hợp với mọi đối tượng và nhu cầu sử dụng.

---

#### 📐 **THÔNG SỐ KỸ THUẬT & PHÂN LOẠI**
- **Ngành hàng:** {cat_name}
- **Tình trạng:** Mới 100% nguyên seal / full box
- **Xuất xứ:** Chính hãng / Nhập khẩu tiêu chuẩn
- **Màu sắc & Kích thước:** Đa dạng tuỳ chọn theo phân loại sản phẩm.

---

#### 💡 **HƯỚNG DẪN SỬ DỤNG & BẢO QUẢN**
1. Đọc kỹ hướng dẫn sử dụng đi kèm sản phẩm trước khi dùng.
2. Bảo quản nơi khô ráo, thoáng mát, tránh ánh nắng trực tiếp hoặc nhiệt độ quá cao.
3. Vệ sinh định kỳ bằng khăn mềm ẩm để giữ sản phẩm luôn như mới.

---

#### 🛡️ **CHÍNH SÁCH BÁN HÀNG & BẢO HÀNH**
- 🔄 **Đổi trả 1-1 trong 7 ngày** nếu có lỗi do nhà sản xuất (hàng nguyên tem mác).
- 🚚 **Giao hàng toàn quốc:** Đóng gói chống sốc kỹ càng, hỗ trợ đồng kiểm khi nhận hàng.
- 💬 **Hỗ trợ 24/7:** Đội ngũ CSKH sẵn sàng tư vấn và giải đáp mọi thắc mắc của bạn qua chat.
"""

    def _estimate_price_range(
        self, title_hint: str, category_id: str
    ) -> tuple[int, int]:
        norm = _strip_accents(title_hint)
        if "tai nghe" in norm or "chuot" in norm:
            return 250000, 490000
        if "ban phim" in norm or "loa" in norm:
            return 450000, 950000
        if "ao khoac" in norm or "giay" in norm:
            return 290000, 550000
        if "ao" in norm or "quan" in norm or "vay" in norm:
            return 120000, 250000
        if "kem" in norm or "serum" in norm or "son" in norm:
            return 180000, 350000
        if "binh" in norm or "den" in norm:
            return 99000, 220000
        return 150000, 350000

    def _generate_tags(self, title_hint: str, cat_name: str) -> list[str]:
        words = [w.lower() for w in re.findall(r"\w+", title_hint) if len(w) > 1]
        base_tags = [
            "chính hãng",
            "cao cấp",
            "freeship",
            "bảo hành uy tín",
            "giao hàng nhanh",
        ]
        return list(dict.fromkeys(words + base_tags))[:8]

    def _generate_smart_replies(self, buyer_message: str) -> list[str]:
        norm = _strip_accents(buyer_message)

        # 1. Stock / Inventory inquiry
        if any(
            k in norm
            for k in [
                "con hang",
                "con khong",
                "co san",
                "con size",
                "con mau",
                "het hang",
            ]
        ):
            return [
                "Dạ chào bạn, sản phẩm bên shop hiện vẫn còn sẵn hàng ạ! Bạn đặt sớm để shop đóng gói gửi đi ngay trong ngày nhé.",
                "Chào bạn, hàng luôn có sẵn tại kho và được kiểm tra kỹ trước khi gửi. Bạn cần shop tư vấn thêm về size hay màu sắc không ạ?",
                "Dạ shop còn bạn nhé! Mọi đơn đặt trước 16h hôm nay sẽ được giao cho đơn vị vận chuyển ngay ạ.",
            ]

        # 2. Shipping / Delivery time inquiry
        if any(
            k in norm
            for k in [
                "bao lau",
                "may ngay",
                "ship",
                "giao hang",
                "khi nao nhan",
                "hoa toc",
            ]
        ):
            return [
                "Dạ chào bạn, shop gửi hàng từ kho ngay trong ngày. Khu vực nội thành nhận sau 1-2 ngày, ngoại tỉnh khoảng 2-3 ngày làm việc ạ.",
                "Chào bạn, shop có hỗ trợ giao hàng hỏa tốc trong 2 giờ hoặc chuyển phát nhanh. Bạn đặt hàng để shop ưu tiên xử lý sớm nhé!",
                "Dạ thời gian giao trung bình từ 1-3 ngày tùy khu vực ạ. Bạn có thể theo dõi trực tiếp lộ trình đơn hàng trên ứng dụng nha!",
            ]

        # 3. Price / Discount / Voucher inquiry
        if any(
            k in norm
            for k in [
                "giam gia",
                "voucher",
                "bop gia",
                "gia si",
                "re hon",
                "ma giam",
                "uu dai",
            ]
        ):
            return [
                "Dạ giá trên sản phẩm hiện đã là mức giá ưu đãi tốt nhất của shop rồi ạ. Bạn nhớ vào trang chủ shop thu thập thêm voucher giảm giá và mã freeship nhé!",
                "Chào bạn, hiện shop đang có chương trình tặng mã giảm giá theo dõi shop và voucher cho đơn hàng mới. Bạn áp dụng ngay ở bước thanh toán nha!",
                "Dạ nếu bạn mua số lượng nhiều, bạn nhắn tin số lượng cụ thể để shop gửi bạn bảng giá ưu đãi kèm quà tặng thêm nhé!",
            ]

        # 4. Quality / Authenticity / Warranty inquiry
        if any(
            k in norm
            for k in [
                "chinh hang",
                "auth",
                "chat luong",
                "bao hanh",
                "doi tra",
                "that khong",
            ]
        ):
            return [
                "Dạ shop cam kết sản phẩm 100% chính hãng, có bảo hành đầy đủ và hỗ trợ đổi trả miễn phí trong 7 ngày nếu có lỗi từ nhà sản xuất ạ!",
                "Chào bạn, sản phẩm chuẩn như mô tả và hình ảnh shop đăng. Bạn được kiểm tra hàng thoải mái trước khi thanh toán nên hoàn toàn yên tâm nhé!",
                "Dạ sản phẩm đi kèm phiếu bảo hành chính hãng và tem niêm phong. Shop luôn đặt uy tín và sự hài lòng của khách hàng lên hàng đầu ạ.",
            ]

        # 5. Size / Consultation inquiry
        if any(
            k in norm
            for k in [
                "size",
                "kich thuoc",
                "cao",
                "nang",
                "mac vua",
                "vua khong",
                "tu van",
            ]
        ):
            return [
                "Dạ bạn cho shop xin thêm thông tin chiều cao và cân nặng để shop tư vấn chuẩn size nhất cho bạn nhé!",
                "Chào bạn, mẫu này form chuẩn regular/oversize thoải mái. Nếu bạn thích mặc rộng rãi một chút có thể tăng 1 size nha!",
                "Dạ bạn xem bảng quy đổi kích thước ở phần mô tả sản phẩm, hoặc nhắn tin số đo 3 vòng để shop chọn size vừa vặn nhất cho bạn ạ.",
            ]

        # Default smart replies
        return [
            "Dạ chào bạn! Shop có thể hỗ trợ thông tin chi tiết gì cho bạn về sản phẩm này ạ?",
            "Dạ cảm ơn bạn đã quan tâm đến sản phẩm của shop. Bạn cần tư vấn thêm về tính năng, màu sắc hay khuyến mãi cứ nhắn shop nhé!",
            "Chào bạn! Sản phẩm đang có sẵn và có nhiều ưu đãi hôm nay. Bạn đặt hàng ngay để nhận quà tặng kèm nhé!",
        ]
