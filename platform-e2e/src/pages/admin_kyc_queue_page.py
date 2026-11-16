"""Admin KYC verification tab (`/admin/cases?tab=kyc`, change admin-kyc-queue).

Locators come from the visible Vietnamese texts the spec fixes; no data-testid is assumed.
Business assertions live in step definitions.
"""

from __future__ import annotations

from urllib.parse import urlencode

from playwright.sync_api import Locator

from src.core.base_page import BasePage

TAB_NAME = "Xác minh người bán"
NEXT_LINK = "Trang sau"
EMPTY_TEXT = "Không có hồ sơ nào đang chờ duyệt."
FORBIDDEN_TITLE = "Không có quyền"
OPEN_LINK = "Mở tài liệu"
APPROVE = "Duyệt"
REJECT = "Từ chối"


class AdminKycQueuePage(BasePage):
    path = "/admin/cases"
    name = "admin kyc queue"
    base_url_setting = "admin_base_url"

    def is_displayed(self) -> bool:
        return self.table.or_(self.page.get_by_text(EMPTY_TEXT)).first.is_visible()

    def kyc_url(self, **params: str) -> str:
        query = urlencode({"tab": "kyc", **params})
        return f"{self.app_base_url()}{self.path}?{query}"

    def open_kyc(self, **params: str) -> None:
        self.page.goto(self.kyc_url(**params), wait_until="domcontentloaded")

    @property
    def table(self) -> Locator:
        return self.page.get_by_role("table")

    @property
    def body_rows(self) -> Locator:
        return self.table.get_by_role("row").filter(has=self.page.get_by_role("cell"))

    def row(self, submission_id: str) -> Locator:
        return self.page.get_by_role("row").filter(has_text=submission_id)

    @property
    def tab_link(self) -> Locator:
        return self.page.get_by_role("link", name=TAB_NAME)

    @property
    def next_link(self) -> Locator:
        return self.page.get_by_role("link", name=NEXT_LINK)

    @property
    def forbidden(self) -> Locator:
        return self.page.get_by_text(FORBIDDEN_TITLE)

    def view_button(self, row: Locator, n: int) -> Locator:
        return row.get_by_role("button", name=f"Xem tài liệu {n}", exact=True)

    def view_buttons(self, row: Locator) -> Locator:
        return row.get_by_role("button", name="Xem tài liệu")

    def open_link(self, row: Locator) -> Locator:
        return row.get_by_role("link", name=OPEN_LINK)

    def approve_button(self, row: Locator) -> Locator:
        return row.get_by_role("button", name=APPROVE, exact=True)

    def reject_button(self, row: Locator) -> Locator:
        return row.get_by_role("button", name=REJECT, exact=True)

    @property
    def dialog(self) -> Locator:
        return self.page.get_by_role("dialog")

    def pick_reason(self, label: str) -> None:
        radio = self.dialog.get_by_role("radio", name=label)
        if radio.count():
            radio.first.check()
            return
        self.dialog.get_by_role("combobox").first.select_option(label=label)
