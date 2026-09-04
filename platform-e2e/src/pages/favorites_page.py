"""Favorites page (`/favorites`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class FavoritesPage(BasePage):
    path = routes.FAVORITES
    name = "favorites"

    @property
    def items(self) -> Locator:
        return self.page.locator('a[href^="/listing/"]')

    @property
    def heading(self) -> Locator:
        return self.page.get_by_text("Sản phẩm yêu thích", exact=False)

    def count(self) -> int:
        return self.items.count()

    def is_displayed(self) -> bool:
        return "/favorites" in self.page.url

    # ── Wishlist collections (team-engagement) ──────────────────────────
    @property
    def create_collection_form(self) -> Locator:
        return self.page.get_by_label("Tạo bộ sưu tập")

    @property
    def collection_name_input(self) -> Locator:
        return self.page.get_by_label("Tên bộ sưu tập", exact=True)

    @property
    def create_collection_submit(self) -> Locator:
        return self.create_collection_form.get_by_role("button", name="Tạo mới")

    @property
    def collection_rows(self) -> Locator:
        return self.page.get_by_test_id("collection-row")

    def collection_row(self, name: str) -> Locator:
        return self.page.locator(f'[data-testid="collection-row"][data-name="{name}"]')

    def create_collection(self, name: str) -> None:
        self.collection_name_input.fill(name)
        self.create_collection_submit.click()
