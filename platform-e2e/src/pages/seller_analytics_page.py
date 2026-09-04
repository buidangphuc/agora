"""Seller Analytics & Wallet Dashboard Page (`/seller/analytics`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerAnalyticsPage(BasePage):
    path = routes.SELLER_ANALYTICS
    name = "seller analytics"

    @property
    def revenue_metric_card(self) -> Locator:
        return self.page.get_by_text("Doanh Thu Hôm Nay", exact=False)

    @property
    def wallet_balance_text(self) -> Locator:
        return self.page.get_by_text("Số Dư Ví Người Bán", exact=False).first

    @property
    def payout_button(self) -> Locator:
        return self.page.get_by_role("button", name="Rút Tiền Về Ngân Hàng").first

    @property
    def payout_amount_input(self) -> Locator:
        return self.page.locator('input[name="payout_amount"], input[type="number"]').first

    def is_displayed(self) -> bool:
        return "/seller/analytics" in self.page.url
