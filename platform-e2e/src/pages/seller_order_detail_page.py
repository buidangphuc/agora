"""Seller order detail / Advanced Profile (`/seller/orders/{order_id}`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerOrderDetailPage(BasePage):
    path = routes.SELLER_ORDER_DETAIL
    name = "seller order detail"

    @property
    def packing_slip(self) -> Locator:
        return self.page.get_by_test_id("packing-slip")

    @property
    def stepper(self) -> Locator:
        return self.page.get_by_role("navigation", name="Progress")

    @property
    def current_step(self) -> Locator:
        return self.stepper.locator('li[aria-current="step"]')

    @property
    def ship_button(self) -> Locator:
        return self.page.get_by_role("button", name="Bàn giao vận chuyển", exact=True)

    @property
    def print_button(self) -> Locator:
        return self.page.get_by_role("button", name="In phiếu", exact=True)

    @property
    def confirm_dialog(self) -> Locator:
        return self.page.get_by_role("dialog")

    @property
    def confirm_ship(self) -> Locator:
        return self.confirm_dialog.get_by_role("button", name="Xác nhận bàn giao")

    @property
    def shipped_toast(self) -> Locator:
        return self.page.get_by_text("Đã bàn giao vận chuyển", exact=True)

    @property
    def not_found(self) -> Locator:
        return self.page.get_by_text("Không tìm thấy nội dung", exact=True)

    @property
    def back_to_seller_link(self) -> Locator:
        return self.page.get_by_role("link", name="Về Kênh người bán")

    def is_displayed(self) -> bool:
        return "/seller/orders/" in self.page.url
