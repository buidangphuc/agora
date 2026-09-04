"""Saved-searches panel on the search results page (`/search?q=...`).

The "Tìm kiếm đã lưu" panel (team-search via the gateway) lets a logged-in buyer
save the current query and re-run / delete saved ones. This is a standalone page
object (not registered in the PageFactory) so step files instantiate it directly
with `world.page`, mirroring the existing SearchPage locator style.
"""

from __future__ import annotations

from playwright.sync_api import Locator

from src.core.base_page import BasePage


class SavedSearchesPage(BasePage):
    #: Literal path (kept local so we never touch the shared routes module).
    path = "/search"
    name = "saved searches"

    def navigate_query(self, query: str) -> None:
        self.page.goto(f"{self.url()}?q={query}", wait_until="domcontentloaded")

    # ── Locators (selected by visible Vietnamese text / role) ─────────────
    @property
    def panel_header(self) -> Locator:
        return self.page.get_by_text("Tìm kiếm đã lưu", exact=True)

    @property
    def save_button(self) -> Locator:
        return self.page.get_by_role("button", name="Lưu tìm kiếm này")

    @property
    def empty_state(self) -> Locator:
        return self.page.get_by_text("Chưa có tìm kiếm nào được lưu", exact=False)

    def saved_item(self, query: str) -> Locator:
        """The saved-search entry rendered as a link whose text is the query."""
        return self.page.get_by_role("link", name=query, exact=True)

    def is_displayed(self) -> bool:
        return "/search" in self.page.url
