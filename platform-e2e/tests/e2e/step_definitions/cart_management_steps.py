from __future__ import annotations

import re
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from tests.e2e.support.world import World


@when("the buyer views the cart")
def buyer_views_the_cart(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/cart")
    world.page.wait_for_load_state("networkidle")


@then("the buyer can increase the item quantity")
def buyer_increases_quantity(world: World) -> None:
    plus_btn = world.page.get_by_role("button", name="+").first
    expect(plus_btn).to_be_visible(timeout=timeouts.DEFAULT)
    plus_btn.click()
    world.page.wait_for_load_state("networkidle")
    # Verify subtotal or price is rendered
    expect(world.page.locator("body")).to_contain_text("TÓM TẮT ĐƠN HÀNG", timeout=timeouts.DEFAULT)


@when("the buyer clears all items from the cart")
def buyer_clears_all_items(world: World) -> None:
    world.page.once("dialog", lambda dialog: dialog.accept())
    clear_btn = world.page.get_by_role("button", name="Xóa tất cả")
    expect(clear_btn).to_be_visible(timeout=timeouts.DEFAULT)
    clear_btn.click()
    world.page.wait_for_load_state("networkidle")


@then("the cart displays the empty state")
def cart_displays_empty_state(world: World) -> None:
    expect(world.page.locator("body")).to_contain_text("Giỏ hàng của bạn đang trống", timeout=timeouts.DEFAULT)
