"""Shop public profile page (`/shop/[id]`) — follow / unfollow affordances.

Wraps the team-frontend `FollowSellerButton`, a single toggle whose label is
"+ Theo dõi" when not following and "✓ Đang theo dõi" once followed. Locators
only — assertions live in the step definitions.
"""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class ShopFollowPage(BasePage):
    path = routes.SHOP  # "/shop/{shop_id}"
    name = "shop follow"

    @property
    def follow_toggle(self) -> Locator:
        """The follow/unfollow button (matches both states — one element)."""
        return self.page.get_by_role("button", name="Theo dõi")

    @property
    def following_state(self) -> Locator:
        """Matches the toggle only while it reads "✓ Đang theo dõi"."""
        return self.page.get_by_role("button", name="Đang theo dõi")

    def is_displayed(self) -> bool:
        return "/shop/" in self.page.url

    def open(self, shop_id: str) -> None:
        self.navigate(shop_id=shop_id)

    def toggle_follow(self) -> None:
        self.follow_toggle.click()
