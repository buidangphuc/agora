"""Steps for frontend/uif_orders.feature (ui-phase-orders failure paths)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import timeouts
from tests.e2e.step_definitions.d2_orders_steps import det, lst
from tests.e2e.support.uif_support import ALERTS
from tests.e2e.support.world import World


@then("an error alert with a retry button replaces the order list and the empty state is not shown")
def list_failed(world: World) -> None:
    page = world.page
    alert = page.locator(ALERTS).filter(has_text="Không tải được danh sách đơn hàng")
    expect(alert).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(alert.get_by_role("button", name="Thử lại")).to_be_visible()
    expect(lst(world).order_cards).to_have_count(0)
    expect(lst(world).empty_text).to_have_count(0)
    expect(page.get_by_text("Chưa có đơn hàng")).to_have_count(0)


@when("the buyer presses the retry button of the alert once")
def press_retry(world: World) -> None:
    # The retry refreshes the route (router.refresh), so one press after the service is back must
    # re-fetch; a same-URL link served from Next's router cache would not.
    world.page.locator(ALERTS).get_by_role("button", name="Thử lại").click()


@then("the buyer's order is listed again")
def listed_again(world: World) -> None:
    expect(lst(world).order_cards).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(world.page.locator(ALERTS)).to_have_count(0)


@then("an error alert with a retry button is shown and no 404 result is rendered")
def detail_failed(world: World) -> None:
    page = world.page
    alert = page.locator(ALERTS).filter(has_text="Không tải được đơn hàng")
    expect(alert).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(alert.get_by_role("button", name="Thử lại")).to_be_visible()
    expect(page.get_by_text("Không tìm thấy đơn hàng")).to_have_count(0)
    expect(page.get_by_text("404", exact=True)).to_have_count(0)


@then("the order detail is rendered again")
def detail_again(world: World) -> None:
    oid = world.state.extra["d2_order_id"]
    expect(world.page.get_by_text(oid[:8]).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.locator(ALERTS)).to_have_count(0)
    assert det(world) is not None
