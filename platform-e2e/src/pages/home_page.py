"""Home / marketplace landing page (:3000 `/`)."""

from __future__ import annotations

from functools import cached_property

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage
from src.pages.components import HeaderComponent, RecommendationsRowComponent


class HomePage(BasePage):
    path = routes.HOME
    name = "home"

    @cached_property
    def header(self) -> HeaderComponent:
        return HeaderComponent(self.page)

    @cached_property
    def recommendations(self) -> RecommendationsRowComponent:
        """The "Gợi ý cho bạn" row (context HOMEPAGE, no seed)."""
        return RecommendationsRowComponent(self.page)

    @property
    def category_heading(self) -> Locator:
        return self.page.get_by_text("Danh Mục", exact=False)

    @property
    def cart_link(self) -> Locator:
        return self.page.locator('a[href="/cart"]')

    @property
    def listing_links(self) -> Locator:
        return self.page.locator('a[href^="/listing/"]')

    def is_displayed(self) -> bool:
        return self.header.search_input.is_visible()

    def search_for(self, term: str) -> None:
        self.header.search_for(term)
