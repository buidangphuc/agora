"""AI and Observability Cockpit step definitions."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then

from src.constants import PageName, timeouts
from src.pages import CockpitPage
from tests.e2e.support.world import World


@then("I should see the observability telemetry HUD")
def verify_cockpit_hud(world: World) -> None:
    cockpit: CockpitPage = world.get_page(PageName.ADMIN_COCKPIT)  # type: ignore[assignment]
    expect(cockpit.metrics_hud_container).to_be_visible(timeout=timeouts.DEFAULT)
