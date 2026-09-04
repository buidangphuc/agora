"""Account security page (`/account/security`, nav "Bảo Mật").

Shows the logged-in user's active login sessions and login history, each session
row carrying a "Thu hồi" (revoke) button. Anonymous visitors are redirected to
`/login` by the RSC page. Standalone page object (not in the PageFactory) so step
files instantiate it directly with `world.page`.
"""

from __future__ import annotations

from playwright.sync_api import Locator

from src.core.base_page import BasePage


class AccountSecurityPage(BasePage):
    #: Literal path (kept local so we never touch the shared routes module).
    path = "/account/security"
    name = "account security"

    # ── Navigation entry point ────────────────────────────────────────────
    @property
    def nav_link(self) -> Locator:
        """The "Bảo Mật" header link (visible only when authenticated)."""
        return self.page.get_by_role("link", name="Bảo Mật")

    # ── Section locators (selected by visible Vietnamese text / role) ─────
    @property
    def heading(self) -> Locator:
        return self.page.get_by_role("heading", name="Bảo mật tài khoản")

    @property
    def sessions_section(self) -> Locator:
        return self.page.get_by_role("heading", name="Phiên đăng nhập")

    @property
    def login_history_section(self) -> Locator:
        return self.page.get_by_role("heading", name="Lịch sử đăng nhập")

    @property
    def revoke_button(self) -> Locator:
        return self.page.get_by_role("button", name="Thu hồi").first

    def is_displayed(self) -> bool:
        return "/account/security" in self.page.url
