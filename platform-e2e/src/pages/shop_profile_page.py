"""Shop Public Profile Page (`/shop/[id]`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class ShopProfilePage(BasePage):
    path = routes.SHOP
    name = "shop profile"

    @property
    def shop_banner_title(self) -> Locator:
        return self.page.locator("h1, h2").first

    @property
    def follow_button(self) -> Locator:
        return self.page.get_by_role("button", name="+ Theo Dõi").first

    @property
    def chat_button(self) -> Locator:
        return self.page.get_by_role("button", name="Chat Ngay").first

    def is_displayed(self) -> bool:
        return "/shop/" in self.page.url
