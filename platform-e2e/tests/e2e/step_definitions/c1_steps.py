"""c1 e2e track: gaps of notification-delivery-hardening, notify-chat-and-shipment and
ops-cockpit-real-data.

Scenarios that need a REAL fault (Kafka, team-domain, team-notification, team-analytics
or Jaeger stopped) are tagged `@destructive` in the feature files: they run only in the
serial lane, and every teardown restores what it stopped.
"""

from __future__ import annotations

import json
import time
import uuid

import httpx
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.api.services import GatewayError
from src.constants import PageName, timeouts
from src.utils import data as fake
from tests.e2e.flows import stop_container
from tests.e2e.step_definitions.journey_steps import (
    _act_as,
    _buyer,
    _find_notifications,
    _order,
    _poll_notification,
    _seller,
    _thread_messages,
)
from tests.e2e.step_definitions.notification_hardening_steps import _wait_notifications_ready
from tests.e2e.support import c1_support as c1
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

NEUTRAL_SENDER_TITLE = "Tin nhắn mới từ Người dùng"
CHAT_GROUP = "team-notification.chat"
CHAT_TOPIC = "chat.events"
ANALYTICS_GROUP = "team-analytics"
ORDER_TOPIC = "order.events"
_DELIVERY_WINDOW_S = 90
_SETTLE_S = 8


def _x(world: World) -> dict:
    return world.state.extra


def _start_after(world: World, name: str, restore, wait) -> None:  # noqa: ANN001
    """Register teardown that starts `name` again (idempotent) and waits for it."""

    def _restore() -> None:
        restore()
        wait()

    world.add_cleanup(_restore)


# ── notification-delivery-hardening: outbox, restart, names ──────────────────────
@when("Kafka is stopped")
def kafka_stopped(world: World) -> None:
    restore = stop_container(c1.redpanda_container())
    _x(world)["restore_kafka"] = restore
    _start_after(world, "kafka", restore, c1.wait_kafka_ready)


@when("Kafka is started again")
def kafka_started(world: World) -> None:
    _x(world)["restore_kafka"]()
    c1.wait_kafka_ready()


@then("the buyer receives exactly one chat notification for the seller's reply")
def buyer_one_chat_notification_after_recovery(world: World) -> None:
    thread_id = _x(world)["chat_thread_id"]
    deadline = time.monotonic() + _DELIVERY_WINDOW_S
    found: list[dict] = []
    while time.monotonic() < deadline:
        found = _find_notifications(
            world, _buyer(world), "NOTIFICATION_TYPE_CHAT", f"/chat/{thread_id}"
        )
        if found:
            break
        time.sleep(3)
    assert found, f"no chat notification within {_DELIVERY_WINDOW_S}s after Kafka recovered"
    time.sleep(_SETTLE_S)  # a duplicate relay would land inside the settle window
    found = _find_notifications(
        world, _buyer(world), "NOTIFICATION_TYPE_CHAT", f"/chat/{thread_id}"
    )
    assert len(found) == 1, f"expected exactly one chat notification, got {found}"
    assert found[0].get("body") == _x(world)["chat_reply"], found[0]


@when("a user outside the thread tries to send a message into it")
def outsider_sends_message(world: World) -> None:
    password = world.settings.seed_password
    token = world.service_factory.auth.register(fake.unique_username("outsider"), password, "buyer")
    world.service_factory.set_token(token)
    text = f"Tin nhắn không hợp lệ {uuid.uuid4().hex[:8]}"
    status = None
    try:
        world.service_factory.chat.send_message(_x(world)["chat_thread_id"], text)
    except GatewayError as exc:
        status = exc.status
    _x(world)["rejected_text"] = text
    _x(world)["rejected_status"] = status


@then("the message is rejected and is not stored in the thread")
def message_rejected_not_stored(world: World) -> None:
    status = _x(world)["rejected_status"]
    assert status is not None and 400 <= status < 500, f"outsider's SendMessage -> {status}"
    for user in (_buyer(world), _seller(world)):
        contents = [m.get("content") for m in _thread_messages(world, user)]
        assert _x(world)["rejected_text"] not in contents, "rejected message was stored"


@then("neither participant has a chat notification for the rejected message")
def no_notification_for_rejected(world: World) -> None:
    text = _x(world)["rejected_text"]
    for user in (_buyer(world), _seller(world)):
        found = _find_notifications(world, user, "NOTIFICATION_TYPE_CHAT", text)
        assert not found, f"a rejected message still produced a notification: {found}"


@when("team-notification is stopped and its chat consumer group is rewound to the start")
def stop_and_rewind_chat_group(world: World) -> None:
    name = c1.notification_container()
    restore = stop_container(name)
    _x(world)["restore_notification"] = restore
    _start_after(world, name, restore, lambda: pe.wait_healthy(name, 120))
    deadline = time.monotonic() + 60
    last = ""
    while time.monotonic() < deadline:
        out = c1.rpk(
            "group", "seek", CHAT_GROUP, "--to", "start", "--topics", CHAT_TOPIC, check=False
        )
        last = (out.stdout + out.stderr).strip()
        if out.returncode == 0:
            return
        time.sleep(2)  # the group is still draining its departed member
    raise AssertionError(f"could not rewind {CHAT_GROUP}: {last}")


@when("team-notification is running again and has replayed chat.events")
def start_and_wait_replay(world: World) -> None:
    _x(world)["restore_notification"]()
    pe.wait_healthy(c1.notification_container(), 120)
    _act_as(world, _buyer(world))
    _wait_notifications_ready(world)
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        current, end = c1.group_topic_offsets(CHAT_GROUP, CHAT_TOPIC)
        if current >= end:
            time.sleep(_SETTLE_S)
            return
        time.sleep(2)
    raise AssertionError(f"{CHAT_GROUP} did not catch up on {CHAT_TOPIC}")


@then("the buyer still has exactly one chat notification for the seller's reply")
def buyer_still_one_notification(world: World) -> None:
    thread_id = _x(world)["chat_thread_id"]
    found = _find_notifications(
        world, _buyer(world), "NOTIFICATION_TYPE_CHAT", f"/chat/{thread_id}"
    )
    assert len(found) == 1, f"the replayed event notified again: {found}"


@when("the sender-name lookup is unavailable")
def sender_lookup_unavailable(world: World) -> None:
    # The buyer is not the thread's seller, so the only name source is team-identity.
    name = c1.identity_container()
    restore = stop_container(name)
    _start_after(world, name, restore, lambda: pe.wait_healthy(name, 120))


@then("the seller's chat notification is created with the neutral sender label")
def chat_notification_neutral_label(world: World) -> None:
    thread_id = _x(world)["chat_thread_id"]
    found = _poll_notification(
        world, _seller(world), "NOTIFICATION_TYPE_CHAT", f"/chat/{thread_id}"
    )
    assert found, "no chat notification when the name lookup failed"
    assert (
        found[0].get("title") == NEUTRAL_SENDER_TITLE
    ), f"title {found[0].get('title')!r} is not the neutral label {NEUTRAL_SENDER_TITLE!r}"
    assert found[0].get("body") == _x(world)["chat_inquiry"], found[0]


# ── notify-chat-and-shipment: shipment atomicity and analytics skip ──────────────
@when("another seller tries to create a shipment for the order")
def other_seller_ships(world: World) -> None:
    password = world.settings.seed_password
    token = world.service_factory.auth.register(fake.unique_username("seller"), password, "seller")
    world.service_factory.set_token(token)
    code = f"SPX-NOPE-{uuid.uuid4().hex[:8].upper()}"
    status = None
    try:
        world.service_factory.order.create_shipment(
            order_id=world.state.order_id, carrier="SPX Express", tracking_code=code
        )
    except GatewayError as exc:
        status = exc.status
    _x(world)["rejected_tracking"] = code
    _x(world)["rejected_status"] = status


@then("the shipment is rejected and the order is not shipped")
def shipment_rejected(world: World) -> None:
    status = _x(world)["rejected_status"]
    assert status is not None and 400 <= status < 500, f"foreign CreateShipment -> {status}"
    order = _order(world)
    assert order.get("status") == "ORDER_STATUS_PENDING", f"order changed: {order}"
    try:
        tracking = world.service_factory.order.get_shipment_tracking(_x(world)["rejected_tracking"])
    except GatewayError:
        return
    assert not tracking.get("shipment"), f"a shipment was stored: {tracking}"


@then("the buyer has no order notification for the rejected shipment")
def no_order_notification_for_rejected(world: World) -> None:
    time.sleep(_SETTLE_S)  # an OrderShipped event would be consumed well inside this window
    found = _find_notifications(
        world, _buyer(world), "NOTIFICATION_TYPE_ORDER", _x(world)["rejected_tracking"]
    )
    assert not found, f"a rejected shipment still notified the buyer: {found}"


@given("the order.events end offset is noted")
def note_order_end(world: World) -> None:
    _x(world)["order_end_before"] = c1.topic_end_offset(ORDER_TOPIC)


@then("within 30 seconds team-analytics' order consumer has committed past the OrderShipped event")
def analytics_past_shipped(world: World) -> None:
    before = _x(world)["order_end_before"]
    deadline = time.monotonic() + 30
    end = before
    # 1) the OrderShipped event reaches order.events (outbox relay) ...
    while time.monotonic() < deadline and end <= before:
        end = c1.topic_end_offset(ORDER_TOPIC)
        time.sleep(1)
    assert end > before, "no OrderShipped event was published to order.events"
    # 2) ... and the analytics consumer skips it and advances its offset beyond it.
    current = 0
    while time.monotonic() < deadline:
        current, _ = c1.group_topic_offsets(ANALYTICS_GROUP, ORDER_TOPIC)
        if current >= end:
            break
        time.sleep(1)
    assert current >= end, f"{ANALYTICS_GROUP} stuck at {current} < {end} on {ORDER_TOPIC}"
    _x(world)["analytics_committed"] = current


@then("team-analytics is still running")
def analytics_running(world: World) -> None:
    from tests.e2e.support import tii_support as tii

    out = c1.docker("inspect", tii.analytics_container(), "--format", "{{.State.Status}}")
    assert out.stdout.strip() == "running", out.stdout


# ── ops-cockpit-real-data ────────────────────────────────────────────────────────
def _fetch_admin_metrics(world: World) -> dict:
    token = world.state.current_user.token
    world.service_factory.set_token(token)
    return world.service_factory.metrics.get_cockpit_metrics()


@then("the cockpit page renders the figures the gateway returns for that admin")
def page_renders_gateway_figures(world: World) -> None:
    page = world.page
    metrics = page.get_by_test_id("cockpit_metrics")
    expect(metrics).to_be_visible(timeout=timeouts.DEFAULT)
    api = _fetch_admin_metrics(world)
    seen = _x(world).get("metrics") or {}
    # The page may be one poll behind the API read: accept either snapshot's figure.
    accepted = {
        f"{v:,}"
        for v in (api.get("total_orders_24h"), seen.get("total_orders_24h"))
        if isinstance(v, int)
    }
    assert accepted, f"gateway returned no total_orders_24h for the admin: {api}"
    deadline = time.monotonic() + 15
    text = ""
    while time.monotonic() < deadline:
        text = metrics.inner_text()
        if any(a in text for a in accepted):
            break
        time.sleep(1)
    assert any(a in text for a in accepted), f"HUD does not show {accepted}: {text!r}"
    orders = page.get_by_test_id("cockpit_orders")
    recent = api.get("recent_orders") or []
    if recent:
        expect(orders).to_contain_text(recent[0]["order_id"][:8], timeout=timeouts.DEFAULT)


@when("the cockpit endpoint is called as the admin")
def call_cockpit_as_admin(world: World) -> None:
    _x(world)["metrics"] = _fetch_admin_metrics(world)


@then("within 90 seconds recent_traces contains a trace that opens in the Jaeger UI")
def recent_trace_opens_in_jaeger(world: World) -> None:
    deadline = time.monotonic() + 90
    traces: list[dict] = []
    while time.monotonic() < deadline:
        traces = _fetch_admin_metrics(world).get("recent_traces") or []
        if traces:
            break
        world.service_factory.search.search("laptop")
        time.sleep(3)
    assert traces, "recent_traces stayed empty although requests passed through the gateway"
    trace = traces[0]
    assert trace.get("trace_id") and trace.get("jaeger_url"), f"incomplete trace: {trace}"
    assert trace["trace_id"] in trace["jaeger_url"], trace
    _x(world)["trace"] = trace
    # The id resolves in Jaeger itself (query API behind the same UI link).
    base = trace["jaeger_url"].split("/trace/")[0]
    r = httpx.get(f"{base}/api/traces/{trace['trace_id']}", timeout=10)
    assert (
        r.status_code == httpx.codes.OK
    ), f"Jaeger does not know {trace['trace_id']}: {r.status_code}"
    assert r.json().get("data"), f"Jaeger returned no trace for {trace['trace_id']}"


@then("the response keeps its shape and the figures are empty")
def figures_empty(world: World) -> None:
    metrics = _x(world)["metrics"]
    for key in ("services", "timestamp", "total_rps"):
        assert key in metrics, f"response lost its shape: missing {key}"
    assert metrics.get("total_orders_24h") is None, metrics.get("total_orders_24h")
    assert metrics.get("total_revenue_24h") is None, metrics.get("total_revenue_24h")
    assert not metrics.get("recent_orders"), metrics.get("recent_orders")


@then("recent_traces is empty")
def traces_empty(world: World) -> None:
    assert not _x(world)["metrics"].get("recent_traces"), _x(world)["metrics"].get("recent_traces")


@when("Jaeger is stopped")
def jaeger_stopped(world: World) -> None:
    name = c1.jaeger_container()
    restore = stop_container(name)
    _start_after(world, name, restore, lambda: pe.wait_healthy(name, 120))


@then('the HUD shows "Chưa có dữ liệu" for the traces')
def hud_traces_empty(world: World) -> None:
    world.navigate_to(PageName.ADMIN_COCKPIT)
    page = world.page
    expect(page.get_by_test_id("cockpit_metrics")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_test_id("cockpit_traces")).to_have_count(0)
    expect(page.get_by_text("Chưa có dữ liệu — dùng liên kết Jaeger", exact=False)).to_be_visible()


_NULL_PAYLOAD = {
    "timestamp": "2026-01-01T00:00:00Z",
    "prometheus_available": False,
    "total_rps": 0,
    "avg_latency_ms": None,
    "total_orders_24h": None,
    "total_revenue_24h": None,
    "services": None,
    "recent_orders": None,
    "recent_traces": None,
}


@when("the cockpit receives a response with every figure null")
def cockpit_gets_null_payload(world: World) -> None:
    # Browser-level stub of the poll response only (the SSR first paint stays real):
    # the page contract under test is how it RENDERS null, not what the backend sends.
    world.page.route(
        "**/api/admin/metrics",
        lambda route: route.fulfill(
            status=200, content_type="application/json", body=json.dumps(_NULL_PAYLOAD)
        ),
    )
    world.navigate_to(PageName.ADMIN_COCKPIT)
    # The first poll lands one interval (3 s) after the server render.
    expect(world.page.get_by_text("Prometheus không khả dụng", exact=False)).to_be_visible(
        timeout=15_000
    )


@then('the HUD shows a dash or "Chưa có dữ liệu" and no placeholder number, row or trace id')
def hud_shows_empty_state(world: World) -> None:
    page = world.page
    cards = page.get_by_test_id("cockpit_metrics").inner_text()
    assert cards.count("—") >= 2, f"throughput/latency do not render a dash: {cards!r}"
    assert (
        cards.count("Chưa có dữ liệu") >= 2
    ), f"orders/GMV do not say 'Chưa có dữ liệu': {cards!r}"
    for fabricated in ("1,420", "384,500,000"):
        assert fabricated not in page.content(), f"fabricated figure {fabricated} rendered"
    expect(page.get_by_test_id("cockpit_orders")).to_contain_text("Chưa có dữ liệu")
    expect(page.get_by_test_id("cockpit_orders").get_by_text("🛒", exact=False)).to_have_count(0)
    expect(page.get_by_test_id("cockpit_health")).to_contain_text("Chưa có dữ liệu")
    expect(page.get_by_test_id("cockpit_traces")).to_have_count(0)
