import asyncio
from typing import Final

from loguru import logger

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.adapters.listing import ListingFacts
from app.modules.business.ai_assistant.grounding import ListingPort
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    ChatCopilotResponse,
)
from app.modules.business.ai_assistant.text import strip_accents

MAX_MESSAGE_CHARS: Final = 1000
MAX_DRAFTS: Final = 3
MAX_DRAFT_CHARS: Final = 300


class ChatCopilotService:
    def __init__(
        self,
        *,
        redaction: RedactionPolicy,
        listing: ListingPort | None = None,
        listing_timeout_s: float = 0.2,
    ) -> None:
        self._redaction = redaction
        self._listing = listing
        self._listing_timeout_s = listing_timeout_s

    async def suggest(self, request: ChatCopilotRequest) -> ChatCopilotResponse:
        buyer_msg = request.buyer_message.strip()[:MAX_MESSAGE_CHARS]
        logger.info(
            "chat_copilot.replies buyer_message={}",
            self._redaction.redact_text(buyer_msg),
        )
        facts = await self._facts(request)
        drafts = _generate_smart_replies(buyer_msg, facts)
        return ChatCopilotResponse(
            quick_replies=[d[:MAX_DRAFT_CHARS] for d in drafts[:MAX_DRAFTS]]
        )

    async def _facts(self, request: ChatCopilotRequest) -> ListingFacts | None:
        if self._listing is None or not request.listing_id:
            return None
        try:
            facts = await asyncio.wait_for(
                self._listing.get(
                    request.listing_id, timeout_s=self._listing_timeout_s
                ),
                timeout=self._listing_timeout_s,
            )
        except Exception as exc:
            logger.warning("chat_copilot.listing_failed error={}", type(exc).__name__)
            return None
        if facts is None or not facts.published or facts.seller_id != request.seller_id:
            return None
        return facts


def _stock_replies(facts: ListingFacts | None) -> list[str]:
    if facts is None:
        return [
            "Dạ chào bạn, shop kiểm tra tình trạng sản phẩm rồi phản hồi bạn sớm nhé ạ.",
            "Chào bạn, bạn cho shop xin thêm size hoặc màu bạn quan tâm để shop kiểm tra giúp bạn ạ.",
            "Dạ bạn chờ shop một chút để xác nhận hàng trong kho rồi báo lại bạn nhé!",
        ]
    if facts.stock <= 0:
        return [
            "Dạ chào bạn, sản phẩm này hiện đã hết hàng ạ. Shop xin lỗi vì sự bất tiện này.",
            "Chào bạn, mẫu này tạm hết hàng rồi ạ. Bạn để lại tin nhắn, shop sẽ báo khi có hàng trở lại nhé!",
            "Dạ sản phẩm này đang hết hàng, bạn xem thử các mẫu tương tự của shop nhé ạ.",
        ]
    return [
        "Dạ chào bạn, sản phẩm này bên shop vẫn còn hàng ạ! Bạn đặt sớm để shop xử lý đơn cho bạn nhé.",
        "Chào bạn, shop còn hàng ạ. Bạn cần shop tư vấn thêm về size hay màu sắc không ạ?",
        "Dạ sản phẩm này shop còn hàng bạn nhé! Bạn chốt đơn để shop chuẩn bị hàng cho bạn ạ.",
    ]


def _generate_smart_replies(
    buyer_message: str, facts: ListingFacts | None
) -> list[str]:
    norm = strip_accents(buyer_message)

    # 1. Shipping / Delivery time inquiry
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
            "Dạ chào bạn, thời gian giao hàng phụ thuộc vào khu vực nhận. Bạn cho shop xin địa chỉ nhận hàng để shop kiểm tra giúp bạn nhé ạ.",
            "Chào bạn, bạn gửi shop địa chỉ nhận hàng, shop sẽ xác nhận với đơn vị vận chuyển và báo lại bạn ạ.",
            "Dạ bạn có thể theo dõi lộ trình đơn hàng trên ứng dụng sau khi đặt nha. Bạn cho shop xin địa chỉ để shop hỗ trợ thêm ạ!",
        ]

    # 2. Stock / Inventory inquiry
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
        return _stock_replies(facts)

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
            "Dạ shop cam kết sản phẩm 100% chính hãng, có bảo hành đầy đủ và hỗ trợ đổi trả theo chính sách của shop nếu có lỗi từ nhà sản xuất ạ!",
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
        "Chào bạn! Bạn nhắn shop điều bạn quan tâm về sản phẩm, shop sẽ phản hồi bạn sớm nhé!",
    ]
