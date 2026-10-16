"""Admin users page (`/admin/users`, change frontend-user-enforcement).

Reads the data-testids the view exposes: `users-search`, `users-row`, `users-status-form`,
`users-flash`. Business assertions live in step definitions.
"""

from __future__ import annotations

from playwright.sync_api import Locator

from src.core.base_page import BasePage

FORBIDDEN_TITLE = "Cần tài khoản Quản trị"


class AdminUsersPage(BasePage):
    path = "/admin/users"
    name = "admin users"

    def is_displayed(self) -> bool:
        return self.page.get_by_role("heading", name="Quản lý người dùng").is_visible()

    @property
    def heading(self) -> Locator:
        return self.page.get_by_role("heading", name="Quản lý người dùng")

    @property
    def search_form(self) -> Locator:
        return self.page.get_by_test_id("users-search")

    @property
    def rows(self) -> Locator:
        return self.page.get_by_test_id("users-row")

    @property
    def flash(self) -> Locator:
        return self.page.get_by_test_id("users-flash")

    @property
    def table(self) -> Locator:
        return self.page.get_by_role("table")

    def row_of(self, username: str) -> Locator:
        return self.rows.filter(has_text=username)

    def status_form_of(self, username: str) -> Locator:
        return self.row_of(username).get_by_test_id("users-status-form")

    def search(self, query: str) -> None:
        form = self.search_form
        self.wait_until_interactive(form.get_by_role("button", name="Tìm"))
        form.get_by_label("Tên đăng nhập hoặc ID").fill(query)
        form.get_by_role("button", name="Tìm").click()

    def submit_status(
        self,
        username: str,
        status_label: str,
        reason_code: str,
        until_local: str | None = None,
    ) -> None:
        form = self.status_form_of(username)
        submit = form.get_by_role("button", name="Cập nhật trạng thái")
        self.wait_until_interactive(submit)
        form.get_by_label("Trạng thái mới").select_option(label=status_label)
        form.get_by_label("Mã lý do").select_option(value=reason_code)
        if until_local is not None:
            form.get_by_label("Tạm khóa đến").fill(until_local)
        submit.click()
