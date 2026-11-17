"""Seller verification page (`/seller/verification`): status label, upload form, consent, submit."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerVerificationPage(BasePage):
    app = "seller"
    path = routes.SELLER_VERIFICATION
    name = "seller verification"

    @property
    def file_inputs(self) -> Locator:
        return self.page.locator("input[type=file]")

    @property
    def consent_checkbox(self) -> Locator:
        return self.page.get_by_role("checkbox")

    @property
    def submit_button(self) -> Locator:
        return self.page.get_by_role("button", name="Gửi hồ sơ xác minh")

    @property
    def retry_link(self) -> Locator:
        return self.page.get_by_role("link", name="Thử lại")

    @property
    def error_alert(self) -> Locator:
        return self.page.get_by_role("alert")

    @property
    def nav_entries(self) -> Locator:
        return self.page.get_by_role("link", name="Xác minh danh tính", exact=True)

    def text(self, value: str) -> Locator:
        return self.page.get_by_text(value, exact=False)

    def is_displayed(self) -> bool:
        return "/seller/verification" in self.page.url
