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

    # ── Discovery rework: sort, active filters, pagination, states, mobile ──
    @property
    def active_filters(self) -> Locator:
        return self.page.get_by_test_id("active-filters")

    def remove_filter_link(self, label_prefix: str) -> Locator:
        """The "×" link of an active-filter tag, e.g. 'Giá' -> aria-label 'Bỏ lọc Giá: ...'."""
        return self.active_filters.get_by_role("link", name=re.compile(f"^Bỏ lọc {label_prefix}"))

    @property
    def clear_all_link(self) -> Locator:
        return self.page.get_by_role("link", name="Xóa tất cả bộ lọc")

    def sort_link(self, label: str) -> Locator:
        return self.page.get_by_role("link", name=label, exact=True)

    @property
    def pagination(self) -> Locator:
        """The numbered pager (desktop). The compact 375px pager is a second, hidden nav."""
        return self.page.get_by_role("navigation", name="Phân trang").first

    def page_link(self, number: int) -> Locator:
        return self.pagination.get_by_role("link", name=f"Trang {number}")

    @property
    def error_alert(self) -> Locator:
        return self.page.get_by_role("alert").filter(has_text="Không tải được kết quả tìm kiếm")

    @property
    def retry_link(self) -> Locator:
        return self.page.get_by_role("link", name="Thử lại")

    @property
    def clear_filters_button(self) -> Locator:
        """Zero-results action ("Xóa bộ lọc"), a link to /search."""
        return self.page.get_by_role("link", name="Xóa bộ lọc", exact=True)

    @property
    def filter_button(self) -> Locator:
        """Mobile trigger of the filter Drawer; the count Badge is part of its name."""
        return self.page.get_by_role("button", name=re.compile("^Bộ lọc"))

    @property
    def filter_drawer(self) -> Locator:
        return self.page.get_by_role("dialog", name="Bộ lọc")

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
