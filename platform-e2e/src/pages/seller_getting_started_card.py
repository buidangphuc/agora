"""Getting-started card on the seller dashboard (`/seller`, change seller-getting-started)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.core.base_page import BasePage

CARD_TITLE = "Bắt đầu bán hàng"
ONBOARD_BUTTON = "Đồng ý và trở thành Người Bán"
STEPS = (
    ("Đặt tên gian hàng", "/seller/shop"),
    ("Thêm ảnh bìa gian hàng", "/seller/shop"),
    ("Đăng sản phẩm đầu tiên", "/seller/new"),
    ("Tạo voucher hoặc flash sale đầu tiên", "/seller/marketing"),
)
DONE_LABEL = "Đã xong"
NOT_DONE_LABEL = "Chưa xong"


class SellerGettingStartedPage(BasePage):
    app = "seller"
    path = "/seller"
    name = "seller getting started"

    @property
    def title(self) -> Locator:
        return self.page.get_by_text(CARD_TITLE, exact=True)

    @property
    def card(self) -> Locator:
        return self.page.locator("section, div, aside").filter(has=self.title).last

    @property
    def kpi_heading(self) -> Locator:
        return self.page.get_by_text("Tổng quan gian hàng").first

    @property
    def onboarding_button(self) -> Locator:
        return self.page.get_by_role("button", name=ONBOARD_BUTTON)

    def step(self, label: str) -> Locator:
        return self.page.get_by_role("listitem").filter(has_text=label)

    def is_displayed(self) -> bool:
        return self.title.is_visible()
