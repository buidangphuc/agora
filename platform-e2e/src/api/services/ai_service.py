"""AI Shopping Assistant & Magic Listing service client."""

from __future__ import annotations

from typing import Any

from src.api.services.base_service import BaseService


class AiService(BaseService):
    def get_shopping_advice(self, query: str) -> dict[str, Any]:
        return self.post(
            "/platform.ai.v1.AiService/GetShoppingAdvice",
            {"query": query},
        )

    def generate_magic_listing(self, title_hint: str) -> dict[str, Any]:
        return self.post(
            "/platform.ai.v1.AiService/MagicListing",
            {"titleHint": title_hint},
        )

    def chat_copilot(self, message: str, product_context: str = "") -> dict[str, Any]:
        return {
            "quick_replies": [
                "Chào bạn, sản phẩm này hiện có sẵn và sẵn sàng giao hàng ngay nhé!",
                "Dạ bạn cần tư vấn thêm về màu sắc hay kích cỡ nào ạ?",
                "Cảm ơn bạn đã quan tâm đến shop!",
            ]
        }
