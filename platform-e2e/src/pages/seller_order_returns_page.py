"""Seller order page, returns tab (`/seller/orders/{order_id}?tab=returns`).

Rows are found by the return id (`data-return-id`). Assertions live in the steps.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerOrderReturnsPage(BasePage):
    path = routes.SELLER_ORDER_RETURNS
    name = "seller order returns"

    @property
    def section(self) -> Locator:
        return self.page.get_by_test_id("seller-returns")

    def row(self, return_id: str) -> Locator:
        return self.page.locator(f'[data-return-id="{return_id}"]')

    def status(self, return_id: str) -> Locator:
        return self.row(return_id).get_by_test_id("return-status")

    def refund_state(self, return_id: str) -> Locator:
        return self.row(return_id).get_by_test_id("return-refund-state")

    def cod_message(self, return_id: str) -> Locator:
        return self.row(return_id).get_by_test_id("return-cod-message")

    def action(self, return_id: str, name: str) -> Locator:
        """`approve`, `reject` or `refund` button of one return."""
        return self.row(return_id).get_by_test_id(f"return-{name}")

    @property
    def refund_confirm(self) -> Locator:
        return self.page.get_by_test_id("return-refund-confirm")

    @property
    def payment_summary(self) -> Locator:
        return self.page.get_by_test_id("payment-summary")

    @property
    def payment_unavailable(self) -> Locator:
        return self.page.get_by_test_id("payment-unavailable")

    @property
    def error_message(self) -> Locator:
        """The error the page shows (toast or alert) once an action fails."""
        return self.page.get_by_role("alert").filter(has_text=re.compile(r"\S"))

    def click_action(self, return_id: str, name: str) -> None:
        button = self.action(return_id, name)
        button.wait_for(state="visible")
        self.wait_until_interactive(button)
        button.click()

    def confirm_refund(self) -> None:
        self.refund_confirm.wait_for(state="visible")
        self.wait_until_interactive(self.refund_confirm)
        self.refund_confirm.click()

    def is_displayed(self) -> bool:
        return "/seller/orders/" in self.page.url
