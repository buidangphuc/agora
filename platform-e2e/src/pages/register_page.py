"""Registration page (`/register`). Fields `#username`, `#password`, `#role`."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class RegisterPage(BasePage):
    path = routes.REGISTER
    name = "register"

    @property
    def username_input(self) -> Locator:
        return self.page.locator("#username")

    @property
    def password_input(self) -> Locator:
        return self.page.locator("#password")

    @property
    def role_select(self) -> Locator:
        return self.page.locator("#role")

    @property
    def submit_button(self) -> Locator:
        return self.page.get_by_role("button", name="Đăng ký")

    def is_displayed(self) -> bool:
        return self.username_input.is_visible()

    def register(self, username: str, password: str, role: str = "buyer") -> None:
        self.username_input.fill(username)
        self.password_input.fill(password)
        self.role_select.select_option(role)
        self.submit_button.click()
