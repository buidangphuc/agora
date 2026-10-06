"""Admin cockpit step definitions (replace-cockpit-mock-metrics, ops-cockpit-real-data).

Drives real traffic through the gateway, then asserts the shaped
`CockpitMetricsResponse` from `GET /api/admin/metrics` is Prometheus-sourced
(not the old math/rand stub), that raw Prometheus is never exposed, that only the
seeded admin can read it (401 / 403), and that orders/GMV come from real paid orders.
Login steps ("an admin is logged in", "no one is logged in") live in common_steps.
"""

from __future__ import annotations

import time
import uuid
from numbers import Number

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import GatewayError
from src.constants import PageName, timeouts
from src.models import Listing
from tests.e2e.flows import create_order_via_api, seed_listing
from tests.e2e.support.world import World

SEARCH_SERVICE = "search"
# The replaced stub produced roughly `124 + rand()` RPS; a live idle stack must not
# sit on that baseline.
LEGACY_STUB_RPS = 124.0
# Figures the first cockpit fabricated; they must never reappear.
LEGACY_FABRICATED_ORDERS = 1420
LEGACY_FABRICATED_REVENUE = 384_500_000
# The gateway pushes its RED metrics to the OTel collector on the SDK's default 60 s
# periodic-reader interval and Prometheus scrapes the collector every 5 s, so traffic
# only becomes a non-zero rate once the next export lands (up to ~65 s later). Any
# earlier traffic inside the 1 m rate window hides this, which is why the scenario
# only fails when the stack is quiet (e.g. at the start of a parallel run).
METRICS_PIPELINE_LAG_S = 90.0
# The paid-order scenario allows the consumer batch interval plus ingestion lag.
ORDER_FACTS_LAG_S = 30.0
PAID_STATES = ("ORDER_STATUS_PAID", "PAID", "ORDER_STATUS_CONFIRMED", "CONFIRMED")


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
    svc = world.service_factory.metrics
    deadline = time.monotonic() + METRICS_PIPELINE_LAG_S
    metrics = world.state.extra["metrics"]
    while True:
        row = svc.service_row(metrics, SEARCH_SERVICE)
        assert row, "no search service row in cockpit metrics"
        if _rps(row) > 0.0:
            return
        if time.monotonic() >= deadline:
            break
        time.sleep(3.0)
        metrics = svc.get_cockpit_metrics()
    raise AssertionError(
        f"search RPS stayed 0 for {METRICS_PIPELINE_LAG_S:.0f}s after real traffic: {row}"
    )


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


@then(
    "total_orders_24h and total_revenue_24h are real figures or null and no fabricated rows are returned"
)
def orders_revenue_not_fabricated(world: World) -> None:
    metrics = world.state.extra["metrics"]
    # Orders/GMV come from team-analytics order_facts: a non-negative integer, or null
    # when analytics is unavailable — never an invented constant such as 1420 / 384500000.
    for key in ("total_orders_24h", "total_revenue_24h"):
        assert key in metrics, f"{key} missing from response shape"
        val = metrics[key]
        assert val is None or (
            isinstance(val, int) and val >= 0
        ), f"{key} must be null or a non-negative integer, got {val!r}"
    assert metrics["total_orders_24h"] != LEGACY_FABRICATED_ORDERS
    assert metrics["total_revenue_24h"] != LEGACY_FABRICATED_REVENUE
    for order in metrics.get("recent_orders") or []:
        assert order.get("order_id"), f"recent order without an id: {order}"
        assert not {"buyer", "buyer_id", "address"} & set(order), f"buyer PII leaked: {order}"
    for trace in metrics.get("recent_traces") or []:
        assert trace.get("trace_id") and trace.get("jaeger_url"), f"incomplete trace: {trace}"


# ── Admin gate (ops-cockpit-real-data) ───────────────────────────────────
@when("GET /api/admin/metrics is called")
def call_cockpit_raw(world: World) -> None:
    world.state.extra["cockpit_status"] = (
        world.service_factory.metrics.cockpit_response().status_code
    )


@then(parsers.parse("the gateway answers {code:d}"))
def gateway_answers(world: World, code: int) -> None:
    got = world.state.extra["cockpit_status"]
    assert got == code, f"GET /api/admin/metrics -> {got}, expected {code}"


@when('the buyer opens the "admin cockpit" page')
def buyer_opens_cockpit(world: World) -> None:
    world.navigate_to(PageName.ADMIN_COCKPIT)


@then("the page shows the admin-required 403 result and no metrics")
def page_shows_403(world: World) -> None:
    cockpit = world.get_page(PageName.ADMIN_COCKPIT)
    expect(cockpit.forbidden_result).to_be_visible(timeout=timeouts.DEFAULT)  # type: ignore[attr-defined]
    expect(world.page.get_by_text("Total Gateway Throughput")).to_have_count(0)


# ── Paid order shows up in the 24h figures ───────────────────────────────
def _read_cockpit(world: World) -> dict:
    world.service_factory.set_token(world.state.extra["admin_token"])
    return world.service_factory.metrics.get_cockpit_metrics()


@given("the 24h order figures are noted")
def note_24h_figures(world: World) -> None:
    assert world.state.current_user and world.state.current_user.token, "admin must be logged in"
    world.state.extra["admin_token"] = world.state.current_user.token
    before = _read_cockpit(world)
    assert (
        before.get("total_orders_24h") is not None
    ), "team-analytics must be reachable: total_orders_24h is null"
    world.state.extra["orders_before"] = before["total_orders_24h"]
    world.state.extra["revenue_before"] = before["total_revenue_24h"]


@when(parsers.parse("a buyer places and pays an order of {qty:d} x {price:d} VND"))
def buyer_places_and_pays(world: World, qty: int, price: int) -> None:
    seller = world.state.seeded_seller
    buyer = world.state.extra["seeded_buyer"]
    assert seller and buyer, "scenario must be tagged @needsSeller @needsBuyer"
    listing = Listing(
        title=f"[E2E] Cockpit order {uuid.uuid4().hex[:8]}", price=price, stock=qty + 5
    )
    seed_listing(world, listing, seller)
    create_order_via_api(world, buyer, listing.listing_id, quantity=qty)
    order_id = world.state.order_id
    world.service_factory.set_token(buyer.token)
    world.service_factory.payment.mock_pay(order_id, qty * price, success=True)
    # PAID lands when team-order consumes PaymentSettled; OrderPaidEvent follows.
    deadline = time.monotonic() + 20.0
    status = ""
    while time.monotonic() < deadline:
        status = world.service_factory.order.get_order(order_id).get("order", {}).get("status", "")
        if status in PAID_STATES:
            break
        time.sleep(0.5)
    assert status in PAID_STATES, f"order {order_id} is {status!r}, expected PAID"
    world.state.extra["paid_order_id"] = order_id


@then(parsers.parse("within 30 seconds total_orders_24h has increased by at least {n:d}"))
def orders_increased(world: World, n: int) -> None:
    # Other scenarios pay orders concurrently (xdist workers share the stack), so the
    # figures are a floor: this order must be counted, never exactly "+n". The poll
    # waits for *this* order to be ingested, which is the observable proof that its
    # own contribution has landed in both the count and the GMV.
    order_id = world.state.extra["paid_order_id"]
    deadline = time.monotonic() + ORDER_FACTS_LAG_S
    metrics: dict = {}
    while time.monotonic() < deadline:
        metrics = _read_cockpit(world)
        if any(o.get("order_id") == order_id for o in metrics.get("recent_orders") or []):
            break
        time.sleep(1.0)
    world.state.extra["metrics"] = metrics
    delta = (metrics.get("total_orders_24h") or 0) - world.state.extra["orders_before"]
    assert delta >= n, f"total_orders_24h grew by {delta}, expected at least {n}"


@then(parsers.parse("total_revenue_24h has increased by at least {amount:d}"))
def revenue_increased(world: World, amount: int) -> None:
    metrics = world.state.extra["metrics"]
    delta = (metrics.get("total_revenue_24h") or 0) - world.state.extra["revenue_before"]
    assert delta >= amount, f"total_revenue_24h grew by {delta}, expected at least {amount}"


@then(parsers.parse("the recent orders include that order with a total of {amount:d}"))
def recent_orders_include_order(world: World, amount: int) -> None:
    recent = world.state.extra["metrics"].get("recent_orders") or []
    order_id = world.state.extra["paid_order_id"]
    mine = [o for o in recent if o.get("order_id") == order_id]
    assert mine, f"order {order_id} is not in recent_orders within the window: {recent}"
    assert mine[0].get("total") == amount, f"recent order is {mine[0]}, expected total {amount}"
