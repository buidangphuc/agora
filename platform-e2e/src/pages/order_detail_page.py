"""Order Detail Page (`/account/orders/[id]`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class OrderDetailPage(BasePage):
    path = routes.ACCOUNT_ORDER_DETAIL
    name = "order detail"

    # ── Header ───────────────────────────────────────────────────────────
    @property
    def title(self) -> Locator:
        return self.page.get_by_role("heading", level=1)

    @property
    def status_badge(self) -> Locator:
        return self.page.get_by_test_id("order-status")

    @property
    def stepper(self) -> Locator:
        return self.page.get_by_role("navigation", name="Progress").first

    @property
    def cancelled_alert(self) -> Locator:
        return self.page.get_by_text("Đơn hàng đã hủy", exact=True)

    @property
    def reorder_button(self) -> Locator:
        return self.page.get_by_role("button", name="Mua lại", exact=True).first

    @property
    def cancel_button(self) -> Locator:
        return self.page.get_by_role("button", name="Hủy đơn", exact=True)

    @property
    def cancel_confirm_button(self) -> Locator:
        return self.page.get_by_test_id("cancel-confirm")

    # ── Timeline / tabs ──────────────────────────────────────────────────
    @property
    def timeline_container(self) -> Locator:
        return self.page.get_by_test_id("order-timeline")

    @property
    def timeline_failure(self) -> Locator:
        return self.page.get_by_test_id("timeline-failure")

    @property
    def tracking_code(self) -> Locator:
        return self.page.get_by_text("Mã vận đơn", exact=False)

    def tab(self, label: str) -> Locator:
        return self.page.get_by_role("tab", name=label)

    # ── Return (RMA) ─────────────────────────────────────────────────────
    @property
    def rma_refund_button(self) -> Locator:
        return self.page.get_by_role("button", name="Yêu cầu trả hàng", exact=True).first

    @property
    def rma_dialog(self) -> Locator:
        return self.page.get_by_role("dialog")

    @property
    def rma_reason_select(self) -> Locator:
        return self.page.get_by_test_id("return-reason")

    @property
    def rma_amount_input(self) -> Locator:
        return self.page.get_by_test_id("return-amount")

    @property
    def rma_confirm_button(self) -> Locator:
        return self.page.get_by_test_id("return-submit")

    @property
    def rma_status(self) -> Locator:
        return self.page.get_by_test_id("return-status")

    @property
    def rma_success_alert(self) -> Locator:
        return self.page.get_by_text("Đã gửi yêu cầu trả hàng / hoàn tiền", exact=False)

    # ── Exception pages ──────────────────────────────────────────────────
    @property
    def forbidden_result(self) -> Locator:
        return self.page.get_by_role("heading", name="Bạn không có quyền xem đơn hàng này")

    @property
    def back_to_orders_link(self) -> Locator:
        return self.page.get_by_role("link", name="Về đơn hàng của tôi")

    def is_displayed(self) -> bool:
        return "/account/orders/" in self.page.url

    def open_rma_modal(self) -> None:
        self.rma_refund_button.click()

    def submit_rma_request(self, reason_value: str = "changed_mind") -> None:
        if reason_value:
            self.rma_reason_select.select_option(reason_value)
        self.rma_confirm_button.click()
