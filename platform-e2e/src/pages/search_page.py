"""Search results page (`/search?q=...`)."""

from __future__ import annotations

import re
from functools import cached_property

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage
from src.pages.components import ListingCardComponent


class SearchPage(BasePage):
    path = routes.SEARCH
    name = "search"

    @cached_property
    def results(self) -> ListingCardComponent:
        return ListingCardComponent(self.page)

    @property
    def empty_state(self) -> Locator:
        return self.page.get_by_text("Không tìm thấy sản phẩm", exact=False)

    def navigate_query(self, query: str) -> None:
        self.page.goto(f"{self.url()}?q={query}", wait_until="domcontentloaded")

    def has_results(self) -> bool:
        return self.results.count() > 0

    def open_first_result(self) -> None:
        self.results.open_first()

    def is_displayed(self) -> bool:
        return "/search" in self.page.url

    # ── Facets (F2) ──────────────────────────────────────────────────────
    @property
    def facets_sidebar(self) -> Locator:
        return self.page.get_by_test_id("search-facets")

    @property
    def results_wrapper(self) -> Locator:
        return self.page.get_by_test_id("search-results")

    def facet_group(self, name: str) -> Locator:
        """A facet group by its testid suffix: categories | price_ranges | ratings | sellers."""
        return self.page.get_by_test_id(f"facet-{name}")

    @property
    def facet_buckets(self) -> Locator:
        return self.page.get_by_test_id("facet-bucket")

    def facet_bucket(self, key: str) -> Locator:
        """A single facet bucket button by its `data-key` (e.g. '100000-500000')."""
        return self.page.locator(f'[data-testid="facet-bucket"][data-key="{key}"]')

    @staticmethod
    def bucket_count(bucket: Locator) -> int:
        """Parse the `(N)` count rendered inside a facet bucket button."""
        text = bucket.inner_text()
        m = re.search(r"\((\d+)\)", text)
        return int(m.group(1)) if m else -1

    _DISTINCT_RESULTS_JS = (
        'Array.from(document.querySelectorAll(\'[data-testid="search-results"] '
        "a[href^=\"/listing/\"]')).reduce((s, e) => s.add(e.getAttribute('href')), new Set()).size"
    )

    def result_count(self) -> int:
        """Distinct listings rendered inside the search results grid.

        Each card renders more than one anchor to the same `/listing/<id>`, so
        count *distinct* hrefs rather than raw anchors.
        """
        return int(self.page.evaluate(self._DISTINCT_RESULTS_JS))

    def wait_for_result_count(self, expected: int, timeout: float | None = None) -> None:
        """Block until the grid has re-rendered to `expected` distinct results.

        A facet click is a client-side (RSC) navigation, so the old grid stays in
        the DOM briefly after the request settles; poll the distinct count instead.
        """
        self.page.wait_for_function(
            f"n => ({self._DISTINCT_RESULTS_JS}) === n",
            arg=expected,
            timeout=timeout,
        )
