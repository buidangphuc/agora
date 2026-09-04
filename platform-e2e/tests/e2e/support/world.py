"""The World: per-scenario context object (mirrors bds `CustomWorld`).

Holds the Playwright page/context, the page & service factories, a logger, and
the typed `state` bag. Created by the `world` fixture in tests/conftest.py; steps
receive it as a parameter. Replaces Cucumber's World with a plain object carried
by a pytest fixture.
"""

from __future__ import annotations

from playwright.sync_api import BrowserContext, Page

from config.settings import get_settings
from src.constants import PageName
from src.core import PageFactory, ServiceFactory
from src.core.base_page import BasePage
from src.utils import ScenarioLogger

from .state import ScenarioState


class World:
    def __init__(self, context: BrowserContext, page: Page, scenario_name: str) -> None:
        self.settings = get_settings()
        self.context = context
        self.page = page
        self.logger = ScenarioLogger(scenario_name)
        self.page_factory = PageFactory(page)
        self.service_factory = ServiceFactory()
        self.state = ScenarioState()
        self._current_page: BasePage | None = None

    # ── Page navigation / tracking ───────────────────────────────────────
    def navigate_to(self, name: PageName | str, **params: str) -> BasePage:
        page = self.page_factory.get(name)
        page.navigate(**params)
        self._current_page = page
        return page

    def get_page(self, name: PageName | str) -> BasePage:
        page = self.page_factory.get(name)
        self._current_page = page
        return page

    @property
    def current_page(self) -> BasePage:
        if self._current_page is None:
            raise RuntimeError("No current page set — navigate first.")
        return self._current_page

    def set_current_page(self, page: BasePage) -> None:
        self._current_page = page

    # ── Web context pre-setup (suppress consent popups, etc.) ─────────────
    def setup_web_context(self) -> None:
        """Pre-seed storage to keep first-load banners/popups out of the way.

        No-op friendly: extend with cookies/localStorage the app checks. Kept as a
        hook so scenarios have a stable, banner-free starting point.
        """
        try:
            self.context.add_cookies(
                [
                    {
                        "name": "e2e",
                        "value": "1",
                        "url": self.settings.base_url,
                    }
                ]
            )
        except Exception:  # noqa: BLE001 - best-effort; never fail setup on this
            pass

    def cleanup(self) -> None:
        self.service_factory.close()
