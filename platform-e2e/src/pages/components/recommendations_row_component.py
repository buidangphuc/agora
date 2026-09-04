"""Recommendations row: the "Gợi ý cho bạn" section (surface-recommendations).

The row renders as a `<section>` whose header carries the brand label
"Gợi ý cho bạn" (present on both home and PDP), followed by a product grid of
listing cards. It appears on the home page (context HOMEPAGE, no seed) and the
product-detail page (context SIMILAR_ITEMS, seeded with the current listing id).

The frontend renders NOTHING when the list is empty (service UNAVAILABLE / no
results), so `heading` simply resolves to zero elements in the graceful-empty
case — the step asserts that, this object stays assertion-free.
"""

from __future__ import annotations

from playwright.sync_api import Locator, Page

from src.core.base_component import BaseComponent

HEADING_TEXT = "Gợi ý cho bạn"


class RecommendationsRowComponent(BaseComponent):
    """Wraps the "Gợi ý cho bạn" recommendations section on home / PDP."""

    def __init__(self, root: Page) -> None:
        super().__init__(root)

    @property
    def heading(self) -> Locator:
        """The brand label of the row (present iff the row rendered)."""
        return self.root.get_by_text(HEADING_TEXT, exact=True)

    @property
    def section(self) -> Locator:
        """The `<section>` wrapper enclosing the heading and the card grid."""
        return self.root.locator("section", has_text=HEADING_TEXT)

    @property
    def cards(self) -> Locator:
        """Product-card anchors within the recs section (links to /listing/<id>)."""
        return self.section.locator('a[href^="/listing/"]')

    def is_present(self) -> bool:
        return self.heading.count() > 0

    def card_count(self) -> int:
        return self.cards.count()
