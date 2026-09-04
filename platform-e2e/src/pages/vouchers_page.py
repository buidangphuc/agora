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
        return self.page.locator("div.border").filter(has_text="Voucher")

    @property
    def save_voucher_buttons(self) -> Locator:
        return self.page.get_by_role("button", name="Lưu mã")

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
