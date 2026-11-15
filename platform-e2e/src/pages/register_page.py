"""Registration page (`/register`). Fields `#username`, `#password`, `#role`."""

from __future__ import annotations

import re

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
    def terms_checkbox(self) -> Locator:
        return self.page.get_by_role("checkbox", name=re.compile("Điều khoản", re.I))

    @property
    def privacy_checkbox(self) -> Locator:
        return self.page.get_by_role("checkbox", name=re.compile("bảo mật", re.I))

    @property
    def submit_button(self) -> Locator:
        return self.page.get_by_role("button", name="Đăng ký")

    def is_displayed(self) -> bool:
        return self.username_input.is_visible()

    def register(
        self,
        username: str,
        password: str,
        role: str = "buyer",
        *,
        accept_terms: bool = True,
        accept_privacy: bool = True,
        submit: bool = True,
    ) -> None:
        # Wait for hydration: a fill or submit before it is lost or posts natively.
        self.wait_until_interactive(self.submit_button)
        self.username_input.fill(username)
        self.password_input.fill(password)
        self.role_select.select_option(role)
        self.terms_checkbox.set_checked(accept_terms)
        self.privacy_checkbox.set_checked(accept_privacy)
        if submit:
            self.submit_button.click()
