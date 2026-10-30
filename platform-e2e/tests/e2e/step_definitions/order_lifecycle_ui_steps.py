"""Steps for frontend/order_lifecycle_ui.feature (change frontend-order-lifecycle).

Black box: orders are seeded through the gateway by the real buyer and seller, the browser is
logged in as the persona under test, and every outcome is read off the rendered page and,
where the spec promises state, off the gateway. Reuses the d2 seeding helpers.
"""

from __future__ import annotations

import json
import re
import time
from typing import Any

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import OrderService
from src.api.services.base_service import GatewayError
from src.constants import timeouts
from tests.e2e.flows.auth_flow import session_cookies
from tests.e2e.support import d2_support as d2
from tests.e2e.support import order_lifecycle_ui_support as ui
from tests.e2e.support.apps import app_url
from tests.e2e.support.world import World

PAYMENT_BUTTONS = ("Thanh toán thành công", "Thanh toán thất bại")


def base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


def ctx(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("olc", {})


def btoken(world: World) -> str:
    return ctx(world)["buyer_token"]


def oid(world: World) -> str:
    return ctx(world)["order_id"]


def login_as(world: World, token: str) -> None:
    world.context.add_cookies(
        session_cookies(world.settings, token)
    )


def goto(world: World, path: str):
    return world.page.goto(app_url(world.settings, path), wait_until="domcontentloaded")


def start_buyer(world: World) -> None:
    """A buyer (browser logged in) and a seller with one published listing."""
    buyer = d2.seed_buyer(world)
    shop = d2.seed_shop(price=100_000, stock=50)
    ctx(world).update(
        buyer_token=buyer.token, seller_token=shop["token"], listing_id=shop["listing_id"]
    )


# ── Given ────────────────────────────────────────────────────────────────
@given("a lifecycle buyer has a paid online order")
def paid_online_order(world: World) -> None:
    start_buyer(world)
    ctx(world)["order_id"] = ui.paid_order(btoken(world), ctx(world)["listing_id"])


@given("a lifecycle buyer has an online order whose payment failed")
def failed_payment_order(world: World) -> None:
    start_buyer(world)
    order_id = ui.place(btoken(world), ctx(world)["listing_id"])
    ui.settle_payment(btoken(world), order_id, False)
    ctx(world)["order_id"] = order_id
    assert ui.get_order(btoken(world), order_id).get("status") == "ORDER_STATUS_PENDING"


@given("a lifecycle buyer has an online order with no payment transaction")
def no_transaction_order(world: World) -> None:
    start_buyer(world)
    ctx(world)["order_id"] = ui.place(btoken(world), ctx(world)["listing_id"])


@given("a lifecycle buyer has an online order awaiting payment")
def awaiting_payment_order(world: World) -> None:
    start_buyer(world)
    ctx(world)["order_id"] = ui.place(btoken(world), ctx(world)["listing_id"], pay_tx=True)


@given("the lifecycle seller ships the order")
def seller_ships(world: World) -> None:
    OrderService(token=ctx(world)["seller_token"]).update_order_status(
        oid(world), "ORDER_STATUS_SHIPPED"
    )


@given("the lifecycle pay page offers the payment simulator before the deadline")
def simulator_before_deadline(world: World) -> None:
    """Positive control for the expiry scenario: the buttons exist while the order is live."""
    goto(world, f"/checkout/pay/{oid(world)}")
    for name in PAYMENT_BUTTONS:
        expect(world.page.get_by_role("button", name=name)).to_be_visible(
            timeout=timeouts.NAVIGATION
        )


@given("the lifecycle order completed by itself")
def completed_by_itself(world: World) -> None:
    order = ui.wait_order(
        btoken(world),
        oid(world),
        lambda o: o.get("status") == "ORDER_STATUS_COMPLETED",
        45,
        "COMPLETED",
    )
    assert order.get("returnWindowEndsAt"), order


def seller_session(world: World) -> dict[str, Any]:
    seller = d2.seed_seller_session(world, listings=1)
    buyer = d2.seed_buyer_token()
    ctx(world).update(
        seller_token=seller["token"],
        buyer_token=buyer["token"],
        listing_id=seller["listings"][0]["id"],
    )
    return seller


@given("a lifecycle seller is signed in with a paid online order left unshipped")
def seller_with_paid_unshipped(world: World) -> None:
    seller_session(world)
    ctx(world)["order_id"] = ui.paid_order(btoken(world), ctx(world)["listing_id"])


@given("a lifecycle seller is signed in with a new order placed by a buyer")
def seller_with_new_order(world: World) -> None:
    seller_session(world)
    ctx(world)["order_id"] = ui.place(btoken(world), ctx(world)["listing_id"], "PAYMENT_METHOD_COD")


@given("the lifecycle owning seller can open the seller order page")
def owner_opens_order(world: World) -> None:
    """Positive control: the same link works for the order's own seller."""
    resp = goto(world, f"/seller/orders/{oid(world)}")
    assert resp is not None and resp.status == 200, resp and resp.status
    expect(world.page.get_by_text(oid(world)[:8], exact=False).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


# ── Opening pages ────────────────────────────────────────────────────────
@when("the lifecycle buyer opens the order detail")
def open_detail(world: World) -> None:
    goto(world, f"/account/orders/{oid(world)}")


@when("the lifecycle buyer opens the order list")
def open_list(world: World) -> None:
    goto(world, "/account/orders")


@when("the lifecycle buyer opens the pay page of the order")
def open_pay_page(world: World) -> None:
    ctx(world)["pay_response"] = goto(world, f"/checkout/pay/{oid(world)}")


@when("the lifecycle buyer opens the notifications page")
def open_notifications(world: World) -> None:
    goto(world, "/notifications")


@when("the lifecycle seller opens the notifications page")
def seller_open_notifications(world: World) -> None:
    goto(world, "/notifications")


@when("the lifecycle seller opens the dashboard")
def seller_dashboard(world: World) -> None:
    goto(world, "/seller")


@when("the lifecycle seller opens the seller order detail")
def seller_order_detail(world: World) -> None:
    goto(world, f"/seller/orders/{oid(world)}")


# ── Cancel with a reason ─────────────────────────────────────────────────
@when(parsers.parse('the lifecycle buyer cancels it choosing the reason "{label}"'))
def cancel_with_reason(world: World, label: str) -> None:
    page = world.page
    button = page.get_by_role("button", name="Hủy đơn", exact=True)
    expect(button).to_be_enabled(timeout=timeouts.NAVIGATION)
    page.wait_for_timeout(500)
    button.click()
    dialog = page.get_by_role("dialog")
    expect(dialog).to_be_visible(timeout=timeouts.DEFAULT)
    if dialog.locator("select").count():
        dialog.locator("select").first.select_option(label=label)
    elif dialog.get_by_role("radio", name=label).count():
        dialog.get_by_role("radio", name=label).first.check()
    else:
        dialog.get_by_text(label, exact=True).first.click()
    dialog.get_by_role("button", name="Xác nhận hủy đơn").click()


@then(parsers.parse('the lifecycle order detail shows the status "{text}"'))
def detail_status(world: World, text: str) -> None:
    expect(world.page.get_by_test_id("order-status")).to_have_text(
        text, timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the lifecycle order reads back cancel reason "{reason}"'))
def reads_cancel_reason(world: World, reason: str) -> None:
    order = ui.get_order(btoken(world), oid(world))
    assert order.get("status") == "ORDER_STATUS_CANCELLED", order
    assert order.get("cancelReason") == reason, order


@then(parsers.parse('the lifecycle order reads back status "{status}"'))
def reads_status(world: World, status: str) -> None:
    order = ui.wait_order(
        btoken(world), oid(world), lambda o: o.get("status") == status, 15, status
    )
    assert order["status"] == status


@then(parsers.parse('the lifecycle order reads back status "{status}" with a completion time'))
def reads_completed(world: World, status: str) -> None:
    order = ui.wait_order(
        btoken(world), oid(world), lambda o: o.get("status") == status, 15, status
    )
    assert order.get("completedAt"), order


# ── Pay page ─────────────────────────────────────────────────────────────
@when(parsers.parse('the lifecycle buyer presses "{label}" on the order'))
def press_link(world: World, label: str) -> None:
    link = world.page.get_by_role("link", name=label).first
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    link.click()
    world.page.wait_for_url(
        re.compile(rf".*/checkout/pay/{oid(world)}"), timeout=timeouts.NAVIGATION
    )


def simulator_button(world: World):
    return world.page.get_by_role("button", name="Thanh toán thành công")


@then("the lifecycle pay page offers the payment simulator")
def pay_page_simulator(world: World) -> None:
    page = world.page
    retry = page.get_by_role("button", name="Thử lại")
    # a reused FAILED transaction opens in its failed phase; the retry returns to the simulator
    expect(simulator_button(world).or_(retry).first).to_be_visible(timeout=timeouts.NAVIGATION)
    if retry.count() and not simulator_button(world).count():
        retry.first.click()
    expect(simulator_button(world)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("button", name="Thanh toán thất bại")).to_be_visible()


@when("the lifecycle buyer pays successfully on the pay page")
def pay_success(world: World) -> None:
    page = world.page
    page.wait_for_timeout(500)
    simulator_button(world).click()
    expect(page.get_by_role("heading", name="Thanh toán thành công")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the lifecycle order list shows the status "{text}"'))
def list_status(world: World, text: str) -> None:
    expect(world.page.get_by_text(text, exact=True).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("the lifecycle pay page answers 200 and offers the payment simulator")
def pay_answers_200(world: World) -> None:
    resp = ctx(world)["pay_response"]
    assert resp is not None and resp.status == 200, resp and resp.status
    pay_page_simulator(world)


@then(parsers.parse('the lifecycle payment of the order reads back status "{status}"'))
def payment_reads(world: World, status: str) -> None:
    deadline = time.monotonic() + 15
    got = ""
    while time.monotonic() < deadline:
        got = ui.payment_status(btoken(world), oid(world))
        if got == status:
            return
        time.sleep(0.5)
    raise AssertionError(f"payment of {oid(world)} reads {got!r}, expected {status!r}")


@then(parsers.parse('the lifecycle pay page shows "{text}" and no payment buttons'))
def pay_expired(world: World, text: str) -> None:
    page = world.page
    expect(page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)
    for name in PAYMENT_BUTTONS:
        expect(page.get_by_role("button", name=name)).to_have_count(0)


# ── Timers ───────────────────────────────────────────────────────────────
@when("the lifecycle payment deadline passes without payment")
def deadline_passes(world: World) -> None:
    ui.wait_order(
        btoken(world),
        oid(world),
        lambda o: o.get("status") == "ORDER_STATUS_CANCELLED",
        ui_expiry_wait(),
        "CANCELLED by expiry",
    )


def ui_expiry_wait() -> float:
    return 45.0 + 3 * 2.0 + 25


@when("the lifecycle auto-complete time passes")
def auto_complete_passes(world: World) -> None:
    ui.wait_order(
        btoken(world),
        oid(world),
        lambda o: o.get("status") == "ORDER_STATUS_COMPLETED",
        45,
        "COMPLETED by auto-complete",
    )


@when("the lifecycle return window passes")
def window_passes(world: World) -> None:
    order = ui.get_order(btoken(world), oid(world))
    assert order.get("returnWindowEndsAt"), order
    ui.sleep_until(ui.ts(order["returnWindowEndsAt"]) + 1.5)


@when("the lifecycle ship-by time passes")
def ship_by_passes(world: World) -> None:
    order = ui.get_order(btoken(world), oid(world))
    assert order.get("shipBy"), f"order has no ship-by: {order}"
    ui.sleep_until(ui.ts(order["shipBy"]) + 3.0)


# ── Receipt, review, returns ─────────────────────────────────────────────
@when("the lifecycle buyer confirms receipt of the order")
def confirm_receipt(world: World) -> None:
    page = world.page
    button = page.get_by_role("button", name="Đã nhận được hàng").first
    expect(button).to_be_enabled(timeout=timeouts.NAVIGATION)
    button.click()
    dialog = page.get_by_role("dialog")
    expect(dialog).to_be_visible(timeout=timeouts.DEFAULT)
    dialog.get_by_test_id("receipt-confirm").click()
    expect(dialog).to_have_count(0, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the lifecycle order detail offers the "{label}" button'))
def offers_button(world: World, label: str) -> None:
    expect(world.page.get_by_role("button", name=label).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then(parsers.re(r'the lifecycle order detail offers "(?P<label>[^"]+)"$'))
def offers_text(world: World, label: str) -> None:
    expect(world.page.get_by_text(label).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the lifecycle order detail no longer offers "{label}"'))
def no_longer_offers(world: World, label: str) -> None:
    expect(world.page.get_by_text(label)).to_have_count(0, timeout=timeouts.DEFAULT)


@then(parsers.parse('the lifecycle order detail offers "{action}" and "{window}"'))
def offers_return_and_window(world: World, action: str, window: str) -> None:
    page = world.page
    expect(page.get_by_role("button", name=action, exact=True).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(page.get_by_text(window).first).to_be_visible()


@then(parsers.parse('the lifecycle order detail shows "{closed}" and no "{action}"'))
def shows_window_closed(world: World, closed: str, action: str) -> None:
    page = world.page
    expect(page.get_by_text(closed).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("button", name=action, exact=True)).to_have_count(0)


@when(parsers.parse('the lifecycle buyer submits a return with the reason "{reason}"'))
def submit_return(world: World, reason: str) -> None:
    page = world.page
    button = page.get_by_role("button", name="Yêu cầu trả hàng", exact=True).first
    expect(button).to_be_enabled(timeout=timeouts.NAVIGATION)
    page.wait_for_timeout(500)
    button.click()
    expect(page.get_by_role("dialog")).to_be_visible(timeout=timeouts.DEFAULT)
    page.get_by_test_id("return-reason").select_option(reason)
    page.get_by_test_id("return-submit").click()


@then(parsers.parse('the lifecycle returns tab shows a "{badge}" badge'))
def returns_tab_badge(world: World, badge: str) -> None:
    page = world.page
    expect(page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.NAVIGATION)
    page.get_by_role("tab", name="Trả hàng / Hoàn tiền").click()
    expect(page.get_by_test_id("return-status")).to_have_text(badge, timeout=timeouts.DEFAULT)


@then("the lifecycle order has exactly one PENDING return")
def one_pending_return(world: World) -> None:
    res = OrderService(token=btoken(world)).post(
        "/platform.order.v1.OrderService/ListOrderReturns", {"orderId": oid(world)}
    )
    returns = res.get("returns", [])
    assert len(returns) == 1, returns
    assert returns[0].get("status") == "RETURN_STATUS_PENDING", returns


@then(parsers.parse('a lifecycle return request through the gateway is refused with "{code}"'))
def return_refused(world: World, code: str) -> None:
    try:
        OrderService(token=btoken(world)).create_return_request(oid(world), "defective", 1000)
    except GatewayError as err:
        assert json.loads(err.body).get("code") == code, (err.status, err.body)
        return
    raise AssertionError("CreateReturnRequest after the window was accepted")


# ── Seller ───────────────────────────────────────────────────────────────
def kpi_cell(world: World):
    return world.page.locator("div").filter(has_text="Cần gửi").last


@then(parsers.parse('the lifecycle dashboard shows "{kpi}" without "{overdue}"'))
def dashboard_no_overdue(world: World, kpi: str, overdue: str) -> None:
    page = world.page
    expect(page.get_by_text(kpi).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_text(overdue)).to_have_count(0)


@then(parsers.parse('the lifecycle dashboard shows "{kpi}" with "{overdue}"'))
def dashboard_overdue(world: World, kpi: str, overdue: str) -> None:
    page = world.page
    expect(page.get_by_text(kpi).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_text(overdue)).to_be_visible()


@then(parsers.parse('the lifecycle seller order detail shows "{deadline}" and the "{tag}" tag'))
def seller_detail_ship_by(world: World, deadline: str, tag: str) -> None:
    page = world.page
    expect(page.get_by_text(deadline).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_text(tag, exact=True).first).to_be_visible()


# ── Notifications ────────────────────────────────────────────────────────
def find_link(world: World, title: str, href: str | None, timeout: float = 75.0):
    """Reload the notifications page until a link titled `title` shows (feed is async).

    The route streams behind loading.tsx: at domcontentloaded the list can still sit in
    React's hidden Suspense segment, where get_by_role finds nothing. Wait for the page
    heading to be revealed before each count so a poll never reads the fallback.
    """
    page = world.page
    deadline = time.monotonic() + timeout
    while True:
        expect(page.get_by_role("heading", name="Trung tâm thông báo")).to_be_visible(
            timeout=timeouts.NAVIGATION
        )
        links = page.get_by_role("link").filter(has_text=title)
        if href:
            links = links.and_(page.locator(f'a[href="{href}"]'))
        if links.count():
            return links
        assert time.monotonic() < deadline, f"no {title!r} notification linking to {href}"
        time.sleep(2)
        page.reload(wait_until="domcontentloaded")


@when(parsers.parse('the lifecycle seller follows the "{title}" notification'))
def seller_follows(world: World, title: str) -> None:
    links = find_link(world, title, f"/seller/orders/{oid(world)}")
    links.first.click()
    world.page.wait_for_url(
        re.compile(rf".*/seller/orders/{oid(world)}"), timeout=timeouts.NAVIGATION
    )
    # a client-side transition has no document response: reload the landing URL for its status
    ctx(world)["landing"] = world.page.reload(wait_until="domcontentloaded")


@then("the lifecycle seller lands on the seller order page of that order with status 200")
def seller_lands(world: World) -> None:
    landing = ctx(world)["landing"]
    assert landing.status == 200, landing.status
    assert world.page.url.rstrip("/").endswith(f"/seller/orders/{oid(world)}"), world.page.url
    expect(world.page.get_by_text(oid(world)[:8], exact=False).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when("a different lifecycle seller opens the seller order page of that order")
def other_seller_opens(world: World) -> None:
    _, token = d2.register("seller")
    login_as(world, token)
    ctx(world)["other_response"] = goto(world, f"/seller/orders/{oid(world)}")


@then("the lifecycle response is the not-found page with status 404 and no order data")
def not_found_no_data(world: World) -> None:
    resp = ctx(world)["other_response"]
    assert resp is not None and resp.status == 404, resp and resp.status
    body = world.page.inner_text("body")
    assert oid(world) not in body, "order id leaked"
    assert oid(world)[:8] not in body, "order id leaked"
    assert "Hạn gửi" not in body and "Tổng thanh toán" not in body, "order data leaked"


@then("the lifecycle buyer sees exactly one payment reminder")
def one_reminder(world: World) -> None:
    link = f"/checkout/pay/{oid(world)}"
    links = find_link(world, "Đơn hàng sắp hết hạn thanh toán", link, timeout=40)
    assert links.count() == 1, links.count()


@when("the lifecycle buyer follows the payment reminder")
def follow_reminder(world: World) -> None:
    links = find_link(world, "Đơn hàng sắp hết hạn thanh toán", f"/checkout/pay/{oid(world)}")
    links.first.click()
    world.page.wait_for_url(
        re.compile(rf".*/checkout/pay/{oid(world)}"), timeout=timeouts.NAVIGATION
    )
