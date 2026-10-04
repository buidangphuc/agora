"""Seller Orders Table List (`/seller/orders`): status Tabs, search, Table, Pagination."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerOrdersPage(BasePage):
    path = routes.SELLER_ORDERS
    name = "seller orders"

    @property
    def tabs(self) -> Locator:
        return self.page.get_by_role("navigation", name="Tabs")

    def tab(self, label: str) -> Locator:
        """A status tab link (its accessible name carries the count, so match by prefix)."""
        return self.tabs.get_by_role("link", name=label)

    @property
    def active_tab(self) -> Locator:
        return self.tabs.locator('a[aria-current="page"]')

    @property
    def orders_table(self) -> Locator:
        return self.page.locator("table").first

    @property
    def order_rows(self) -> Locator:
        return self.page.locator("tbody tr")

    @property
    def search_box(self) -> Locator:
        return self.page.get_by_role("searchbox", name="Tìm đơn hàng")

    @property
    def pagination(self) -> Locator:
        return self.page.get_by_role("navigation", name="Phân trang")

    @property
    def detail_links(self) -> Locator:
        return self.page.get_by_role("link", name="Chi tiết")

    @property
    def fulfill_button(self) -> Locator:
        return self.page.get_by_role("button", name="Xác nhận gửi").first

    def is_displayed(self) -> bool:
        return "/seller/orders" in self.page.url
