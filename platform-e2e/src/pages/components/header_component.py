"""Global header component: search bar present on every page (from app layout)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.core.base_component import BaseComponent


class HeaderComponent(BaseComponent):
    @property
    def search_input(self) -> Locator:
        return self.root.get_by_placeholder("Tìm kiếm sản phẩm", exact=False)

    def search_for(self, term: str) -> None:
        """Type a query and submit — navigates to /search?q=<term>."""
        box = self.search_input
        box.fill(term)
        box.press("Enter")
