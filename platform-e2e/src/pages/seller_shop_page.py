"""Seller shop profile (`/seller/shop`): the shop display-name form."""

from __future__ import annotations

import re

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerShopPage(BasePage):
    app = "seller"
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
        return self.page.get_by_text("Đã lưu thông tin gian hàng", exact=True)

    @property
    def blank_error(self) -> Locator:
        return self.page.get_by_text("Nhập tên gian hàng.", exact=True)

    @property
    def too_long_error(self) -> Locator:
        return self.page.get_by_text("Tên gian hàng tối đa 80 ký tự.", exact=True)

    @property
    def banner_file_input(self) -> Locator:
        return self.page.locator('input[type="file"]')

    @property
    def banner_preview(self) -> Locator:
        return self.page.get_by_role("img", name=re.compile("banner", re.I))

    @property
    def tagline_input(self) -> Locator:
        return self.page.get_by_label("Tagline", exact=False)

    @property
    def theme_select(self) -> Locator:
        return self.page.get_by_label("Giao diện", exact=False)

    @property
    def picker_search(self) -> Locator:
        return self.page.get_by_label("Tìm sản phẩm", exact=False)

    @property
    def cap_line(self) -> Locator:
        return self.page.get_by_text("Tối đa 6 sản phẩm nổi bật.", exact=True)

    @property
    def error_alert(self) -> Locator:
        return self.page.get_by_role("alert")

    def add_button_for(self, title: str) -> Locator:
        return (
            self.page.get_by_role("listitem")
            .filter(has_text=title)
            .get_by_role("button", name=re.compile("Thêm"))
        )

    def selected_titles(self) -> Locator:
        return self.page.get_by_role("listitem").filter(
            has=self.page.get_by_role("button", name=re.compile(r"^Xóa "))
        )

    def is_displayed(self) -> bool:
        return self.name_input.is_visible()
