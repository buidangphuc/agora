"""Loyalty daily check-in widget on the home page (`/`).

Rendered by team-frontend `LoyaltyWidget` for logged-in buyers only: a "Điểm danh"
section showing the streak + coin balance and a check-in button wired to the
gateway EngagementService.CheckIn RPC. Locators only — assertions live in steps.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class LoyaltyCheckinWidget(BasePage):
    path = routes.HOME
    name = "loyalty checkin widget"

    @property
    def widget(self) -> Locator:
        """The <section> that wraps the daily check-in widget."""
        return self.page.locator("section").filter(has_text="Điểm danh nhận xu").first

    @property
    def heading(self) -> Locator:
        return self.page.get_by_text("Điểm danh nhận xu", exact=False)

    @property
    def check_in_button(self) -> Locator:
        # Toggles to "Đang điểm danh..." while pending — the single check-in button.
        return self.widget.get_by_role("button", name="Điểm danh")

    @property
    def streak_value(self) -> Locator:
        """First <strong> in the widget renders the streak count."""
        return self.widget.locator("strong").first

    @property
    def earned_marker(self) -> Locator:
        """The "(+N)" coins-earned badge — only present after a successful check-in."""
        return self.widget.get_by_text(re.compile(r"\(\+\d+\)"))

    def is_displayed(self) -> bool:
        return self.heading.is_visible()

    def check_in(self) -> None:
        self.check_in_button.click()
