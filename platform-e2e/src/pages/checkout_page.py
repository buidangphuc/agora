"""Checkout wizard (`/checkout`): Địa chỉ -> Vận chuyển -> Thanh toán -> Xác nhận.

The step and selections live in the URL (`step`, `addr`, `pay`, `voucher`). Placing an
order requires a saved address. Business assertions belong in step definitions.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator

from src.constants import routes, timeouts
from src.core.base_page import BasePage

STEPS = ("address", "shipping", "payment", "confirm")


class CheckoutPage(BasePage):
    path = routes.CHECKOUT
    name = "checkout"

    @property
    def checkout_unavailable_notice(self) -> Locator:
        # Rendered instead of the wizard when `checkout-enabled` is OFF.
        return self.page.get_by_text("Thanh toán tạm thời không khả dụng", exact=False)

    # ── Shell and stepper ─────────────────────────────────────────────────
    @property
    def stepper(self) -> Locator:
        return self.page.get_by_role("navigation", name="Progress")

    @property
    def current_step_item(self) -> Locator:
        return self.stepper.locator('li[aria-current="step"]')

    @property
    def global_search(self) -> Locator:
        return self.page.get_by_placeholder("Tìm kiếm sản phẩm")

    # ── Step actions ──────────────────────────────────────────────────────
    @property
    def continue_action(self) -> Locator:
        # "Tiếp tục" is a link (enabled) or a disabled button; the mobile bar has a twin.
        return (
            self.page.get_by_role("link", name="Tiếp tục")
            .or_(self.page.get_by_role("button", name="Tiếp tục"))
            .first
        )

    @property
    def back_link(self) -> Locator:
        return self.page.get_by_role("link", name="Quay lại", exact=True).first

    # ── Address step ──────────────────────────────────────────────────────
    @property
    def change_address_button(self) -> Locator:
        return self.page.get_by_role("button", name="Thay đổi")

    @property
    def dialog(self) -> Locator:
        return self.page.get_by_role("dialog")

    def address_option(self, recipient: str) -> Locator:
        return self.dialog.get_by_role("radio", name=re.compile(re.escape(recipient)))

    @property
    def confirm_address_button(self) -> Locator:
        return self.dialog.get_by_role("button", name="Xác nhận")

    @property
    def add_address_button(self) -> Locator:
        return self.page.get_by_role("button", name="Thêm địa chỉ", exact=True)

    # ── Payment step ──────────────────────────────────────────────────────
    @property
    def payment_radios(self) -> Locator:
        return self.page.locator('input[name="paymentMethod"]')

    @property
    def checked_payment_radio(self) -> Locator:
        return self.page.locator('input[name="paymentMethod"]:checked')

    # ── Voucher (selector row on the Thanh toán step, modal) ──────────────
    @property
    def voucher_open_button(self) -> Locator:
        return self.page.get_by_role("button", name="Chọn hoặc nhập mã").or_(
            self.page.get_by_role("button", name="Đổi mã")
        )

    @property
    def voucher_input(self) -> Locator:
        return self.page.locator('input[name="voucher_code"]')

    @property
    def apply_voucher_button(self) -> Locator:
        return self.dialog.get_by_role("button", name="Áp dụng")

    @property
    def voucher_discount(self) -> Locator:
        return self.page.get_by_test_id("voucher-discount")

    @property
    def order_total(self) -> Locator:
        return self.page.get_by_test_id("order-total")

    @property
    def voucher_error(self) -> Locator:
        # FormItem help text under the code input (aria-live), inside the modal.
        return self.dialog.locator('p[aria-live="polite"]')

    # ── Confirm step ──────────────────────────────────────────────────────
    @property
    def place_order_button(self) -> Locator:
        return self.page.get_by_role("button", name="Đặt hàng").first

    @property
    def saga_alert(self) -> Locator:
        return self.page.get_by_test_id("saga-alert-slot").get_by_role("alert")

    @property
    def alert_retry_button(self) -> Locator:
        return self.page.get_by_test_id("saga-alert-slot").get_by_role("button", name="Thử lại")

    @property
    def alert_back_to_cart_link(self) -> Locator:
        return self.page.get_by_test_id("saga-alert-slot").get_by_role(
            "link", name="Quay lại giỏ hàng"
        )

    # ── Helpers ───────────────────────────────────────────────────────────
    def current_step(self) -> str:
        m = re.search(r"[?&]step=([a-z]+)", self.page.url)
        return m.group(1) if m else "address"

    def continue_to(self, step: str) -> None:
        """Click "Tiếp tục" until the URL shows `step` (address -> ... -> confirm)."""
        target = STEPS.index(step)
        while STEPS.index(self.current_step()) < target:
            before = self.current_step()
            self.continue_action.click()
            self.page.wait_for_url(
                re.compile(rf".*[?&]step=(?!{before}\b)[a-z]+"), timeout=timeouts.NAVIGATION
            )

    def is_displayed(self) -> bool:
        return "/checkout" in self.page.url
