"""Edit-listing page (`/seller/{listing_id}/edit`) — reuses the ListingForm."""

from __future__ import annotations

from playwright.sync_api import Locator, expect

from src.constants import routes, timeouts
from src.core.base_page import BasePage


class SellerEditListingPage(BasePage):
    path = routes.SELLER_EDIT  # "/seller/{listing_id}/edit"
    name = "seller edit listing"

    @property
    def title_input(self) -> Locator:
        return self.page.locator("#title")

    @property
    def price_input(self) -> Locator:
        return self.page.locator("#price")

    @property
    def submit_button(self) -> Locator:
        return self.page.get_by_role("button", name="Lưu thay đổi")

    @property
    def success_message(self) -> Locator:
        return self.page.get_by_text("thành công", exact=False)

    def is_displayed(self) -> bool:
        return self.title_input.is_visible()

    def update_title(self, new_title: str) -> None:
        self.title_input.fill(new_title)
        self.submit_button.click()
        expect(self.success_message).to_be_visible(timeout=timeouts.NAVIGATION)
