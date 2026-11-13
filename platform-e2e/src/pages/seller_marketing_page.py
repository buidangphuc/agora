"""Seller marketing hub (`/seller/marketing`, change seller-marketing-hub).

Locators come from the visible texts the spec fixes (tab names, state labels, buttons, messages);
no data-testid is assumed. Business assertions live in steps.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator

from src.constants import timeouts
from src.core.base_page import BasePage

VOUCHER_TAB = "Voucher của shop"
FLASH_TAB = "Flash sale"
FORBIDDEN_TITLE = "Không có quyền"
VOUCHER_STATES = ("Đang chạy", "Sắp diễn ra", "Đã kết thúc", "Đã tắt")
CAMPAIGN_STATES = {
    "SCHEDULED": "Sắp diễn ra",
    "ACTIVE": "Đang diễn ra",
    "ENDED": "Đã kết thúc",
    "CANCELLED": "Đã huỷ",
}
DUPLICATE_CODE = "Mã voucher đã tồn tại"
STARTED_MEANWHILE = "Flash sale đã bắt đầu, không thể huỷ"
PAGE_SIZE = 20

_NEXT = re.compile(r"Trang sau|next|sau", re.I)
_SUBMIT_VOUCHER = re.compile(r"^(tạo voucher|lưu|tạo)", re.I)
_SUBMIT_CAMPAIGN = re.compile(r"^(tạo flash sale|lưu|tạo)", re.I)
_CONFIRM = re.compile(r"^(xác nhận|đồng ý|tắt voucher|huỷ flash sale|huỷ)$", re.I)


class SellerMarketingPage(BasePage):
    app = "seller"
    path = "/seller/marketing"
    name = "seller marketing hub"

    def is_displayed(self) -> bool:
        return "/seller/marketing" in self.page.url

    def tab(self, label: str) -> Locator:
        return self.page.get_by_role("tab", name=label).or_(
            self.page.get_by_role("link", name=label, exact=True)
        )

    def selected_tab(self, label: str) -> Locator:
        return (
            self.page.get_by_role("tab", name=label, selected=True)
            .or_(self.page.locator('a[aria-current="page"]').filter(has_text=label))
            .or_(self.page.locator('[aria-selected="true"]').filter(has_text=label))
        )

    @property
    def nav_link(self) -> Locator:
        return self.page.get_by_role("link", name="Marketing", exact=True).first

    @property
    def forbidden(self) -> Locator:
        return self.page.get_by_text(FORBIDDEN_TITLE).first

    def row(self, text: str) -> Locator:
        return self.page.get_by_role("row").filter(has_text=text)

    def rows_with(self, prefix: str) -> Locator:
        return self.page.get_by_role("row").filter(has_text=re.compile(re.escape(prefix)))

    @property
    def data_rows(self) -> Locator:
        return self.page.locator("tbody").get_by_role("row")

    @property
    def next_page(self) -> Locator:
        return self.page.get_by_role("link", name=_NEXT).or_(
            self.page.get_by_role("button", name=_NEXT)
        )

    def field(self, label: str) -> Locator:
        return self.page.get_by_label(re.compile(label, re.I)).first

    def text(self, message: str) -> Locator:
        return self.page.get_by_text(message, exact=False).first

    @property
    def dialog(self) -> Locator:
        return self.page.get_by_role("dialog").filter(has_not_text="Cookie preferences")

    def press(self, scope: Locator, name: str | re.Pattern[str]) -> None:
        button = scope.get_by_role("button", name=name).first
        self.wait_until_interactive(button)
        button.click()

    def confirm(self) -> None:
        self.dialog.first.wait_for(state="visible", timeout=timeouts.DEFAULT)
        self.press(self.dialog, _CONFIRM)

    def fill_voucher_form(self, code: str, quota: int, start: str, end: str) -> None:
        self.field(r"^(mã voucher|code)").fill(code)
        for label, value in (
            ("phần trăm|giá trị giảm|discount", "10"),
            ("tối thiểu|min", "0"),
            ("giảm tối đa|max", "50000"),
        ):
            box = self.page.get_by_label(re.compile(label, re.I))
            if box.count():
                box.first.fill(value)
        self.field(r"lượt dùng|số lượng|quota").fill(str(quota))
        self.field(r"bắt đầu|start").fill(start[:16])
        self.field(r"kết thúc|end").fill(end[:16])

    def submit_voucher(self) -> None:
        self.press(self.page, _SUBMIT_VOUCHER)

    def submit_campaign(self) -> None:
        self.press(self.page, _SUBMIT_CAMPAIGN)
