from typing import Final

from loguru import logger

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.retrieval import ProductRetriever
from app.modules.business.ai_assistant.schemas import (
    ProductCard,
    ShoppingAssistantRequest,
    ShoppingAssistantResponse,
)
from app.modules.business.ai_assistant.text import strip_accents

RAG_UNAVAILABLE_REPLY: Final = (
    "Dạ xin lỗi bạn, hiện mình chưa tra cứu được kho sản phẩm nên chưa gợi ý được "
    "sản phẩm cụ thể. Bạn thử lại sau ít phút hoặc mô tả thêm nhu cầu để mình tư vấn nhé!"
)


class ShoppingAssistantService:
    def __init__(
        self, retriever: ProductRetriever, *, redaction: RedactionPolicy
    ) -> None:
        self._retriever = retriever
        self._redaction = redaction

    async def answer(
        self, request: ShoppingAssistantRequest
    ) -> ShoppingAssistantResponse:
        query = request.message.strip()
        logger.info(
            "shopping_assistant.query user_id={} message={}",
            request.user_id,
            self._redaction.redact_text(query),
        )

        retrieval_failed = False
        try:
            matched_products = await self._retriever.retrieve(
                query, top_k=request.top_k
            )
        except Exception as exc:  # fail open: the assistant still answers
            # The exception text (backend/embedding errors, never the query) keeps
            # the failure diagnosable.
            logger.warning(
                "shopping_assistant.rag_failed error={}: {}",
                type(exc).__name__,
                str(exc)[:200],
            )
            matched_products = []
            retrieval_failed = True

        if retrieval_failed:
            reply_text = RAG_UNAVAILABLE_REPLY
        else:
            reply_text = _build_assistant_reply(query, matched_products)

        return ShoppingAssistantResponse(
            reply_text=reply_text,
            product_cards=matched_products,
            suggested_followups=_build_suggested_followups(query),
        )


def _build_assistant_reply(query: str, products: list[ProductCard]) -> str:
    if not products:
        return (
            f'Dạ chào bạn! Hiện tại mình chưa tìm thấy sản phẩm chính xác cho yêu cầu "{query}". '
            "Bạn có thể thử tìm với các từ khóa phổ biến như 'áo thun', 'tai nghe', 'giày sneaker', hoặc 'bình giữ nhiệt' nha!"
        )

    top_item = products[0]
    count = len(products)
    if not top_item.rating_text:  # a retrieved listing: no rating/discount facts
        return (
            f'Dạ chào bạn! Dựa trên tìm kiếm "{query}", AI Assistant tìm thấy {count} sản phẩm phù hợp. '
            f"Nổi bật nhất là **{top_item.title}** với giá **{top_item.price:,.0f}đ**. "
            "Bạn tham khảo các thẻ sản phẩm bên dưới để xem chi tiết nhé!"
        )
    return (
        f'Dạ chào bạn! Dựa trên tìm kiếm "{query}", AI Assistant gợi ý cho bạn {count} sản phẩm tốt nhất. '
        f"Nổi bật nhất là **{top_item.title}** với giá chỉ **{top_item.price:,.0f}đ** "
        f"({top_item.discount_rate}% giảm giá, đánh giá {top_item.rating_text}). "
        "Bạn tham khảo các thẻ sản phẩm bên dưới để xem chi tiết và đặt hàng nhé!"
    )


def _build_suggested_followups(query: str) -> list[str]:
    norm = strip_accents(query)
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
