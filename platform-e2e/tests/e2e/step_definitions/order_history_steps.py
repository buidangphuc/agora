from __future__ import annotations

import re
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from tests.e2e.support.world import World


@when("the buyer navigates to the orders page")
def buyer_navigates_to_orders_page(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/account/orders")
    world.page.wait_for_load_state("networkidle")


@then("the orders list displays the placed order cards with status badges")
def orders_list_displays_cards(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/account/orders.*"), timeout=timeouts.DEFAULT)
    expect(world.page.locator("body")).to_contain_text("Đơn hàng của tôi", timeout=timeouts.DEFAULT)
    expect(world.page.locator("body")).to_contain_text("Mã đơn:", timeout=timeouts.DEFAULT)
