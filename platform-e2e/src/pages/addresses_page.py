"""Delivery addresses (`/account/addresses`) with an add-address modal."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class AddressesPage(BasePage):
    path = routes.ACCOUNT_ADDRESSES
    name = "addresses"

    @property
    def add_button(self) -> Locator:
        return self.page.get_by_role("button", name="Thêm địa chỉ").first

    @property
    def save_button(self) -> Locator:
        return self.page.get_by_role("button", name="Thêm mới")

    def is_displayed(self) -> bool:
        return "/account/addresses" in self.page.url

    def address_by_recipient(self, name: str) -> Locator:
        return self.page.get_by_text(name, exact=False)

    def add_address(self, recipient: str, phone: str) -> None:
        self.add_button.click()
        self.page.fill('input[name="recipientName"]', recipient)
        self.page.fill('input[name="phone"]', phone)
        self.page.fill('input[name="street"]', "123 Đường Test")
        self.page.fill('input[name="ward"]', "P. Test")
        self.page.fill('input[name="district"]', "Quận 1")
        self.page.fill('input[name="city"]', "TP. HCM")
        self.save_button.click()
