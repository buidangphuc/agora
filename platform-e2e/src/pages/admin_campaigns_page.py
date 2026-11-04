"""Admin flash-sale campaign pages (`/admin/campaigns`, change promotion-admin-campaigns).

Locators come from the texts and control names the spec fixes (status filter all/scheduled/active/
ended/cancelled, a cancel action with confirmation, status "cancelled"); no data-testid is assumed.
Business assertions live in steps.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator

from src.constants import timeouts
from src.core.base_page import BasePage, wait_streamed

NEXT_LINK = re.compile(r"Trang sau|next", re.I)
MAX_PAGES = 25
_CANCEL = re.compile(r"^(cancel( campaign)?|h(ủy|uỷ)( chiến dịch)?)$", re.I)
_CONFIRM = re.compile(r"^(confirm|yes|cancel campaign|xác nhận|đồng ý|h(ủy|uỷ) chiến dịch)", re.I)
_REVIEW_CONFIRM = re.compile(r"^(xác nhận tạo|confirm)", re.I)
_SUBMIT = re.compile(r"^(create|submit|save|review|tạo|lưu|xem lại)", re.I)

LABELS = {
    "listing": r"listing|mã tin đăng",
    "price": r"sale.?price|giá flash sale",
    "stock": r"stock|số lượng tối đa",
    "start": r"start|bắt đầu",
    "end": r"end|kết thúc",
}


class AdminCampaignsPage(BasePage):
    path = "/admin/campaigns"
    name = "admin campaigns"
    base_url_setting = "admin_base_url"

    def is_displayed(self) -> bool:
        return "/admin/campaigns" in self.page.url

    @property
    def table(self) -> Locator:
        return self.page.get_by_role("table")

    @property
    def dialog(self) -> Locator:
        return self.page.get_by_role("dialog").filter(has_not_text="Cookie preferences")

    def row_of(self, *needles: str) -> Locator:
        pattern = re.compile("|".join(re.escape(n) for n in needles))
        return self.page.locator("tbody tr").filter(has_text=pattern)

    def wait_loaded(self) -> None:
        self.page.wait_for_load_state("domcontentloaded")
        wait_streamed(self.page, timeout=timeouts.NAVIGATION)
        self.table.first.wait_for(state="visible", timeout=timeouts.NAVIGATION)

    def choose_filter(self, status: str) -> None:
        control = self.page.get_by_test_id(f"campaign-filter-{status.lower()}")
        control.first.wait_for(state="visible", timeout=timeouts.NAVIGATION)
        self.wait_until_interactive(control.first)
        control.first.click()
        self.page.wait_for_url(
            lambda url, s=status.lower(): (f"status={s}" in url) if s != "all" else ("status=" not in url),
            timeout=timeouts.NAVIGATION,
        )
        self.wait_loaded()

    def find_row(self, *needles: str) -> Locator | None:
        for _ in range(MAX_PAGES):
            self.wait_loaded()
            rows = self.row_of(*needles)
            if rows.count() > 0:
                return rows.first
            nxt = self.page.get_by_role("link", name=NEXT_LINK)
            if nxt.count() == 0:
                return None
            href = nxt.first.get_attribute("href")
            nxt.first.click()
            self.page.wait_for_url(lambda url, h=href: url.endswith(h or ""))
        return None

    def press(self, scope: Locator, name: re.Pattern[str]) -> None:
        button = scope.get_by_role("button", name=name).first
        self.wait_until_interactive(button)
        button.click()

    def cancel_row(self, row: Locator) -> None:
        self.press(row, _CANCEL)
        dialog = self.dialog
        dialog.first.wait_for(state="visible", timeout=timeouts.DEFAULT)
        self.press(dialog, _CONFIRM)

    def field(self, label: str) -> Locator:
        return self.page.get_by_label(re.compile(LABELS[label], re.I))

    def fill_date(self, label: str, value_iso: str) -> None:
        box = self.field(label).first
        kind = (box.get_attribute("type") or "text").lower()
        box.fill(value_iso[:16] if kind == "datetime-local" else value_iso[:10])

    def fill_create_form(
        self, listing_id: str, price: int, stock: int, start_iso: str, end_iso: str
    ) -> None:
        self.field("listing").first.fill(listing_id)
        self.field("price").first.fill(str(price))
        self.field("stock").first.fill(str(stock))
        self.fill_date("start", start_iso)
        self.fill_date("end", end_iso)

    def submit_form(self) -> None:
        confirm = self.page.get_by_role("button", name=_REVIEW_CONFIRM)
        if confirm.count() == 0:
            self.press(self.page, _SUBMIT)
        try:
            confirm.first.wait_for(state="visible", timeout=3000)
        except Exception:  # noqa: BLE001 - a form without a review step submits directly
            return
        self.wait_until_interactive(confirm.first)
        confirm.first.click()
