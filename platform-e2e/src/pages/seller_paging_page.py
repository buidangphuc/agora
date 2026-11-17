"""Locators of the server-paged seller lists (change seller-server-paging-ui).

Covers `/seller/orders` and `/seller` (tabs, rows, cursor pagination, error and empty states) and the
shared listing picker of `/seller/ads` and `/seller/bundles`. The picker contract is
`data-testid="listing-picker"` (root) and `listing-picker-option` (one per option, holding a checkbox
that is checked once chosen). The selection lives in the bundle form, whose label counts it as
"(N đã chọn)" across searches. Assertions live in the steps.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator

from src.constants import routes, timeouts
from src.core.base_page import BasePage, wait_streamed

ORDERS_ERROR = "Không tải được danh sách đơn hàng"
PRODUCTS_ERROR = "Không tải được danh sách sản phẩm"
PICKER_EMPTY = "Không tìm thấy sản phẩm"
PICKER_ERROR = "Không tải được sản phẩm"


class SellerPagingPage(BasePage):
    app = "seller"
    path = routes.SELLER_ORDERS
    name = "seller server paging"

    def is_displayed(self) -> bool:
        return "/seller" in self.page.url

    def open(self, path: str) -> None:
        self.page.goto(f"{self.app_base_url()}{path}", wait_until="domcontentloaded")
        wait_streamed(self.page, timeouts.NAVIGATION)

    @property
    def tabs(self) -> Locator:
        return self.page.get_by_role("navigation", name="Tabs")

    def tab(self, label: str) -> Locator:
        return self.tabs.get_by_role("link", name=re.compile(rf"^{re.escape(label)}"))

    @property
    def order_rows(self) -> Locator:
        return self.page.locator("tbody tr").filter(
            has=self.page.get_by_role("link", name="Chi tiết")
        )

    @property
    def order_links(self) -> Locator:
        return self.page.get_by_role("link", name="Chi tiết")

    @property
    def search_box(self) -> Locator:
        return self.page.get_by_role("searchbox", name="Tìm đơn hàng")

    @property
    def next_link(self) -> Locator:
        return self.page.get_by_role("link", name="Trang sau")

    @property
    def orders_error(self) -> Locator:
        return self.page.get_by_text(ORDERS_ERROR, exact=True)

    @property
    def products_error(self) -> Locator:
        return self.page.get_by_text(PRODUCTS_ERROR, exact=True)

    @property
    def retry_links(self) -> Locator:
        return self.page.get_by_role("link", name="Thử lại")

    @property
    def no_results(self) -> Locator:
        return self.page.get_by_text("Không có kết quả", exact=True)

    @property
    def clear_filters(self) -> Locator:
        return self.page.get_by_role("link", name="Xoá bộ lọc")

    @property
    def kpi_row(self) -> Locator:
        return self.page.get_by_test_id("kpi-row")

    def kpi(self, title: str) -> Locator:
        return self.kpi_row.locator(":scope > div").filter(has_text=title)

    def product_rows(self, tag: str) -> Locator:
        return self.page.locator("tbody tr").filter(has_text=tag)

    @property
    def product_search(self) -> Locator:
        return self.page.get_by_role("searchbox", name="Tìm sản phẩm")

    @property
    def onboarding_button(self) -> Locator:
        return self.page.get_by_role("button", name="Đồng ý và trở thành Người Bán")

    @property
    def picker(self) -> Locator:
        return self.page.get_by_test_id("listing-picker")

    @property
    def picker_search(self) -> Locator:
        return self.picker.get_by_role("textbox", name="Tìm sản phẩm")

    @property
    def picker_options(self) -> Locator:
        return self.picker.get_by_test_id("listing-picker-option")

    def picker_checkbox(self, option: Locator) -> Locator:
        return option.get_by_role("checkbox")

    @property
    def picker_checked(self) -> Locator:
        return self.picker_options.filter(has=self.page.get_by_role("checkbox", checked=True))

    def selection_count(self, n: int) -> Locator:
        return self.page.get_by_text(f"({n} đã chọn)")

    @property
    def picker_empty(self) -> Locator:
        return self.picker.get_by_text(PICKER_EMPTY, exact=True)

    @property
    def picker_error(self) -> Locator:
        return self.picker.get_by_text(PICKER_ERROR, exact=True)

    @property
    def picker_retry(self) -> Locator:
        return self.picker.get_by_role("button", name="Thử lại")

    @property
    def bundle_name(self) -> Locator:
        return self.page.get_by_label("Tên combo")

    @property
    def bundle_price(self) -> Locator:
        return self.page.get_by_label("Giá combo")

    @property
    def create_bundle(self) -> Locator:
        return self.page.get_by_role("button", name="Tạo combo")
