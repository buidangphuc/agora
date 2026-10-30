"""Admin moderation pages (`/admin/listings`, `/admin/cases`, change frontend-listing-moderation).

Locators come from the visible Vietnamese texts the spec fixes (button names, dialog texts,
tab names); no data-testid is assumed. Business assertions live in step definitions.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator, Page

from src.constants import timeouts
from src.core.base_page import BasePage

FORBIDDEN_TITLE = "Không có quyền"
EMPTY_QUEUE = "Hàng chờ trống"
EMPTY_REPORTS = "Không có báo cáo nào"
NEXT_LINK = "Trang sau"
MAX_PAGES = 25

_CONFIRM = re.compile(r"^(Xác nhận|Từ chối|Khóa|Duyệt|Đóng báo cáo|Gửi báo cáo)$")


class AdminModerationPage(BasePage):
    path = "/admin/listings"
    name = "admin moderation"
    base_url_setting = "admin_base_url"

    def is_displayed(self) -> bool:
        return self.table.or_(self.page.get_by_text(EMPTY_QUEUE)).first.is_visible()

    @property
    def table(self) -> Locator:
        return self.page.get_by_role("table")

    @property
    def forbidden(self) -> Locator:
        return self.page.get_by_text(FORBIDDEN_TITLE)

    @property
    def content_or_empty(self) -> Locator:
        """A rendered queue / report list: the table or one of the two empty texts."""
        return (
            self.table.or_(self.page.get_by_text(EMPTY_QUEUE)).or_(
                self.page.get_by_text(EMPTY_REPORTS)
            )
        ).first

    @property
    def dialog(self) -> Locator:
        # Exclude the cookie-consent banner, which is also role=dialog.
        return self.page.get_by_role("dialog").filter(has_not_text="Cookie preferences")

    def rows_of(self, text: str) -> Locator:
        return self.page.locator("tbody tr").filter(has_text=text)

    def find_row(self, text: str) -> Locator | None:
        """The first table row containing `text`, following "Trang sau" links; None when absent."""
        for _ in range(MAX_PAGES):
            self.page.wait_for_load_state("domcontentloaded")
            self.content_or_empty.wait_for(state="visible", timeout=timeouts.NAVIGATION)
            rows = self.rows_of(text)
            if rows.count() > 0:
                return rows.first
            nxt = self.page.get_by_role("link", name=NEXT_LINK)
            if nxt.count() == 0:
                return None
            # Read the target before clicking: from page 2 on the URL already carries
            # `cursor=`, so only the exact next href proves the client navigation happened.
            href = nxt.first.get_attribute("href")
            nxt.first.click()
            self.page.wait_for_url(lambda url, h=href: url.endswith(h or ""))
        return None

    def press(self, scope: Locator, name: str) -> None:
        button = scope.get_by_role("button", name=name, exact=True).first
        self.wait_until_interactive(button)
        button.click()

    def choose(self, scope: Locator, field: str, label: str) -> None:
        """Pick `label` in the control named `field` (native select, AntD select or radio group)."""
        box = scope.get_by_role("combobox", name=re.compile(field)).first
        if box.count():
            if box.evaluate("e => e.tagName") == "SELECT":
                box.select_option(label=label)
            else:
                box.click()
                self.page.get_by_role("option", name=label, exact=True).first.click()
            return
        scope.get_by_text(label, exact=True).first.click()

    def fill_note(self, scope: Locator, note: str) -> None:
        scope.locator("textarea").first.fill(note)

    def confirm(self, scope: Locator) -> None:
        button = scope.get_by_role("button", name=_CONFIRM).last
        self.wait_until_interactive(button)
        button.click()


def page_text(page: Page) -> str:
    return page.locator("body").inner_text()
