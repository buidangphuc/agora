"""Login page (`/login`). Form fields `#username`, `#password`; submit "Đăng nhập"."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class LoginPage(BasePage):
    path = routes.LOGIN
    name = "login"

    @property
    def username_input(self) -> Locator:
        return self.page.locator("#username")

    @property
    def password_input(self) -> Locator:
        return self.page.locator("#password")

    @property
    def submit_button(self) -> Locator:
        return self.page.get_by_role("button", name="Đăng nhập")

    @property
    def error_message(self) -> Locator:
        # Server-action error text rendered in a red <p>.
        return self.page.get_by_text("không chính xác", exact=False)

    def is_displayed(self) -> bool:
        return self.username_input.is_visible()

    def login(self, username: str, password: str) -> None:
        self.username_input.fill(username)
        self.password_input.fill(password)
        self.submit_button.click()
