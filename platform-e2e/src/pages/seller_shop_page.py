"""Seller shop profile (`/seller/shop`): the shop display-name form."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerShopPage(BasePage):
    path = routes.SELLER_SHOP
    name = "seller shop"

    @property
    def name_input(self) -> Locator:
        return self.page.get_by_label("Tên gian hàng", exact=False)

    @property
    def save_button(self) -> Locator:
        return self.page.get_by_role("button", name="Lưu thay đổi")

    @property
    def saved_toast(self) -> Locator:
        return self.page.get_by_text("Đã lưu tên gian hàng", exact=True)

    @property
    def blank_error(self) -> Locator:
        return self.page.get_by_text("Nhập tên gian hàng.", exact=True)

    @property
    def too_long_error(self) -> Locator:
        return self.page.get_by_text("Tên gian hàng tối đa 80 ký tự.", exact=True)

    def is_displayed(self) -> bool:
        return self.name_input.is_visible()
