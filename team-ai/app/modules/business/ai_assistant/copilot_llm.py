import asyncio
import json
import re
from typing import Any, Final

from loguru import logger

from app.modules.business.ai_assistant.adapters.listing import ListingFacts
from app.modules.business.ai_assistant.text import strip_accents

COPILOT_PROMPT_VERSION: Final = "copilot-v1"
DRAFT_COUNT: Final = 3

_SYSTEM_PROMPT: Final = (
    "Bạn là trợ lý soạn tin nhắn cho người bán trên sàn thương mại điện tử. "
    "Soạn đúng 3 tin nhắn trả lời ngắn, lịch sự bằng tiếng Việt cho câu hỏi của người mua. "
    "Chỉ khẳng định còn hàng khi thông tin sản phẩm nói còn hàng; nếu hết hàng hoặc "
    "không rõ thì không nói còn hàng. Không hứa thời gian giao hàng cụ thể. "
    "Trả về duy nhất một mảng JSON gồm 3 chuỗi."
)

_NEGATED_AVAILABILITY: Final = (
    "het hang",
    "khong con hang",
    "khong con san",
    "khong co san",
    "chua co san",
    "chua con hang",
    "tam het",
    "khong con",
)
_AVAILABILITY: Final = ("con hang", "con san", "co san", "con bang", "shop con ")
_OUT_OF_STOCK: Final = ("het hang", "khong con hang", "tam het")
_DELIVERY_PROMISE: Final = re.compile(
    r"\d+\s*(?:-\s*\d+\s*)?(?:ngay|gio|tieng|h\b|phut)"
    r"|trong ngay|hoa toc|ngay hom nay|truoc \d+\s*h|ngay lap tuc|trong \d+"
)


def _norm(text: str) -> str:
    return strip_accents(text.replace("đ", "d").replace("Đ", "D"))


def claims_availability(draft: str) -> bool:
    text = _norm(draft)
    for neg in _NEGATED_AVAILABILITY:
        text = text.replace(neg, " ")
    return any(p in text for p in _AVAILABILITY)


def says_out_of_stock(draft: str) -> bool:
    text = _norm(draft)
    return any(p in text for p in _OUT_OF_STOCK)


def promises_delivery(draft: str) -> bool:
    return _DELIVERY_PROMISE.search(_norm(draft)) is not None


def contradicts_facts(draft: str, facts: ListingFacts | None) -> bool:
    if promises_delivery(draft):
        return True
    in_stock = facts is not None and facts.stock > 0
    if claims_availability(draft) and not in_stock:
        return True
    return facts is not None and facts.stock > 0 and says_out_of_stock(draft)


def _parse_drafts(raw: str) -> list[str] | None:
    text = raw.strip()
    if text.startswith("```"):
        text = re.sub(r"^```[a-zA-Z]*\s*|\s*```$", "", text).strip()
    try:
        data = json.loads(text)
    except ValueError:
        return None
    if not isinstance(data, list):
        return None
    drafts = [d.strip() for d in data if isinstance(d, str) and d.strip()]
    if len(drafts) < DRAFT_COUNT:
        return None
    return drafts[:DRAFT_COUNT]


def _facts_text(facts: ListingFacts | None) -> str:
    if facts is None:
        return "Không có thông tin sản phẩm đã xác nhận; không khẳng định còn hàng."
    status = f"còn hàng ({facts.stock})" if facts.stock > 0 else "hết hàng"
    return f"Sản phẩm: {facts.title} | giá {facts.price} {facts.currency} | {status}"


class LLMCopilotDrafter:
    def __init__(self, llm: Any, *, timeout_seconds: float) -> None:
        self._llm = llm
        self._timeout_seconds = timeout_seconds

    async def draft(self, message: str, facts: ListingFacts | None) -> list[str] | None:
        messages = [
            ("system", _SYSTEM_PROMPT),
            ("human", f"Câu hỏi của người mua: {message}\n{_facts_text(facts)}"),
        ]
        try:
            result = await asyncio.wait_for(
                self._llm.ainvoke(messages), timeout=self._timeout_seconds
            )
            drafts = _parse_drafts(str(result.content))
        except Exception as exc:
            logger.warning("chat_copilot.llm_failed error={}", type(exc).__name__)
            return None
        if drafts is None:
            logger.warning("chat_copilot.llm_unusable")
            return None
        if any(contradicts_facts(d, facts) for d in drafts):
            logger.warning("chat_copilot.llm_rejected")
            return None
        return drafts
