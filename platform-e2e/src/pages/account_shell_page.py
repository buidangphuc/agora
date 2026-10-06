"""Account settings shell (`AccountShell`): breadcrumb, `h1` and the account menu.

Wraps the five `/account/*` settings pages (addresses, security, verification,
referral, following). The menu is a `<nav aria-label="Menu tài khoản">` of real
links; the current page's link carries `aria-current="page"`. Standalone page
object (not in the PageFactory) so step files instantiate it with `world.page`.
Locators only; assertions live in the step definitions.
"""

from __future__ import annotations

from playwright.sync_api import Locator

from src.core.base_page import BasePage


class AccountShellPage(BasePage):
    path = "/account/security"
    name = "account shell"

    @property
    def menu(self) -> Locator:
        return self.page.get_by_role("navigation", name="Menu tài khoản")

    def menu_link(self, label: str) -> Locator:
        return self.menu.get_by_role("link", name=label, exact=True)

    @property
    def current_menu_item(self) -> Locator:
        return self.menu.locator('a[aria-current="page"]')

    @property
    def heading(self) -> Locator:
        return self.page.get_by_role("heading", level=1)

    def is_displayed(self) -> bool:
        return self.menu.is_visible()

    def open(self, route: str) -> None:
        self.page.goto(
            f"{self._settings.base_url.rstrip('/')}{route}", wait_until="domcontentloaded"
        )
