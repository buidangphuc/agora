"""Follow-seller steps: buyer follows/unfollows a shop and sees it under following.

The shop id is the seeded seller's principal id (the JWT `sub`), decoded from the
token captured by the `@needsSeller` seed. Reuses the login Given from `common_steps`.
"""

from __future__ import annotations

import base64
import json

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import timeouts
from src.pages.shop_follow_page import ShopFollowPage
from tests.e2e.support.world import World


def _principal_id(token: str) -> str:
    """Decode the `sub` claim (principal id) from a gateway JWT."""
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")


def _shop(world: World) -> ShopFollowPage:
    page = ShopFollowPage(world.page)
    world.set_current_page(page)
    return page


@when("the buyer opens the seeded seller's shop page")
def buyer_opens_seller_shop(world: World) -> None:
    seller = world.state.seeded_seller
    assert seller and seller.token, "No seeded seller in state (needs @needsSeller)"
    shop_id = _principal_id(seller.token)
    assert shop_id, "Could not derive seller principal id from token"
    world.state.extra["shop_id"] = shop_id
    _shop(world).open(shop_id)


@then("the shop shows a follow action")
def shop_shows_follow_action(world: World) -> None:
    shop = _shop(world)
    expect(shop.follow_toggle).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(shop.following_state).to_have_count(0)
    world.logger.info("Shop page shows the '+ Theo dõi' follow action")


@when("the buyer follows the shop")
def buyer_follows_shop(world: World) -> None:
    shop = _shop(world)
    expect(shop.follow_toggle).to_be_visible(timeout=timeouts.NAVIGATION)
    shop.toggle_follow()
    # Wait for the success toast so the FollowSeller RPC has committed server-side.
    expect(world.page.get_by_text("Đã theo dõi shop", exact=False)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("the shop shows as followed")
def shop_shows_as_followed(world: World) -> None:
    shop = _shop(world)
    expect(shop.following_state).to_be_visible(timeout=timeouts.NAVIGATION)
    world.logger.info("Shop button now reads 'Đang theo dõi'")


@when("the buyer unfollows the shop")
def buyer_unfollows_shop(world: World) -> None:
    shop = _shop(world)
    expect(shop.following_state).to_be_visible(timeout=timeouts.NAVIGATION)
    shop.toggle_follow()
    expect(world.page.get_by_text("Đã bỏ theo dõi shop", exact=False)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("the followed shop appears under the buyer's following list")
def followed_shop_in_following_list(world: World) -> None:
    shop_id = world.state.extra["shop_id"]
    world.page.goto(f"{world.settings.base_url}/account/following", wait_until="domcontentloaded")
    link = world.page.locator(f'a[href="/shop/{shop_id}"]').first
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    world.logger.info(f"Followed shop {shop_id} listed under /account/following")
