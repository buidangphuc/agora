from __future__ import annotations

import re
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from tests.e2e.support.world import World


@when("the admin opens the cockpit HUD")
def admin_opens_cockpit_hud(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/admin/cockpit")
    world.page.wait_for_load_state("networkidle")


@then("the telemetry summary cards display throughput and latency metrics")
def cockpit_displays_summary_cards(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/admin/cockpit.*"), timeout=timeouts.DEFAULT)
    expect(world.page.locator("body")).to_contain_text("Total Gateway Throughput", timeout=timeouts.DEFAULT)
    expect(world.page.locator("body")).to_contain_text("Average gRPC P95 Latency", timeout=timeouts.DEFAULT)
