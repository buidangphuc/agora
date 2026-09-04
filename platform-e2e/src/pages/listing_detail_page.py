"""Product detail page (`/listing/{id}`)."""

from __future__ import annotations

from functools import cached_property

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage
from src.pages.components import RecommendationsRowComponent


class ListingDetailPage(BasePage):
    path = routes.LISTING_DETAIL  # "/listing/{listing_id}"
    name = "listing detail"

    @property
    def title(self) -> Locator:
        return self.page.locator("h1").first

    @cached_property
    def recommendations(self) -> RecommendationsRowComponent:
        """The "Gợi ý cho bạn" row (context SIMILAR_ITEMS, seeded with this listing)."""
        return RecommendationsRowComponent(self.page)

    @property
    def add_to_cart_button(self) -> Locator:
        return self.page.get_by_role("button", name="Thêm Vào Giỏ Hàng")

    # ── Flash-sale slot (team-promotion live-stock meter on the PDP) ──────
    @property
    def flash_sale_meter(self) -> Locator:
        return self.page.get_by_test_id("flash-sale-stock")

    @property
    def flash_sale_banner(self) -> Locator:
        # The whole flash-sale card; carries the "FLASH SALE" label + sale price.
        return self.page.locator("div").filter(has_text="FLASH SALE").first

    def flash_sale_remaining(self) -> int:
        return int(self.flash_sale_meter.get_attribute("data-remaining") or "-1")

    def flash_sale_stock_cap(self) -> int:
        return int(self.flash_sale_meter.get_attribute("data-stock-cap") or "-1")

    @property
    def buy_now_button(self) -> Locator:
        return self.page.get_by_role("button", name="Mua Ngay")

    # ── Wishlist collections slot (team-engagement) ──────────────────────
    @property
    def add_to_collection_button(self) -> Locator:
        """The "Thêm vào bộ sưu tập" control that opens the collection popover."""
        return self.page.get_by_test_id("add-to-collection")

    @property
    def collection_new_name_input(self) -> Locator:
        """Inline "create a new collection" input inside the popover."""
        return self.page.get_by_label("Tên bộ sưu tập mới")

    def collection_option(self, name: str) -> Locator:
        """A collection row inside the add-to-collection popover, matched by label."""
        return self.page.get_by_role("button", name=name, exact=False)

    # ── Price-drop / back-in-stock alert toggles (team-notification) ─────
    def alert_toggle(self, alert_type: str) -> Locator:
        """A "Báo tôi khi …" toggle by data-type ("price_drop" | "back_in_stock")."""
        return self.page.locator(f'[data-testid="alert-toggle"][data-type="{alert_type}"]')

    def alert_toggle_active(self, alert_type: str) -> bool:
        return self.alert_toggle(alert_type).get_attribute("data-active") == "true"

    # ── Rich reviews slot (team-engagement) ─────────────────────────────
    @property
    def review_items(self) -> Locator:
        return self.page.get_by_test_id("review-item")

    @property
    def review_helpful_buttons(self) -> Locator:
        return self.page.get_by_test_id("review-helpful")

    @property
    def verified_purchase_badges(self) -> Locator:
        return self.page.get_by_test_id("verified-purchase")

    @property
    def review_photos(self) -> Locator:
        """Review images rendered inside the review items."""
        return self.page.get_by_test_id("review-item").locator("img")

    @property
    def shop_rating_summary(self) -> Locator:
        return self.page.get_by_test_id("shop-rating-summary")

    def is_displayed(self) -> bool:
        return "/listing/" in self.page.url and self.add_to_cart_button.is_visible()

    def add_to_cart(self) -> None:
        self.add_to_cart_button.click()
        try:
            self.page.wait_for_selector("text=Đã thêm", timeout=3000)
        except Exception:  # noqa: BLE001
            pass
