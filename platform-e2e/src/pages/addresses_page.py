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

    # ── Card list, delete confirmation (ui-phase-account) ────────────────
    @property
    def address_cards(self) -> Locator:
        return self.page.get_by_test_id("address-card")

    def address_card(self, recipient: str) -> Locator:
        return self.address_cards.filter(has_text=recipient)

    def delete_button(self, recipient: str) -> Locator:
        return self.address_card(recipient).get_by_role("button", name="Xóa", exact=True)

    @property
    def confirm_dialog(self) -> Locator:
        return self.page.get_by_role("dialog", name="Xóa địa chỉ?")

    @property
    def confirm_delete_button(self) -> Locator:
        return self.confirm_dialog.get_by_role("button", name="Xóa địa chỉ")
