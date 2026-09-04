from __future__ import annotations

import re
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from src.utils import get_test_data_manager
from tests.e2e.flows import create_order_via_api
from tests.e2e.support.world import World


@given("the buyer has placed an order via API")
def buyer_places_order_api(world: World) -> None:
    buyer = world.state.current_user or get_test_data_manager().get_user_by_role("buyer")
    listing_id = world.state.listing.listing_id if world.state.listing else "default_listing"
    create_order_via_api(world, buyer, listing_id)


@when("the buyer navigates to the order detail page")
def buyer_navigates_to_order_detail(world: World) -> None:
    order_id = world.state.extra.get("order_id", "order_001")
    world.page.goto(f"{world.settings.base_url}/account/orders/{order_id}")
    world.page.wait_for_load_state("networkidle")


@then("the order summary shows the 5-step delivery timeline and items")
def order_shows_timeline(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/account/orders/.*"), timeout=timeouts.DEFAULT)
    # Stepper steps and product list should be present
    expect(world.page.locator("body")).to_contain_text("HÀNH TRÌNH ĐƠN HÀNG")
    expect(world.page.locator("body")).to_contain_text("DANH SÁCH SẢN PHẨM")
