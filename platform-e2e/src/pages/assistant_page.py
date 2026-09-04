"""AI Assistant Page (`/assistant`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class AssistantPage(BasePage):
    path = routes.ASSISTANT
    name = "assistant"

    @property
    def chat_input(self) -> Locator:
        return self.page.get_by_placeholder("Nhập câu hỏi", exact=False)

    @property
    def send_button(self) -> Locator:
        return self.page.get_by_role("button", name="Gửi").first

    @property
    def ai_replies(self) -> Locator:
        return self.page.locator("div.rounded-2xl")

    def is_displayed(self) -> bool:
        return "/assistant" in self.page.url
