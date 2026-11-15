"""Cockpit insight panels (change admin-insights-panels): placements, retention, experiments, AI."""

from __future__ import annotations

import re
import time
from collections.abc import Callable

from playwright.sync_api import Locator, expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from tests.e2e.step_definitions.experiment_reports_steps import *  # noqa: F401,F403
from tests.e2e.support import experiment_reports_support as x
from tests.e2e.support import roe_support as roe
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

PANELS = (
    "insights_placements",
    "insights_retention",
    "insights_experiments",
    "insights_ai_surfaces",
)
PAGE_LOAD_MS = 90_000
POLL_S = 90.0
PERCENT = re.compile(r"^\d+\.\d%$")
INTERVAL = re.compile(r"^\d+\.\d% – \d+\.\d%$")


def _bag(world: World) -> dict:
    return world.state.extra.setdefault("aip", {})


def _open(world: World, query: str = "") -> None:
    world.page.goto(f"{world.settings.admin_base_url}/admin/cockpit{query}", timeout=PAGE_LOAD_MS)
    world.page.wait_for_load_state("networkidle")


def _panel(world: World, test_id: str) -> Locator:
    return world.page.get_by_test_id(test_id)


def _reload_until(world: World, ready: Callable[[], bool], what: str) -> None:
    deadline = time.monotonic() + POLL_S
    while time.monotonic() < deadline:
        if ready():
            return
        world.page.reload()
        world.page.wait_for_load_state("networkidle")
    raise AssertionError(
        f"{what}: never shown within {POLL_S:.0f}s; page text={world.page.inner_text('body')[:600]!r}"
    )


def _row(world: World, test_id: str, text: str) -> Locator:
    return _panel(world, test_id).locator("tr", has_text=text)


def _cells(row: Locator) -> list[str]:
    return [c.strip() for c in row.first.locator("td").all_inner_texts()]


@given("home_feed impressions served by two different model versions were tracked")
def seed_placements(world: World) -> None:
    bag = _bag(world)
    models = [roe.unique("model-a"), roe.unique("model-b")]
    bag["models"] = models
    beacons = []
    for model in models:
        beacons.append(
            roe.beacon(
                "impression",
                listing_id=roe.unique("listing"),
                anonymous_id=roe.unique("anon"),
                impression_id=roe.unique("imp"),
                placement_id="home_feed",
                model_version=model,
            )
        )
    roe.post(beacons)


@given("3 buyers signed up this week")
def seed_signups(world: World) -> None:
    for _ in range(3):
        token, _ = tii.register_buyer()
        x.post_beacons([x.beacon("sign_up")], token)
    _bag(world)["week"] = x.monday(x.now()).date().isoformat()


@given(
    "assistant events were tracked as 10 shown, 4 accepted, 1 edited and 2 dismissed under a fresh model version"
)
def seed_ai(world: World) -> None:
    bag = _bag(world)
    bag["ai_model"] = x.unique("mock:v1|hybrid-v2")
    plan = [("shown", 10), ("accepted", 4), ("edited", 1), ("dismissed", 2)]
    x.post_beacons([x.ai_event("assistant", a, bag["ai_model"]) for a, n in plan for _ in range(n)])


@when("the admin opens the insights cockpit")
def open_cockpit(world: World) -> None:
    _open(world)


@when("the admin opens the insights cockpit for that flag")
def open_for_flag(world: World) -> None:
    flag = world.state.extra["exr"]["flag"]
    _open(world, f"?flag={flag}")


@when("the admin opens the insights cockpit for a flag that was never exposed")
def open_for_unknown_flag(world: World) -> None:
    _open(world, f"?flag={x.unique('never').lower()}")


@when("the buyer opens the insights cockpit")
def buyer_opens(world: World) -> None:
    world.navigate_to(PageName.ADMIN_COCKPIT)


@then("the four insight panels are present on the page")
def four_panels(world: World) -> None:
    for test_id in PANELS:
        expect(_panel(world, test_id)).to_be_visible(timeout=timeouts.DEFAULT)


@then("the placements panel lists one row per model version with its stored value verbatim")
def placements_rows(world: World) -> None:
    models = _bag(world)["models"]
    panel = _panel(world, "insights_placements")
    _reload_until(
        world,
        lambda: all(_row(world, "insights_placements", m).count() >= 1 for m in models),
        "placement rows",
    )
    for model in models:
        rows = _row(world, "insights_placements", model)
        assert rows.count() == 1, (model, rows.count())
        assert "home_feed" in rows.first.inner_text(), rows.first.inner_text()
    expect(panel).not_to_contain_text("Tạm thời không có số liệu")


@then(
    "the retention panel shows a row for this week's cohort with a size of at least 3 and percentage cells"
)
def retention_row(world: World) -> None:
    week = _bag(world)["week"]
    _reload_until(world, lambda: _row(world, "insights_retention", week).count() >= 1, "cohort row")
    cells = _cells(_row(world, "insights_retention", week))
    assert week in cells, cells
    sizes = [int(c) for c in cells if c.isdigit()]
    assert sizes and max(sizes) >= 3, cells
    assert any(PERCENT.match(c) for c in cells), cells


@then(
    parsers.parse(
        'the experiments panel shows a control row with 2 exposed, 1 converted, "{rate}" and an interval'
    )
)
def experiment_row(world: World, rate: str) -> None:
    def converted_row() -> bool:
        row = _row(world, "insights_experiments", "control")
        return row.count() >= 1 and rate in _cells(row)

    _reload_until(world, converted_row, "variant row with the paid order converted")
    cells = _cells(_row(world, "insights_experiments", "control"))
    assert "2" in cells and "1" in cells, cells
    assert rate in cells, cells
    assert any(INTERVAL.match(c) for c in cells), cells
    expect(_panel(world, "insights_experiments")).to_contain_text("Cửa sổ chuyển đổi: 7 ngày")


@then(
    parsers.parse(
        'the AI surfaces panel shows that row with 10, 4, 1, 2, "{accepted}" and "{dismissed}"'
    )
)
def ai_row(world: World, accepted: str, dismissed: str) -> None:
    model = _bag(world)["ai_model"]
    _reload_until(world, lambda: _row(world, "insights_ai_surfaces", model).count() >= 1, "ai row")
    cells = _cells(_row(world, "insights_ai_surfaces", model))
    for want in ("assistant", model, "10", "4", "1", "2", accepted, dismissed):
        assert want in cells, (want, cells)


@then("the experiments panel says there are no exposures and shows no variant rows")
def no_exposures(world: World) -> None:
    panel = _panel(world, "insights_experiments")
    expect(panel).to_contain_text("Chưa có lượt hiển thị cho flag này", timeout=timeouts.DEFAULT)
    assert panel.locator("tbody tr").count() == 0


@then("the page shows the admin-required 403 result and none of the insight panels")
def forbidden_no_panels(world: World) -> None:
    cockpit = world.get_page(PageName.ADMIN_COCKPIT)
    expect(cockpit.forbidden_result).to_be_visible(timeout=timeouts.DEFAULT)  # type: ignore[attr-defined]
    for test_id in PANELS:
        expect(_panel(world, test_id)).to_have_count(0)
