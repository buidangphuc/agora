"""Account privacy page (`/account/privacy`): consent card, export card and deletion card.

Standalone page object (not in the PageFactory) so step files instantiate it with `world.page`.
Locators only; assertions live in the step definitions.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator

from src.pages.account_shell_page import AccountShellPage


class PrivacyPage(AccountShellPage):
    path = "/account/privacy"
    name = "account privacy"

    def toggle(self, label: str) -> Locator:
        return self.page.get_by_label(label, exact=True)

    @property
    def save_button(self) -> Locator:
        return self.page.get_by_role("button", name=re.compile(r"^Lưu"))

    @property
    def history_rows(self) -> Locator:
        return self.page.locator(
            '[data-testid="consent-history"] [data-testid="consent-history-row"], '
            'section[aria-label*="Lịch sử"] tbody tr, section[aria-label*="Lịch sử"] li'
        )

    @property
    def page_two_link(self) -> Locator:
        return self.page.locator('a[href*="page=2"]')

    @property
    def export_button(self) -> Locator:
        return self.page.get_by_role("button", name="Tải dữ liệu của tôi")

    @property
    def delete_offer(self) -> Locator:
        return self.page.get_by_role("button", name="Xóa tài khoản", exact=True)

    @property
    def cancel_button(self) -> Locator:
        return self.page.get_by_role("button", name="Hủy yêu cầu xóa")

    @property
    def dialog(self) -> Locator:
        return self.page.get_by_role("dialog")

    @property
    def password_field(self) -> Locator:
        return self.dialog.get_by_label(re.compile(r"Mật khẩu"))

    @property
    def dialog_confirm(self) -> Locator:
        return self.dialog.get_by_role("button", name=re.compile(r"^(Xác nhận|Xóa tài khoản)"))
