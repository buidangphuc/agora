from loguru import logger

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    ChatCopilotResponse,
)
from app.modules.business.ai_assistant.text import strip_accents


class ChatCopilotService:
    def __init__(self, *, redaction: RedactionPolicy) -> None:
        self._redaction = redaction

    def suggest(self, request: ChatCopilotRequest) -> ChatCopilotResponse:
        buyer_msg = request.buyer_message.strip()
        logger.info(
            "chat_copilot.replies buyer_message={}",
            self._redaction.redact_text(buyer_msg),
        )

        return ChatCopilotResponse(quick_replies=_generate_smart_replies(buyer_msg))


def _generate_smart_replies(buyer_message: str) -> list[str]:
    norm = strip_accents(buyer_message)

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
