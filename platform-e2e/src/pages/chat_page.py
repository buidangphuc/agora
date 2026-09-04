"""Chat / conversations list (`/chat`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class ChatPage(BasePage):
    path = routes.CHAT
    name = "chat"

    @property
    def heading(self) -> Locator:
        return self.page.get_by_text("Trò chuyện", exact=False).first

    def is_displayed(self) -> bool:
        return "/chat" in self.page.url
