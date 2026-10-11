"""Steps for frontend/uif_cart.feature (ui-phase-cart-checkout failure path)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import parsers, then

from src.constants import timeouts
from tests.e2e.support.uif_support import ALERTS
from tests.e2e.support.world import World


@then(parsers.parse('the segment error alert "{title}" offers a retry button'))
def segment_error(world: World, title: str) -> None:
    alert = world.page.locator(ALERTS).filter(has_text=title)
    expect(alert).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(alert.get_by_role("button", name="Thử lại")).to_be_visible()


@then("the cart is rendered again with its item")
def cart_again(world: World) -> None:
    expect(world.page.get_by_text("Không thể tải giỏ hàng")).to_have_count(
        0, timeout=timeouts.DEFAULT
    )
    expect(world.page.get_by_test_id("cart-shop-group")).to_have_count(
        1, timeout=timeouts.NAVIGATION
    )
