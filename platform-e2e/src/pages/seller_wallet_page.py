"""Seller wallet (`/seller/wallet`): balance Statistic, payout confirm Modal, ledger."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerWalletPage(BasePage):
    path = routes.SELLER_WALLET
    name = "seller wallet"

    @property
    def balance_label(self) -> Locator:
        return self.page.get_by_text("Số dư khả dụng", exact=True).first

    @property
    def payout_button(self) -> Locator:
        return self.page.get_by_role("button", name="Rút tiền", exact=True)

    @property
    def zero_balance_hint(self) -> Locator:
        return self.page.get_by_text("Số dư bằng 0, chưa thể rút tiền.", exact=True)

    @property
    def confirm_dialog(self) -> Locator:
        return self.page.get_by_role("dialog")

    @property
    def confirm_payout(self) -> Locator:
        return self.confirm_dialog.get_by_role("button", name="Xác nhận rút tiền")

    @property
    def payout_toast(self) -> Locator:
        return self.page.get_by_text("Đã tạo lệnh rút tiền", exact=True)

    def is_displayed(self) -> bool:
        return "/seller/wallet" in self.page.url
