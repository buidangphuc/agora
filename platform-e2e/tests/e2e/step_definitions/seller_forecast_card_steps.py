"""Steps for seller/seller_forecast_card.feature (seller-forecast-card).

Black box: keys `fc:v1:seller:<seller>:<listing>` are written to the stack Redis DB 3 in the layout the
nightly job documents (the job output itself is covered by forecast/nightly_forecast.feature); the
card is read in the browser and compared with what GetDemandForecast returns through the gateway.
"""

from __future__ import annotations

import math
import re
from datetime import datetime, timedelta, timezone

import pytest
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.pages.seller_analytics_page import SellerAnalyticsPage
from tests.e2e.flows.auth_flow import session_cookies
from tests.e2e.step_definitions import fsv_steps as fsv
from tests.e2e.support import oic_order_support as oic
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import rss_support as rss
from tests.e2e.support import tii_support as tii
from tests.e2e.support.apps import app_url
from tests.e2e.support.world import World

NO_DATA = "Chưa đủ dữ liệu để dự báo"
UNAVAILABLE = "Dự báo tạm thời không khả dụng"


@pytest.fixture
def fcc():
    c = fsv.Ctx()
    yield c
    if c.keys:
        r = fsv.job.Redis()
        try:
            r.delete(*c.keys)
        finally:
            r.close()


def _x(world: World) -> dict:
    return world.state.extra.setdefault("fcc", {})


def _page(world: World) -> SellerAnalyticsPage:
    return world.get_page(PageName.SELLER_ANALYTICS)  # type: ignore[return-value]


def _served(fcc: fsv.Ctx) -> dict:
    resp = fsv._call(fcc, fcc.seller, fcc.seller.user_id, fcc.listing)
    assert resp.status_code == 200, f"HTTP {resp.status_code}: {resp.text[:300]}"
    return resp.json()


@given("a seller with a paid order line and a stored forecast of 14 days for that listing")
def seller_with_forecast(fcc: fsv.Ctx) -> None:
    fsv.paid_line_no_key(fcc)
    key = fsv.seed_forecast_key(fcc.seller.user_id, fcc.listing)
    fcc.keys.add(key)
    fcc.points = fsv._stored_points()


@given("a seller with a paid order line for a listing that has no forecast key")
def seller_without_key(fcc: fsv.Ctx) -> None:
    fsv.paid_line_no_key(fcc)


@given("another seller exists")
def another_seller(fcc: fsv.Ctx) -> None:
    fcc.other = oic.register(fcc.w, "other", "seller")


@given("the seller is logged in the browser")
def seller_in_browser(world: World, fcc: fsv.Ctx) -> None:
    world.context.add_cookies(session_cookies(world.settings, fcc.seller.token))
    world.service_factory.set_token(fcc.seller.token)


@given("the forecast store is stopped")
def forecast_store_stopped(world: World) -> None:
    def restore() -> None:
        rss.compose("start", "redis")
        pe.wait_healthy(rss.REDIS_CONTAINER, 120)

    world.add_cleanup(restore)
    rss.compose("stop", "redis")


@when("the seller loads the analytics page")
def open_analytics(world: World) -> None:
    response = world.page.goto(app_url(world.settings, "/seller/analytics"))
    _x(world)["status"] = response.status if response else 0
    expect(_page(world).kpi_row).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the other seller calls GetDemandForecast for the first seller's listing through the gateway")
def other_calls(fcc: fsv.Ctx) -> None:
    fcc.resp = fsv._call(fcc, fcc.other, fcc.seller.user_id, fcc.listing)


@then("the analytics page answers 200")
def page_200(world: World) -> None:
    assert _x(world)["status"] == 200, _x(world)


@then("the analytics KPI row is still shown")
def kpi_shown(world: World) -> None:
    expect(_page(world).kpi_row).to_be_visible(timeout=timeouts.DEFAULT)


@then("the forecast card lists the listing with 14 rows of p10, p50 and p90 exactly as served")
def card_rows(world: World, fcc: fsv.Ctx) -> None:
    page = _page(world)
    expect(page.forecast_title).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.forecast_rows).to_have_count(14, timeout=timeouts.DEFAULT)
    served = _served(fcc)["dailyForecasts"]
    assert len(served) == 14, served
    for i, point in enumerate(served):
        cells = page.forecast_rows.nth(i).get_by_role("cell").all_inner_texts()
        nums = [float(c.replace(",", ".")) for c in cells[1:4]]
        assert nums == [float(point["p10"]), float(point["p50"]), float(point["p90"])], (i, cells, point)


@then("the forecast card shows the reorder point of the served forecast rounded up")
def card_reorder(world: World, fcc: fsv.Ctx) -> None:
    served = _served(fcc)
    expected = math.ceil(float(served["suggestedReorderPoint"]))
    text = _page(world).forecast_card.inner_text()
    assert f"Điểm đặt hàng lại: {expected} đơn vị" in text, (expected, text)


@then("the forecast card shows the served model version and the run date as dd/mm/yyyy")
def card_version_date(world: World, fcc: fsv.Ctx) -> None:
    served = _served(fcc)
    run = datetime.fromisoformat(served["generatedAt"].replace("Z", "+00:00"))
    local = run.astimezone(timezone(timedelta(hours=7))).strftime("%d/%m/%Y")
    text = _page(world).forecast_card.inner_text()
    assert served["modelVersion"] in text, (served["modelVersion"], text)
    assert local in text, (local, text)


@then(parsers.parse('the forecast card says "{message}"'))
def card_says(world: World, message: str) -> None:
    expect(_page(world).forecast_card).to_contain_text(message, timeout=timeouts.DEFAULT)


@then("the forecast card shows no forecast numbers, reorder point or model version")
def card_no_numbers(world: World, fcc: fsv.Ctx) -> None:
    page = _page(world)
    expect(page.forecast_card).to_be_visible(timeout=timeouts.DEFAULT)
    assert page.forecast_rows.count() == 0
    text = page.forecast_card.inner_text()
    assert "Điểm đặt hàng lại" not in text, text
    assert "seasonal_naive" not in text, text
    assert not re.search(r"\d{2}/\d{2}/\d{4}", text), text


@then(parsers.parse('the forecast call is denied with "{code}" and carries no forecast numbers'))
def denied(fcc: fsv.Ctx, code: str) -> None:
    assert oic.code_of(fcc.resp) == code, f"HTTP {fcc.resp.status_code}: {fcc.resp.text[:300]}"
    for name in ("dailyForecasts", "suggestedReorderPoint", "safetyStock", "modelVersion"):
        assert name not in fcc.resp.text, fcc.resp.text


@then(
    "the warehouse holds an ai_interaction event of that seller whose model_version is the one "
    "team-ai serves and whose impression_id is not empty"
)
def stored_provenance(world: World) -> None:
    seller = world.state.seeded_seller
    token = seller.token
    principal = _principal(token)
    w = oic.OicWorld()
    served = oic.post(
        w,
        oic.Actor(username=seller.username, token=token, role="seller", user_id=principal),
        "/platform.ai.v1.AIService",
        "MagicListing",
        {"titleHint": "Laptop Dell XPS 13 9340 Chính Hãng"},
    ).json()
    assert served.get("modelVersion"), served
    rows = tii.warehouse_rows(
        "SELECT model_version, impression_id FROM tracking_events "
        "WHERE event_type = 'ai_interaction' AND principal_id = ? AND impression_id <> ''",
        [principal],
        timeout_s=60.0,
    )
    assert rows, f"no ai_interaction event with an impression id for {seller.username}"
    assert all(r[0] == served["modelVersion"] for r in rows), (rows, served["modelVersion"])


def _principal(token: str) -> str:
    return rss.jwt_subject(token)
