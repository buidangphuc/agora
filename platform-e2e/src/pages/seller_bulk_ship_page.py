"""Bulk ship bar, modal and batch print page of the seller app (change seller-bulk-ship)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.core.base_page import BasePage

BULK_SHIP = "Gửi hàng loạt"
BULK_PRINT = "In phiếu hàng loạt"
PRINT_NOTICE = "Không in được"
RESULTS = "Kết quả gửi hàng loạt"
CANCELLED_REASON = "Trạng thái đơn không cho phép gửi (có thể đã huỷ hoặc đã gửi)"


class SellerBulkShipPage(BasePage):
    app = "seller"
    path = "/seller/orders"
    name = "seller bulk ship"

    def is_displayed(self) -> bool:
        return "/seller/orders" in self.page.url

    def row(self, order_id: str) -> Locator:
        return self.page.locator("tbody tr").filter(has=self.page.locator(f'a[href*="{order_id}"]'))

    def checkbox(self, order_id: str) -> Locator:
        return self.row(order_id).get_by_role("checkbox")

    @property
    def ship_button(self) -> Locator:
        return self.page.get_by_role("button", name=BULK_SHIP)

    @property
    def print_button(self) -> Locator:
        return self.page.get_by_role("button", name=BULK_PRINT).or_(
            self.page.get_by_role("link", name=BULK_PRINT)
        )

    @property
    def modal(self) -> Locator:
        return self.page.get_by_role("dialog")

    @property
    def tracking_inputs(self) -> Locator:
        return self.modal.get_by_role("textbox")

    @property
    def confirm_button(self) -> Locator:
        return self.modal.get_by_role("button", name="Xác nhận")

    @property
    def results(self) -> Locator:
        return self.page.get_by_role("region", name=RESULTS)

    @property
    def slips(self) -> Locator:
        return self.page.get_by_test_id("packing-slip")

    @property
    def print_notice(self) -> Locator:
        return self.page.get_by_text(PRINT_NOTICE)
