"""Seller Analytics Dashboard Page (`/seller/analytics`): KPI row, funnel, revenue, range Tabs."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerAnalyticsPage(BasePage):
    path = routes.SELLER_ANALYTICS
    name = "seller analytics"

    @property
    def kpi_row(self) -> Locator:
        return self.page.get_by_test_id("kpi-row")

    @property
    def revenue_metric_card(self) -> Locator:
        return self.kpi_row.get_by_text("Doanh thu", exact=True).first

    @property
    def impressions_card(self) -> Locator:
        return self.kpi_row.get_by_text("Lượt hiển thị", exact=True).first

    @property
    def range_tabs(self) -> Locator:
        return self.page.get_by_role("navigation", name="Tabs")

    def range_tab(self, label: str) -> Locator:
        return self.range_tabs.get_by_role("link", name=label, exact=True)

    @property
    def active_range_tab(self) -> Locator:
        return self.range_tabs.locator('a[aria-current="page"]')

    @property
    def revenue_table(self) -> Locator:
        return self.page.get_by_role("table", name="Doanh thu theo ngày")

    def is_displayed(self) -> bool:
        return "/seller/analytics" in self.page.url
