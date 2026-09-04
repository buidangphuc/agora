"""Listing card: a single product tile inside a result/grid (links to detail)."""

from __future__ import annotations

from playwright.sync_api import Locator, Page

from src.core.base_component import BaseComponent


class ListingCardComponent(BaseComponent):
    """Wraps the collection of product cards on a grid page.

    Cards render as anchors to `/listing/<id>`; the visible title is the card link.
    """

    def __init__(self, root: Page) -> None:
        super().__init__(root)

    @property
    def cards(self) -> Locator:
        return self.root.locator('a[href^="/listing/"]')

    def count(self) -> int:
        return self.cards.count()

    def open_first(self) -> None:
        self.cards.first.click()
