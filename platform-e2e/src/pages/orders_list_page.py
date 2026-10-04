"""Buyer order list (`/account/orders`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class OrdersListPage(BasePage):
    path = routes.ACCOUNT_ORDERS
    name = "account orders"

    @property
    def heading(self) -> Locator:
        return self.page.get_by_role("heading", name="Đơn hàng của tôi")

    @property
    def tabs(self) -> Locator:
        return self.page.get_by_role("navigation", name="Tabs")

    def tab(self, label: str) -> Locator:
        return self.tabs.get_by_role("link", name=label)

    @property
    def order_cards(self) -> Locator:
        return self.page.get_by_test_id("order-card")

    @property
    def order_statuses(self) -> Locator:
        return self.page.get_by_test_id("order-status")

    @property
    def pagination(self) -> Locator:
        return self.page.get_by_role("navigation", name="Phân trang")

    def page_link(self, number: int) -> Locator:
        return self.pagination.get_by_role("link", name=f"Trang {number}")

    @property
    def empty_text(self) -> Locator:
        return self.page.get_by_text("Chưa có đơn hàng nào ở trạng thái này")

    @property
    def view_all_link(self) -> Locator:
        return self.page.get_by_role("link", name="Xem tất cả đơn hàng")

    @property
    def detail_links(self) -> Locator:
        return self.page.get_by_role("link", name="Xem chi tiết")

    def is_displayed(self) -> bool:
        return "/account/orders" in self.page.url
