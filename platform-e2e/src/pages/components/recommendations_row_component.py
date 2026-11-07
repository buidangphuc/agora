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
CROSS_SELL_HEADING_TEXT = "Thường được mua cùng"


class RecommendationsRowComponent(BaseComponent):
    """Wraps the "Gợi ý cho bạn" recommendations section on home / PDP."""

    def __init__(self, root: Page, heading_text: str = HEADING_TEXT) -> None:
        super().__init__(root)
        self.heading_text = heading_text

    @property
    def heading(self) -> Locator:
        """The brand label of the row (present iff the row rendered)."""
        return self.root.get_by_text(self.heading_text, exact=True)

    @property
    def section(self) -> Locator:
        """The `<section>` wrapper enclosing the heading and the card grid."""
        return self.root.locator("section", has_text=self.heading_text)

    @property
    def cards(self) -> Locator:
        """Product-card anchors within the recs section (links to /listing/<id>)."""
        return self.section.locator('a[href^="/listing/"]')

    def request_id(self) -> str | None:
        """The `data-recs-request-id` of the section: the request_id the storefront received."""
        return self.section.first.get_attribute("data-recs-request-id")

    def is_present(self) -> bool:
        return self.heading.count() > 0

    def card_count(self) -> int:
        return self.cards.count()
