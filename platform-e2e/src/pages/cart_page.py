"""Cart page (`/cart`): shop groups, quantity controls, voucher row and order summary."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class CartPage(BasePage):
    path = routes.CART
    name = "cart"

    @property
    def checkout_link(self) -> Locator:
        # "Mua hàng" link to /checkout (desktop summary card; the mobile bar has a second one).
        return self.page.locator('a[href^="/checkout"]:visible').first

    @property
    def buy_button(self) -> Locator:
        # Link when checkout is enabled, disabled <button> when the kill-switch is off.
        return (
            self.page.get_by_role("link", name="Mua hàng")
            .or_(self.page.get_by_role("button", name="Mua hàng"))
            .first
        )

    @property
    def shop_groups(self) -> Locator:
        return self.page.get_by_test_id("cart-shop-group")

    def shop_header(self, name: str) -> Locator:
        """The group header link showing a shop name (links to /shop/<sellerId>)."""
        return self.shop_groups.get_by_role("link", name=name, exact=True)

    @property
    def increase_quantity_button(self) -> Locator:
        return self.page.get_by_role("button", name="Tăng số lượng").first

    @property
    def clear_button(self) -> Locator:
        return self.page.get_by_role("button", name="Xóa tất cả")

    @property
    def order_summary(self) -> Locator:
        return self.page.get_by_test_id("order-summary")

    @property
    def empty_state(self) -> Locator:
        return self.page.get_by_text("Giỏ hàng của bạn đang trống", exact=False)

    @property
    def continue_shopping_link(self) -> Locator:
        return self.page.get_by_role("link", name="Tiếp tục mua sắm")

    @property
    def checkout_unavailable_notice(self) -> Locator:
        # Shown (server-side) when the `checkout-enabled` kill-switch is OFF.
        return self.page.get_by_text("Thanh toán tạm thời không khả dụng", exact=False)

    def horizontal_overflow(self) -> int:
        """Pixels by which the page is wider than the viewport (0 = none)."""
        return self.page.evaluate(
            "document.documentElement.scrollWidth - document.documentElement.clientWidth"
        )

    def is_empty(self) -> bool:
        return self.empty_state.is_visible()

    def is_displayed(self) -> bool:
        return "/cart" in self.page.url

    def proceed_to_checkout(self) -> None:
        self.checkout_link.click()
