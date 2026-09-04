"""Admin cockpit live-metrics step definitions (replace-cockpit-mock-metrics).

Drives real traffic through the gateway, then asserts the shaped
`CockpitMetricsResponse` from `GET /api/admin/metrics` is Prometheus-sourced
(not the old math/rand stub) and that raw Prometheus is never exposed.
"""

from __future__ import annotations

from numbers import Number

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.api.services import GatewayError
from src.constants import PageName, timeouts
from tests.e2e.support.world import World

SEARCH_SERVICE = "search"
# The replaced stub produced roughly `124 + rand()` RPS; a live idle stack must not
# sit on that baseline.
LEGACY_STUB_RPS = 124.0


def _rps(row: dict) -> float:
    for key in ("rps", "requests_per_second", "requestsPerSecond", "total_rps"):
        val = row.get(key)
        if isinstance(val, Number):
            return float(val)
    return 0.0


# ── Traffic / reads ──────────────────────────────────────────────────────
@given("traffic has been driven through the gateway to the search service")
def drive_search_traffic(world: World) -> None:
    for _ in range(5):
        try:
            world.service_factory.search.search("laptop")
        except GatewayError:
            pass
    world.state.extra["drove_traffic"] = True


@when("the cockpit metrics are read twice with no traffic in between")
def read_metrics_twice(world: World) -> None:
    first = world.service_factory.metrics.get_cockpit_metrics()
    second = world.service_factory.metrics.get_cockpit_metrics()
    world.state.extra["metrics_first"] = first
    world.state.extra["metrics_second"] = second


@when("the cockpit endpoint is called")
def call_cockpit_endpoint(world: World) -> None:
    world.state.extra["metrics"] = world.service_factory.metrics.get_cockpit_metrics()


@when('an admin opens the "admin cockpit" page')
def admin_opens_cockpit(world: World) -> None:
    world.navigate_to(PageName.ADMIN_COCKPIT)
    world.state.extra["metrics"] = world.service_factory.metrics.get_cockpit_metrics()


# ── Assertions ───────────────────────────────────────────────────────────
@then("the observability HUD is visible")
def hud_visible(world: World) -> None:
    cockpit = world.get_page(PageName.ADMIN_COCKPIT)
    expect(cockpit.metrics_hud_container).to_be_visible(timeout=timeouts.DEFAULT)  # type: ignore[attr-defined]


@then("the cockpit metrics show a non-zero Prometheus-sourced RPS for the search service")
def search_rps_non_zero(world: World) -> None:
    metrics = world.state.extra["metrics"]
    row = world.service_factory.metrics.service_row(metrics, SEARCH_SERVICE)
    assert row, "no search service row in cockpit metrics"
    assert _rps(row) > 0.0, "search RPS should be non-zero after real traffic"


@then("the search service RPS is stable across the two reads rather than a random baseline")
def search_rps_stable(world: World) -> None:
    svc = world.service_factory.metrics
    a = _rps(svc.service_row(world.state.extra["metrics_first"], SEARCH_SERVICE))
    b = _rps(svc.service_row(world.state.extra["metrics_second"], SEARCH_SERVICE))
    # Prometheus-sourced idle values are stable between back-to-back reads; the old
    # `rand()` stub swung wildly each call.
    assert abs(a - b) <= 5.0, f"RPS jumped {a}->{b} between reads (looks random, not Prometheus)"


@then("no service reports the legacy random stub baseline near 124 RPS")
def no_legacy_baseline(world: World) -> None:
    metrics = world.state.extra["metrics_first"]
    for row in metrics.get("services", []) or []:
        assert not (
            LEGACY_STUB_RPS - 1 <= _rps(row) <= LEGACY_STUB_RPS + 50
        ), f"service {row.get('name')} sits on the legacy ~124+rand() baseline"


@then("only the shaped cockpit metrics response is returned")
def only_shaped_response(world: World) -> None:
    metrics = world.state.extra["metrics"]
    assert "services" in metrics, "response is missing the shaped services[]"
    assert isinstance(metrics.get("services"), list)
    # Never a raw Prometheus query result shape.
    assert (
        "resultType" not in metrics and "data" not in metrics
    ), "cockpit leaked a raw Prometheus response shape to the browser"


@then("the gateway does not expose a raw Prometheus query endpoint")
def no_raw_prometheus_endpoint(world: World) -> None:
    status = world.service_factory.metrics.raw_prometheus_query_status()
    assert status >= 400, f"gateway must not forward raw PromQL; /api/v1/query -> {status}"


@then("the response keeps the expected cockpit shape with numeric, non-random metric values")
def shape_and_numeric(world: World) -> None:
    metrics = world.state.extra["metrics"]
    for key in ("services", "total_rps", "avg_latency_ms"):
        assert key in metrics, f"cockpit response missing {key}"
    for row in metrics.get("services", []) or []:
        rps = _rps(row)
        assert (
            isinstance(rps, float) and rps >= 0.0
        ), "metric value must be numeric and non-negative"


@then("total_orders_24h and total_revenue_24h are present as derived, non-authoritative values")
def orders_revenue_derived(world: World) -> None:
    metrics = world.state.extra["metrics"]
    assert "total_orders_24h" in metrics, "total_orders_24h missing from response shape"
    assert "total_revenue_24h" in metrics, "total_revenue_24h missing from response shape"
    assert isinstance(metrics["total_orders_24h"], Number)
    assert isinstance(metrics["total_revenue_24h"], Number)
