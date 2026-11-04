"""Admin voucher pages (`/admin/vouchers`, change promotion-admin-vouchers).

Locators come from the visible texts and control names the spec fixes ("Stop", "Edit", status
"stopped", the outage messages); no data-testid is assumed. Business assertions live in steps.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator

from src.constants import timeouts
from src.core.base_page import BasePage

FORBIDDEN_TITLE = "Không có quyền"
NEXT_LINK = re.compile(r"Trang sau|next", re.I)
MAX_PAGES = 25

DUPLICATE_MESSAGE = "Mã này đã được dùng cho voucher khác"
AUDIT_DOWN_CREATE = "Chưa tạo được voucher: hệ thống kiểm toán đang tạm ngưng"
AUDIT_DOWN_CHANGE = "Chưa thay đổi được voucher: hệ thống kiểm toán đang tạm ngưng"
PROMOTION_DOWN_CREATE = "Chưa tạo được voucher: dịch vụ khuyến mãi đang tạm ngưng"
PROMOTION_DOWN_CHANGE = "Chưa thay đổi được voucher: dịch vụ khuyến mãi đang tạm ngưng"

SPEC_MESSAGES = {
    "code already exists": DUPLICATE_MESSAGE,
    "audit is down, nothing created": AUDIT_DOWN_CREATE,
    "audit is down, nothing changed": AUDIT_DOWN_CHANGE,
    "promotion is down, nothing created": PROMOTION_DOWN_CREATE,
    "promotion is down, nothing changed": PROMOTION_DOWN_CHANGE,
    "promotion is down": "dịch vụ khuyến mãi đang tạm ngưng",
}

QUOTA_LABEL = "quota|lượt dùng tối đa"
START_LABEL = "start|bắt đầu"
END_LABEL = "end|kết thúc"

_STOP = re.compile(r"^(stop|ngừng voucher)$", re.I)
_EDIT = re.compile(r"^(edit|sửa voucher)$", re.I)
_CONFIRM = re.compile(r"^(confirm|stop|save|create|submit|yes|xác nhận|ngừng|lưu|tạo)", re.I)
_SUBMIT = re.compile(r"^(create|review|submit|save|xem lại|lưu)", re.I)


class AdminVouchersPage(BasePage):
    path = "/admin/vouchers"
    name = "admin vouchers"
    base_url_setting = "admin_base_url"

    def is_displayed(self) -> bool:
        return "/admin/vouchers" in self.page.url

    @property
    def table(self) -> Locator:
        return self.page.get_by_role("table")

    @property
    def forbidden(self) -> Locator:
        return self.page.get_by_text(FORBIDDEN_TITLE)

    @property
    def dialog(self) -> Locator:
        return self.page.get_by_role("dialog").filter(has_not_text="Cookie preferences")

    @property
    def stop_controls(self) -> Locator:
        return self.page.get_by_role("button", name=_STOP).or_(
            self.page.get_by_role("link", name=_STOP)
        )

    @property
    def edit_controls(self) -> Locator:
        return self.page.get_by_role("button", name=_EDIT).or_(
            self.page.get_by_role("link", name=_EDIT)
        )

    def text(self, message: str) -> Locator:
        return self.page.get_by_text(SPEC_MESSAGES.get(message, message), exact=False)

    def field(self, label: str) -> Locator:
        return self.page.get_by_label(re.compile(label, re.I))

    def rows_of(self, code: str) -> Locator:
        return self.page.locator("tbody tr").filter(has_text=code)

    def find_row(self, code: str) -> Locator | None:
        for _ in range(MAX_PAGES):
            self.page.wait_for_load_state("domcontentloaded")
            self.table.first.wait_for(state="visible", timeout=timeouts.NAVIGATION)
            rows = self.rows_of(code)
            if rows.count() > 0:
                return rows.first
            nxt = self.page.get_by_role("link", name=NEXT_LINK)
            if nxt.count() == 0:
                return None
            href = nxt.first.get_attribute("href")
            nxt.first.click()
            self.page.wait_for_url(lambda url, h=href: url.endswith(h or ""))
        return None

    def count_rows(self, code: str) -> int:
        total = 0
        for _ in range(MAX_PAGES):
            self.page.wait_for_load_state("domcontentloaded")
            self.table.first.wait_for(state="visible", timeout=timeouts.NAVIGATION)
            total += self.rows_of(code).count()
            nxt = self.page.get_by_role("link", name=NEXT_LINK)
            if nxt.count() == 0:
                break
            href = nxt.first.get_attribute("href")
            nxt.first.click()
            self.page.wait_for_url(lambda url, h=href: url.endswith(h or ""))
        return total

    def open_detail(self, code: str) -> None:
        row = self.find_row(code)
        assert row is not None, f"voucher {code} is not listed"
        row.get_by_role("link", name=re.compile(re.escape(code))).first.click()
        self.page.get_by_text(code).first.wait_for(state="visible", timeout=timeouts.NAVIGATION)

    def press(self, scope: Locator, name: re.Pattern[str]) -> None:
        button = scope.get_by_role("button", name=name).first
        self.wait_until_interactive(button)
        button.click()

    def fill_date(self, label: str, value_iso: str) -> None:
        box = self.field(label).first
        kind = (box.get_attribute("type") or "text").lower()
        if kind == "datetime-local":
            box.fill(value_iso[:16])
        elif kind == "date":
            box.fill(value_iso[:10])
        else:
            box.fill(value_iso[:10])

    def fill_create_form(self, code: str, quota: int, start_iso: str, end_iso: str) -> None:
        self.field(r"^(code|mã voucher)").first.fill(code)
        for label, value in (
            ("discount value|^value|phần trăm giảm", "10"),
            ("số tiền giảm", "10000"),
            ("min.?spend|đơn tối thiểu", "0"),
            ("max.?discount|giảm tối đa", "50000"),
        ):
            box = self.field(label)
            if box.count():
                box.first.fill(value)
        self.field(QUOTA_LABEL).first.fill(str(quota))
        self.fill_date(START_LABEL, start_iso)
        self.fill_date(END_LABEL, end_iso)

    def submit_form(self) -> None:
        self.press(self.page, _SUBMIT)
        dialog = self.dialog
        if dialog.count():
            ack = dialog.get_by_role("checkbox")
            if ack.count():
                ack.first.check()
            self.press(dialog, _CONFIRM)
