"""KYC verification page (`/account/verification`).

The status is a `Tag` inside a `Descriptions` list; the submit form's controls
are labelled through `FormItem`, so they are selected by accessible name.
"""

from __future__ import annotations

from playwright.sync_api import Locator

from src.core.base_page import BasePage


class VerificationPage(BasePage):
    path = "/account/verification"
    name = "verification"

    @property
    def doc_type_select(self) -> Locator:
        return self.page.get_by_role("combobox", name="Loại giấy tờ")

    @property
    def doc_ref_input(self) -> Locator:
        return self.page.get_by_role("textbox", name="Mã tham chiếu tài liệu")

    @property
    def submit_button(self) -> Locator:
        return self.page.get_by_role("button", name="Gửi hồ sơ xác minh")

    def is_displayed(self) -> bool:
        return "/account/verification" in self.page.url
