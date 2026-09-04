"""Admin Observability Cockpit Page (`/admin/cockpit`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class CockpitPage(BasePage):
    path = routes.ADMIN_COCKPIT
    name = "admin cockpit"

    @property
    def metrics_hud_container(self) -> Locator:
        return self.page.locator("div.grid").first

    @property
    def service_status_badges(self) -> Locator:
        return self.page.get_by_text("SERVING", exact=False)

    @property
    def red_metrics_radar(self) -> Locator:
        # Live RED-metrics HUD heading (present once the cockpit renders metrics).
        return self.page.get_by_text("RED Metrics Radar", exact=False)

    def service_rps_cell(self, service: str) -> Locator:
        # Row/tile for a given service in the HUD grid (used to read its live RPS).
        return self.page.get_by_text(service, exact=False).first

    def is_displayed(self) -> bool:
        return "/admin/cockpit" in self.page.url
