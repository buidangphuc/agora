"""Checkout page (`/checkout`).

Placing an order requires a saved address; this slice verifies the checkout page
and its place-order control render, without depending on seeded addresses.
"""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class CheckoutPage(BasePage):
    path = routes.CHECKOUT
    name = "checkout"

    @property
    def checkout_unavailable_notice(self) -> Locator:
        # Rendered instead of CheckoutView when `checkout-enabled` is OFF.
        return self.page.get_by_text("Thanh toán tạm thời không khả dụng", exact=False)

    @property
    def place_order_button(self) -> Locator:
        # Label is "Xác nhận đặt hàng" (COD) or "Tiến hành thanh toán Demo".
        return self.page.get_by_role("button", name="đặt hàng", exact=False).or_(
            self.page.get_by_role("button", name="thanh toán Demo", exact=False)
        )

    # ── Voucher / promo slot (team-promotion redemption at checkout) ──────
    @property
    def voucher_input(self) -> Locator:
        return self.page.locator('input[name="voucher_code"]')

    @property
    def apply_voucher_button(self) -> Locator:
        return self.page.get_by_role("button", name="Apply")  # aria-label="Apply"

    @property
    def voucher_discount(self) -> Locator:
        return self.page.get_by_test_id("voucher-discount")

    @property
    def order_total(self) -> Locator:
        return self.page.get_by_test_id("order-total")

    @property
    def voucher_error(self) -> Locator:
        # Rendered as <p role="alert"> under the voucher input when the code is invalid.
        # Scoped to the paragraph (not the toast live-region, which also uses role=alert).
        return self.page.locator('p[role="alert"]')

    def apply_voucher(self, code: str) -> None:
        self.voucher_input.fill(code)
        self.apply_voucher_button.click()

    def is_displayed(self) -> bool:
        return "/checkout" in self.page.url
