"""Steps for frontend/ui_payment_result.feature (OpenSpec change ui-phase-cart-checkout)."""

from __future__ import annotations

import re
import time

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import PaymentService
from src.constants import timeouts
from tests.e2e.step_definitions.d2_cart_steps import toast
from tests.e2e.step_definitions.d2_checkout_steps import slow_actions
from tests.e2e.support import d2_support as d2
from tests.e2e.support.world import World


@given("a d2 buyer has an order awaiting mock payment")
def order_awaiting_payment(world: World) -> None:
    d2.seed_buyer(world)
    shop = d2.seed_shop(price=300000)
    order_id = d2.place_order(world, shop)
    res = PaymentService(token=world.state.extra["d2_buyer"].token).create_payment(order_id)
    assert (res.get("transaction") or {}).get("id"), res


@when("the buyer opens the d2 payment page for the order")
def open_payment_page(world: World) -> None:
    world.page.goto(
        f"{world.settings.base_url}/checkout/pay/{world.state.extra['d2_order_id']}",
        wait_until="domcontentloaded",
    )


@when(parsers.parse('the buyer presses "{label}" while the payment server is slow'))
def press_simulator(world: World, label: str) -> None:
    page = world.page
    button = page.get_by_role("button", name=label)
    expect(button).to_be_enabled(timeout=timeouts.NAVIGATION)
    page.wait_for_timeout(500)
    slow_actions(world, 1.5)
    button.click()


@then("both simulator buttons are disabled while pending")
def both_disabled(world: World) -> None:
    page = world.page
    expect(page.get_by_role("button", name="Thanh toán thành công")).to_be_disabled(
        timeout=timeouts.DEFAULT
    )
    expect(page.get_by_role("button", name="Thanh toán thất bại")).to_be_disabled()


@then("a success result links to the order list and the order status becomes PAID")
def success_and_paid(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", name="Thanh toán thành công")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(page.get_by_role("link", name="Xem đơn hàng")).to_have_attribute(
        "href", "/account/orders"
    )
    deadline = time.monotonic() + 15
    status = ""
    while time.monotonic() < deadline:
        order = d2.buyer_orders(world).get_order(world.state.extra["d2_order_id"])["order"]
        status = order.get("status", "")
        if status == "ORDER_STATUS_PAID":
            break
        time.sleep(0.5)
    assert status == "ORDER_STATUS_PAID", status


@then("an error result offers retry and change of payment method and an error toast is displayed")
def failure_and_toast(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", name="Thanh toán thất bại")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(page.get_by_role("button", name="Thử lại")).to_be_visible()
    expect(page.get_by_role("link", name="Đổi phương thức")).to_be_visible()
    expect(
        toast(page, re.compile(r"\S{4,}"), "alert").filter(has_not_text="Đóng").first
    ).to_be_visible()


@when("the payment step of the order is forced to fail")
def force_fail(world: World) -> None:
    d2.force_fail_payment(world.state.extra["d2_order_id"])
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        order = d2.buyer_orders(world).get_order(world.state.extra["d2_order_id"])["order"]
        if order.get("status") == "ORDER_STATUS_CANCELLED":
            return
        time.sleep(0.5)
    raise AssertionError("the saga never cancelled the order")


@then("the page explains that stock was released and the order was cancelled")
def compensation_explained(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", name="Đơn hàng đã bị hủy")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(page.get_by_text("tồn kho đã được giải phóng", exact=False)).to_be_visible()
    expect(page.get_by_role("link", name="Xem đơn hàng")).to_have_attribute(
        "href", "/account/orders"
    )


@when("the buyer opens the payment page of the unknown order id")
def open_unknown_payment(world: World) -> None:
    world.page.goto(
        f"{world.settings.base_url}/checkout/pay/does-not-exist", wait_until="domcontentloaded"
    )


@then("a 404 result links to the order list")
def not_found_result(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Không tìm thấy giao dịch")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("link", name="Xem đơn hàng")).to_have_attribute(
        "href", "/account/orders"
    )
