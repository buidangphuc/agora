"""Seller "my listings" page (`/seller`). Gated: requires `listing.write` scope."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerListingsPage(BasePage):
    path = routes.SELLER
    name = "seller listings"

    def listing_by_title(self, title: str) -> Locator:
        return self.page.get_by_text(title, exact=False)

    def has_listing(self, title: str) -> bool:
        return self.listing_by_title(title).first.is_visible()

    def is_displayed(self) -> bool:
        return "/seller" in self.page.url
