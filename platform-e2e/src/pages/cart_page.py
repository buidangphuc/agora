"""Cart page (`/cart`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class CartPage(BasePage):
    path = routes.CART
    name = "cart"

    @property
    def checkout_link(self) -> Locator:
        # Anchor to /checkout labelled "Mua Hàng (n)".
        return self.page.locator('a[href="/checkout"]')

    @property
    def empty_state(self) -> Locator:
        return self.page.get_by_text("Giỏ hàng của bạn đang trống", exact=False)

    @property
    def checkout_unavailable_notice(self) -> Locator:
        # Shown (server-side) when the `checkout-enabled` kill-switch is OFF.
        return self.page.get_by_text("Thanh toán tạm thời không khả dụng", exact=False)

    def is_empty(self) -> bool:
        return self.empty_state.is_visible()

    def is_displayed(self) -> bool:
        return "/cart" in self.page.url

    def proceed_to_checkout(self) -> None:
        self.checkout_link.click()
