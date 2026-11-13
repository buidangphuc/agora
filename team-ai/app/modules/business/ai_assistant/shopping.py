import asyncio
import re
import uuid
from dataclasses import dataclass
from typing import Any, Final, Protocol

from loguru import logger

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.grounding import ListingGrounding
from app.modules.business.ai_assistant.schemas import (
    ProductCard,
    ShoppingAssistantRequest,
    ShoppingAssistantResponse,
)
from app.modules.business.ai_assistant.text import strip_accents

MOCK_BACKEND: Final = "mock"
LLM_BACKEND: Final = "llm_router"
MODEL_NAME: Final = "assistant-v1"

RAG_UNAVAILABLE_REPLY: Final = (
    "Dạ xin lỗi bạn, hiện mình chưa tra cứu được kho sản phẩm nên chưa gợi ý được "
    "sản phẩm cụ thể. Bạn thử lại sau ít phút hoặc mô tả thêm nhu cầu để mình tư vấn nhé!"
)


@dataclass(frozen=True, slots=True)
class AssistantReply:
    text: str
    backend: str


class AssistantReplier(Protocol):
    async def compose(
        self, message: str, cards: list[ProductCard]
    ) -> AssistantReply: ...


class MockAssistantReplier:
    async def compose(self, message: str, cards: list[ProductCard]) -> AssistantReply:
        return AssistantReply(_build_assistant_reply(message, cards), MOCK_BACKEND)


_SYSTEM_PROMPT: Final = (
    "Bạn là trợ lý mua sắm. Chỉ giới thiệu các sản phẩm trong danh sách, "
    "giữ nguyên tên và giá, in đậm tên sản phẩm bằng **tên**. "
    "Không bịa sản phẩm hay giá."
)
_BOLD = re.compile(r"\*\*(.+?)\*\*")
_PRICE_MENTION = re.compile(r"(\d[\d.,]*)\s*(?:đ|₫|vnđ|vnd)(?!\w)", re.IGNORECASE)


def _reply_matches_cards(text: str, cards: list[ProductCard]) -> bool:
    titles = {card.title.casefold() for card in cards}
    prices = {card.price for card in cards}
    bold = [b.strip().casefold() for b in _BOLD.findall(text)]
    if not bold or any(b not in titles for b in bold):
        return False
    for raw in _PRICE_MENTION.findall(text):
        digits = re.sub(r"[.,]", "", raw)
        if not digits or int(digits) not in prices:
            return False
    return True


class LLMAssistantReplier:
    def __init__(self, llm: Any, *, timeout_seconds: float) -> None:
        self._llm = llm
        self._timeout_seconds = timeout_seconds
        self._fallback = MockAssistantReplier()

    async def compose(self, message: str, cards: list[ProductCard]) -> AssistantReply:
        facts = "\n".join(
            f"- {c.title} | giá {c.price} {c.currency}"
            + (f" | đánh giá {c.rating_text}" if c.rating_text else "")
            for c in cards
        )
        messages = [
            ("system", _SYSTEM_PROMPT),
            ("human", f"Câu hỏi: {message}\nSản phẩm:\n{facts}"),
        ]
        try:
            result = await asyncio.wait_for(
                self._llm.ainvoke(messages), timeout=self._timeout_seconds
            )
            text = str(result.content).strip()
        except Exception as exc:
            logger.warning("shopping_assistant.llm_failed error={}", type(exc).__name__)
            return await self._fallback.compose(message, cards)
        if not _reply_matches_cards(text, cards):
            logger.warning("shopping_assistant.llm_reply_rejected")
            return await self._fallback.compose(message, cards)
        return AssistantReply(text, LLM_BACKEND)


class ShoppingAssistantService:
    def __init__(
        self,
        grounding: ListingGrounding,
        replier: AssistantReplier,
        *,
        redaction: RedactionPolicy,
    ) -> None:
        self._grounding = grounding
        self._replier = replier
        self._redaction = redaction

    async def answer(
        self, request: ShoppingAssistantRequest
    ) -> ShoppingAssistantResponse:
        query = request.message.strip()
        safe_query = self._redaction.redact_text(query)
        logger.info("shopping_assistant.query message={}", safe_query)

        grounded = await self._grounding.ground(query)
        if grounded.failed:
            reply = AssistantReply(RAG_UNAVAILABLE_REPLY, MOCK_BACKEND)
        elif not grounded.cards:
            reply = AssistantReply(_build_assistant_reply(safe_query, []), MOCK_BACKEND)
        else:
            reply = await self._replier.compose(safe_query, grounded.cards)

        model_version = f"{reply.backend}:{MODEL_NAME}"
        if grounded.search_model_version:
            model_version += f"|{grounded.search_model_version}"
        return ShoppingAssistantResponse(
            reply_text=reply.text,
            product_cards=grounded.cards,
            suggested_followups=_build_suggested_followups(query),
            request_id=uuid.uuid4().hex,
            model_version=model_version,
        )


def _build_assistant_reply(query: str, products: list[ProductCard]) -> str:
    if not products:
        return (
            f'Dạ chào bạn! Hiện tại mình chưa tìm thấy sản phẩm chính xác cho yêu cầu "{query}". '
            "Bạn có thể thử tìm với các từ khóa phổ biến như 'áo thun', 'tai nghe', 'giày sneaker', hoặc 'bình giữ nhiệt' nha!"
        )

    top_item = products[0]
    count = len(products)
    rating = f", đánh giá {top_item.rating_text}" if top_item.rating_text else ""
    return (
        f'Dạ chào bạn! Dựa trên tìm kiếm "{query}", AI Assistant tìm thấy {count} sản phẩm phù hợp. '
        f"Nổi bật nhất là **{top_item.title}** với giá **{top_item.price:,.0f}đ**{rating}. "
        "Bạn tham khảo các thẻ sản phẩm bên dưới để xem chi tiết nhé!"
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
