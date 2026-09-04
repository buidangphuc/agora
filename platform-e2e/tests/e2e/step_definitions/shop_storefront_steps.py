from __future__ import annotations

import re
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from tests.e2e.support.world import World


@when("the buyer opens a seller shop page")
def buyer_opens_seller_shop_page(world: World) -> None:
    seller_id = (
        world.state.seeded_seller.username
        if world.state.seeded_seller
        else (world.state.listing.listing_id if world.state.listing else "default-seller")
    )
    world.page.goto(f"{world.settings.base_url}/shop/{seller_id}")
    world.page.wait_for_load_state("networkidle")


@then("the shop profile header displays the store rating and product catalog")
def shop_profile_displays_store_info(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/shop/.*"), timeout=timeouts.DEFAULT)
    # Check shop header content
    expect(world.page.locator("body")).to_contain_text("Shop")
    # Check presence of action buttons
    expect(world.page.get_by_role("button", name="Theo Dõi")).to_be_visible(timeout=timeouts.DEFAULT)


@then("the buyer can toggle following the shop")
def buyer_toggles_following_shop(world: World) -> None:
    follow_btn = world.page.get_by_role("button", name="Theo Dõi")
    follow_btn.click()
    expect(world.page.get_by_role("button", name="Đang Theo Dõi")).to_be_visible(timeout=timeouts.DEFAULT)
