"""The World: per-scenario context object (mirrors bds `CustomWorld`).

Holds the Playwright page/context, the page & service factories, a logger, and
the typed `state` bag. Created by the `world` fixture in tests/conftest.py; steps
receive it as a parameter. Replaces Cucumber's World with a plain object carried
by a pytest fixture.
"""

from __future__ import annotations

import re
import time
from collections.abc import Callable

from playwright.sync_api import BrowserContext, Page

from config.settings import get_settings
from src.constants import PageName
from src.core import PageFactory, ServiceFactory
from src.core.base_page import BasePage
from src.utils import ScenarioLogger

from .state import ScenarioState

_GOOGLE_HOST = re.compile(
    r"^https?://([^/]*\.)?(googletagmanager\.com|google-analytics\.com|analytics\.google\.com"
    r"|doubleclick\.net)(:\d+)?(/|$)"
)


def _fulfil_google(route) -> None:  # noqa: ANN001
    if "/collect" in route.request.url or "/g/" in route.request.url:
        route.fulfill(status=204, body="")
    else:
        route.fulfill(
            status=200,
            content_type="application/javascript",
            body="window.dataLayer = window.dataLayer || [];",
        )


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
        self._cleanups: list[Callable[[], None]] = []

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
    def setup_web_context(self, *, consent: bool = True) -> None:
        """Pre-seed storage to keep first-load banners/popups out of the way.

        No-op friendly: extend with cookies/localStorage the app checks. Kept as a
        hook so scenarios have a stable, banner-free starting point.
        """
        cookies = [{"name": "e2e", "value": "1", "url": self.settings.base_url}]
        if (
            consent
        ):  # returning visitor who granted analytics: no banner, ecommerce dataLayer pushes on
            cookies.append(
                {
                    "name": "agora_consent",
                    "value": f"v=1&a=1&m=0&p=0&ts={int(time.time())}",
                    "url": self.settings.base_url,
                }
            )
        if consent:
            self.context.route(
                _GOOGLE_HOST, _fulfil_google
            )  # GTM is loaded now; never hit real Google
        try:
            self.context.add_cookies(cookies)
        except Exception:  # noqa: BLE001 - best-effort; never fail setup on this
            pass

    def add_cleanup(self, fn: Callable[[], None]) -> None:
        """Register teardown (runs LIFO in `cleanup`, even if the scenario failed)."""
        self._cleanups.append(fn)

    def cleanup(self) -> None:
        while self._cleanups:
            try:
                self._cleanups.pop()()
            except Exception as exc:  # noqa: BLE001 - keep tearing down the rest
                self.logger.info(f"cleanup failed: {exc}")
        self.service_factory.close()
