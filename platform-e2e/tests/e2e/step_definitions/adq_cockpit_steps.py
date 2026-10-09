"""Cockpit tracking quality panel (analytics-data-quality, area adq-e2e)."""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from tests.e2e.support import adq_support as adq
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

PANEL = "tracking-quality-panel"
ANALYTICS_SERVICE = "team-analytics"
PAGE_LOAD_MS = 90_000


def _x(world: World) -> dict:
    return world.state.extra


@given("views were tracked and the tracking quality report counts them")
def views_tracked(world: World) -> None:
    token = adq.admin_token(world)
    adq.post_views(3, listingId=f"e2e-adq-{tii.run_id()}")
    adq.poll_report(
        token, lambda r: adq.count(adq.type_row(r, "view"), "events") >= 1, window_hours=24
    )


@given("team-analytics is stopped")
def stop_analytics(world: World) -> None:
    token = adq.admin_token(world)
    adq.dc("stop", ANALYTICS_SERVICE)

    def restore_and_wait() -> None:
        adq.dc("up", "-d", "--no-deps", ANALYTICS_SERVICE)
        from tests.e2e.support import pear_edge_support as pe

        pe.wait_healthy(tii.analytics_container(), 120)
        adq.poll_report(token, lambda r: True, timeout_s=120)  # any 200 through the gateway

    world.add_cleanup(restore_and_wait)


@when("the admin opens the cockpit tracking quality panel")
def open_panel(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/admin/cockpit", timeout=PAGE_LOAD_MS)
    world.page.wait_for_load_state("networkidle")


@then('the "Tracking data quality" panel shows a status and a view count of at least 1')
def panel_shows_status_and_views(world: World) -> None:
    panel = world.page.get_by_test_id(PANEL)
    expect(panel).to_be_visible(timeout=timeouts.DEFAULT)
    expect(panel).to_contain_text("Tracking data quality")
    status = world.page.get_by_test_id("tracking-quality-status")
    expect(status).to_be_visible()
    assert status.inner_text().strip(), "the status badge is empty"
    row = world.page.get_by_test_id("tracking-quality-row-view")
    expect(row).to_be_visible()
    match = re.search(r"view\s+([\d.,]+)", row.inner_text(), re.IGNORECASE)
    assert match, f"no view count in row: {row.inner_text()!r}"
    assert int(re.sub(r"[.,]", "", match.group(1))) >= 1, row.inner_text()


@then('the "Tracking data quality" panel says the data is unavailable and shows no counts')
def panel_unavailable(world: World) -> None:
    panel = world.page.get_by_test_id(PANEL)
    expect(panel).to_be_visible(timeout=timeouts.DEFAULT)
    expect(panel).to_contain_text("Tracking data quality")
    expect(panel).to_contain_text(re.compile("unavailable", re.IGNORECASE))
    rows = world.page.locator('[data-testid^="tracking-quality-row-"]')
    assert rows.count() == 0, f"panel shows {rows.count()} count rows while analytics is down"
