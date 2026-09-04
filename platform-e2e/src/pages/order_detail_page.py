"""Order Detail & Tracking Page (`/account/orders/[id]`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class OrderDetailPage(BasePage):
    path = routes.ACCOUNT_ORDERS
    name = "order detail"

    @property
    def timeline_container(self) -> Locator:
        return self.page.locator("div.border-b").first

    @property
    def tracking_code(self) -> Locator:
        return self.page.get_by_text("Mã Vận Đơn:", exact=False)

    @property
    def rma_refund_button(self) -> Locator:
        return self.page.get_by_role("button", name="Hủy đơn / Yêu cầu hoàn tiền")

    @property
    def rma_reason_select(self) -> Locator:
        return self.page.locator("select").last

    @property
    def rma_confirm_button(self) -> Locator:
        return self.page.get_by_role("button", name="Xác nhận gửi yêu cầu")

    @property
    def rma_success_alert(self) -> Locator:
        return self.page.get_by_text("Đã hủy đơn và kích hoạt hoàn tiền tự động", exact=False)

    def is_displayed(self) -> bool:
        return "/account/orders" in self.page.url

    def open_rma_modal(self) -> None:
        self.rma_refund_button.click()

    def submit_rma_request(self, reason_value: str = "changed_mind") -> None:
        self.rma_reason_select.select_option(reason_value)
        self.rma_confirm_button.click()
