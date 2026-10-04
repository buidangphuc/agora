"""Vouchers Hub Page (`/vouchers`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class VouchersPage(BasePage):
    path = routes.VOUCHERS
    name = "vouchers"

    @property
    def voucher_cards(self) -> Locator:
        """Real vouchers from team-promotion, one card each (`data-code` = voucher code)."""
        return self.page.get_by_test_id("voucher-card")

    def card_by_code(self, code: str) -> Locator:
        return self.page.locator(f'[data-testid="voucher-card"][data-code="{code}"]')

    @property
    def save_voucher_buttons(self) -> Locator:
        """Must stay empty: there is no claim backend, so "Lưu mã" is hidden."""
        return self.page.get_by_role("button", name="Lưu mã")

    @property
    def tabs(self) -> Locator:
        return self.page.get_by_role("navigation", name="Tabs")

    def tab(self, label: str) -> Locator:
        """A voucher tab link by its label (the count badge is part of the name)."""
        return self.tabs.get_by_role("link", name=label)

    @property
    def empty_action(self) -> Locator:
        return self.page.get_by_role("link", name="Xem sản phẩm")

    # ── Create-voucher form + list rows (seller/admin voucher management) ──
    @property
    def create_form(self) -> Locator:
        return self.page.get_by_label("Create voucher")

    @property
    def voucher_rows(self) -> Locator:
        return self.page.get_by_test_id("voucher-row")

    def row_by_code(self, code: str) -> Locator:
        return self.page.locator(f'[data-testid="voucher-row"][data-code="{code}"]')

    def is_displayed(self) -> bool:
        return "/vouchers" in self.page.url
