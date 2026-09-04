from __future__ import annotations

import re
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from tests.e2e.support.world import World


@when("the seller navigates to the seller orders page")
def seller_navigates_to_seller_orders(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/seller/orders")
    world.page.wait_for_load_state("networkidle")


@then("the seller orders dashboard displays the order status tabs")
def seller_orders_displays_tabs(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/seller/orders.*"), timeout=timeouts.DEFAULT)
    expect(world.page.locator("body")).to_contain_text("Quản lý đơn hàng", timeout=timeouts.DEFAULT)
