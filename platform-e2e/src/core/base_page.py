"""Abstract page object base (mirrors bds `BasePage`).

Provides:
- `navigate()` using the configured BASE_URL.
- `find()` — a thin semantic-locator DSL (`role:`, `label:`, `text:`,
  `placeholder:`, `testid:`, `heading:`) that resolves to native Playwright
  locators. Native `get_by_*` remains the preferred path; the DSL exists so
  generic Gherkin steps can pass a string.
- safe waiting helpers (no fixed sleeps).

Concrete pages set `path` and implement `is_displayed()`. Business assertions
belong in step definitions, never here.
"""

from __future__ import annotations

from abc import ABC, abstractmethod

from playwright.sync_api import Locator, Page, expect

from config.settings import get_settings
from src.constants import timeouts


class BasePage(ABC):
    #: Route relative to BASE_URL (may contain `{}` placeholders — format before use).
    path: str = "/"
    name: str = "base"

    def __init__(self, page: Page) -> None:
        self.page = page
        self._settings = get_settings()

    # ── Navigation ───────────────────────────────────────────────────────
    def url(self, **params: str) -> str:
        path = self.path.format(**params) if params else self.path
        return f"{self._settings.base_url.rstrip('/')}{path}"

    def navigate(self, **params: str) -> None:
        self.page.goto(self.url(**params), wait_until="domcontentloaded")

    @abstractmethod
    def is_displayed(self) -> bool:
        """Return True when a hallmark element of this page is visible."""
        raise NotImplementedError

    # ── Semantic-locator DSL ─────────────────────────────────────────────
    def find(self, selector: str) -> Locator:
        """Resolve a prefixed selector string to a Playwright Locator.

        Examples: ``role:button=Đăng nhập``, ``label:Mật khẩu``, ``text:Giỏ hàng``,
        ``placeholder:Tìm kiếm``, ``testid:submit``, ``heading:Đăng nhập``.
        No prefix -> treated as a CSS selector (last resort).
        """
        prefix, _, rest = selector.partition(":")
        if not rest:
            return self.page.locator(selector)  # plain CSS

        if prefix == "role":
            role, sep, name = rest.partition("=")
            if sep:
                return self.page.get_by_role(role.strip(), name=name.strip())  # type: ignore[arg-type]
            return self.page.get_by_role(role.strip())  # type: ignore[arg-type]
        if prefix == "label":
            return self.page.get_by_label(rest)
        if prefix == "text":
            return self.page.get_by_text(rest)
        if prefix == "placeholder":
            return self.page.get_by_placeholder(rest)
        if prefix == "testid":
            return self.page.get_by_test_id(rest)
        if prefix == "heading":
            return self.page.get_by_role("heading", name=rest)
        return self.page.locator(selector)

    # ── Waiting helpers (never fixed sleeps) ─────────────────────────────
    def wait_visible(self, locator: Locator, timeout: int = timeouts.DEFAULT) -> None:
        expect(locator).to_be_visible(timeout=timeout)

    def wait_for_path(self, path_fragment: str, timeout: int = timeouts.NAVIGATION) -> None:
        self.page.wait_for_url(f"**{path_fragment}**", timeout=timeout)

    def click_and_wait_path(self, locator: Locator, path_fragment: str) -> None:
        with self.page.expect_navigation(url=f"**{path_fragment}**"):
            locator.click()
