import re

from loguru import logger

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.schemas import (
    MagicListingRequest,
    MagicListingResponse,
)
from app.modules.business.ai_assistant.text import strip_accents


class ListingCopywriter:
    def __init__(self, *, redaction: RedactionPolicy) -> None:
        self._redaction = redaction

    def generate(self, request: MagicListingRequest) -> MagicListingResponse:
        title_hint = request.title_hint.strip()
        logger.info(
            "magic_listing.generate hint={}", self._redaction.redact_text(title_hint)
        )

        category_id, cat_name = _infer_category(title_hint, request.category_hint)
        generated_title = _generate_seo_title(title_hint, cat_name)
        generated_desc = _generate_seo_description(
            title_hint, cat_name, generated_title
        )
        price_min, price_max = _estimate_price_range(title_hint)
        highlight_tags = _generate_tags(title_hint)

        return MagicListingResponse(
            generated_title=generated_title,
            generated_description=generated_desc,
            suggested_category_id=category_id,
            suggested_price_min=price_min,
            suggested_price_max=price_max,
            highlight_tags=highlight_tags,
        )


def _infer_category(title_hint: str, category_hint: str = "") -> tuple[str, str]:
    if category_hint:
        return category_hint, category_hint

    norm = strip_accents(title_hint)
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


def _generate_seo_title(title_hint: str, cat_name: str) -> str:
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
    title_hint: str, cat_name: str, generated_title: str
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


def _estimate_price_range(title_hint: str) -> tuple[int, int]:
    norm = strip_accents(title_hint)
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


def _generate_tags(title_hint: str) -> list[str]:
    words = [w.lower() for w in re.findall(r"\w+", title_hint) if len(w) > 1]
    base_tags = [
        "chính hãng",
        "cao cấp",
        "freeship",
        "bảo hành uy tín",
        "giao hàng nhanh",
    ]
    return list(dict.fromkeys(words + base_tags))[:8]
