"""Seller Orders & Fulfillment Management Page (`/seller/orders`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerOrdersPage(BasePage):
    path = routes.SELLER_ORDERS
    name = "seller orders"

    @property
    def orders_table(self) -> Locator:
        return self.page.locator("table").first

    @property
    def order_rows(self) -> Locator:
        return self.page.locator("tbody tr")

    @property
    def fulfill_button(self) -> Locator:
        return self.page.get_by_role("link", name="Xử lý giao hàng").first

    @property
    def print_packing_slip_button(self) -> Locator:
        return self.page.get_by_role("button", name="In phiếu đóng gói").first

    def is_displayed(self) -> bool:
        return "/seller/orders" in self.page.url
