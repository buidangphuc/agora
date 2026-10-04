"""Payment page / outcome (`/checkout/pay/[id]`): summary, simulator and Result states."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class PaymentResultPage(BasePage):
    path = routes.CHECKOUT_PAY
    name = "payment result"

    # ── Simulator (pending transaction) ───────────────────────────────────
    @property
    def simulate_success_button(self) -> Locator:
        return self.page.get_by_role("button", name="Thanh toán thành công")

    @property
    def simulate_failure_button(self) -> Locator:
        return self.page.get_by_role("button", name="Thanh toán thất bại")

    # ── Result states (rendered as <h2> inside the Result) ────────────────
    @property
    def success_result(self) -> Locator:
        return self.page.get_by_role("heading", name="Thanh toán thành công")

    @property
    def error_result(self) -> Locator:
        return self.page.get_by_role("heading", name="Thanh toán thất bại")

    @property
    def view_orders_link(self) -> Locator:
        return self.page.get_by_role("link", name="Xem đơn hàng")

    @property
    def continue_shopping_link(self) -> Locator:
        return self.page.get_by_role("link", name="Tiếp tục mua sắm")

    @property
    def retry_button(self) -> Locator:
        return self.page.get_by_role("button", name="Thử lại")

    @property
    def change_method_link(self) -> Locator:
        return self.page.get_by_role("link", name="Đổi phương thức")

    @property
    def cancelled_notice(self) -> Locator:
        return self.page.get_by_text("tồn kho đã được giải phóng", exact=False)

    @property
    def not_found_result(self) -> Locator:
        return self.page.get_by_text("Không tìm thấy giao dịch", exact=False)

    def is_displayed(self) -> bool:
        return "/checkout/pay/" in self.page.url
