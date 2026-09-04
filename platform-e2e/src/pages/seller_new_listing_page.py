"""Create-listing page (`/seller/new`). Gated: requires `listing.write` scope."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage
from src.models import Listing


class SellerNewListingPage(BasePage):
    path = routes.SELLER_NEW
    name = "seller new listing"

    @property
    def title_input(self) -> Locator:
        return self.page.locator("#title")

    @property
    def category_select(self) -> Locator:
        return self.page.locator("#categoryId")

    @property
    def price_input(self) -> Locator:
        return self.page.locator("#price")

    @property
    def stock_input(self) -> Locator:
        return self.page.locator("#stock")

    @property
    def description_input(self) -> Locator:
        return self.page.locator("#description")

    @property
    def submit_button(self) -> Locator:
        return self.page.get_by_role("button", name="Đăng bán ngay")

    def is_displayed(self) -> bool:
        return self.title_input.is_visible()

    @property
    def success_message(self) -> Locator:
        # Server-action confirmation: "✓ Đã đăng bán thành công ...".
        return self.page.get_by_text("thành công", exact=False)

    def fill_and_submit(self, listing: Listing) -> None:
        from playwright.sync_api import expect

        from src.constants import timeouts

        self.title_input.fill(listing.title)
        self.category_select.select_option(listing.category_id)
        self.price_input.fill(str(listing.price))
        self.stock_input.fill(str(listing.stock))
        if listing.description:
            self.description_input.fill(listing.description)
        self.submit_button.click()
        # Wait for the save to complete before callers navigate away (no fixed sleep).
        expect(self.success_message).to_be_visible(timeout=timeouts.NAVIGATION)
