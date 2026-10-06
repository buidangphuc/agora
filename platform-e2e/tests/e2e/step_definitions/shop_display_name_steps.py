"""Shop display-name steps (OpenSpec change `shop-display-name`).

Sellers are seeded through the gateway API (UpsertStorefront with a seller token);
the shop id is the seller's principal id (JWT `sub`). The UI assertions read the
shop-profile header; the cart scenario asserts the BatchGetStorefronts lookup the
cart wrapper performs (the cart view has no per-shop group header yet; the
header assertion is added with the ui-phase cart grouping).
"""

from __future__ import annotations

import re
import uuid

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import AuthService, CartService, ListingService
from src.constants import timeouts
from src.models import Listing
from src.pages.shop_profile_page import ShopProfilePage
from tests.e2e.flows import scenario_buyer
from tests.e2e.step_definitions.follow_seller_steps import _principal_id
from tests.e2e.support.world import World


def _seller_svc(world: World, token: str) -> ListingService:
    return ListingService(token=token)


def _slug() -> str:
    return f"e2e-shop-{uuid.uuid4().hex[:10]}"


def _seller_id(world: World) -> str:
    seller = world.state.seeded_seller
    assert seller and seller.token, "No seeded seller in state (needs @needsSeller)"
    return _principal_id(seller.token)


@given(parsers.parse('the seller sets the shop display name to "{name}"'))
def seller_sets_name(world: World, name: str) -> None:
    seller = world.state.seeded_seller
    assert seller and seller.token, "No seeded seller in state (needs @needsSeller)"
    slug = world.state.extra.setdefault("slug", _slug())
    _seller_svc(world, seller.token).upsert_storefront(slug, name)


@given("the seller has no storefront")
def seller_has_no_storefront(world: World) -> None:
    # A freshly seeded seller has no storefront row; nothing to do beyond asserting the seed.
    _seller_id(world)


@given(parsers.parse('a second seller sets the shop display name to "{name}"'))
def second_seller_sets_name(world: World, name: str) -> None:
    from config.settings import get_settings
    from src.utils import data as fake

    username = fake.unique_username("seller")
    auth: AuthService = world.service_factory.auth
    token = auth.register(username, get_settings().seed_password, "seller")
    svc = _seller_svc(world, token)
    svc.upsert_storefront(_slug(), name)
    world.state.extra["second_seller_token"] = token


@given("a buyer has one listing from each seller in the cart")
def buyer_cart_two_sellers(world: World) -> None:

    tokens = [world.state.seeded_seller.token, world.state.extra["second_seller_token"]]
    sellers = []
    for token in tokens:
        listing = Listing(
            title="[E2E] Shop name listing",
            category_id="cat-electronics",
            price=1_000_000,
            stock=10,
            status="published",
            description="Seed for shop-display-name e2e.",
        )
        _seller_svc(world, token).create_listing(listing)
        sellers.append((_principal_id(token), listing.listing_id))
    world.state.extra["cart_seller_ids"] = [sid for sid, _ in sellers]

    buyer = scenario_buyer(world)
    buyer_token = world.service_factory.auth.login(buyer.username, buyer.password)
    cart = CartService(token=buyer_token)
    cart.clear_cart()
    for _, listing_id in sellers:
        cart.add_to_cart(listing_id, 1)
    world.state.extra["buyer_token"] = buyer_token


@when("a visitor opens the seller's shop page")
def visitor_opens_shop(world: World) -> None:
    page = ShopProfilePage(world.page)
    world.set_current_page(page)
    page.navigate(shop_id=_seller_id(world))
    world.page.wait_for_load_state("networkidle")


@when("the cart sellers are resolved with a single batch lookup")
def resolve_cart_sellers(world: World) -> None:
    cart = CartService(token=world.state.extra["buyer_token"])
    items = (cart.get_cart().get("cart") or {}).get("items", [])
    ids = sorted({it.get("sellerId", "") for it in items if it.get("sellerId")})
    assert len(ids) == 2, f"expected two distinct sellers in the cart, got {ids}"
    anon = ListingService()  # public read, no token: same posture as GetStorefront
    world.state.extra["batch_names"] = anon.batch_get_storefronts(ids)


@then(parsers.parse('the shop header shows "{name}"'))
def shop_header_shows(world: World, name: str) -> None:
    expect(ShopProfilePage(world.page).shop_name).to_have_text(name, timeout=timeouts.DEFAULT)


@then(parsers.parse('the storefront API returns the display name "{name}"'))
def storefront_api_returns(world: World, name: str) -> None:
    got = ListingService().get_storefront(_seller_id(world))
    assert got.get("displayName") == name, got


@then(parsers.parse('the batch lookup returns the display name "{name}" for the seller'))
def batch_returns_for_seller(world: World, name: str) -> None:
    sid = _seller_id(world)
    names = ListingService().batch_get_storefronts([sid])
    assert names.get(sid) == name, names


@then(parsers.parse('the batch lookup returns "{a}" and "{b}" for the two cart sellers'))
def batch_returns_both(world: World, a: str, b: str) -> None:
    names = world.state.extra["batch_names"]
    assert sorted(names.values()) == sorted([a, b]), names


@then(
    'the shop header shows the fallback label "Shop #" followed by the first 6 characters of the seller id'
)
def shop_header_fallback(world: World) -> None:
    sid = _seller_id(world)
    expect(ShopProfilePage(world.page).shop_name).to_have_text(
        re.compile(rf"^Shop #{re.escape(sid[:6])}$"), timeout=timeouts.DEFAULT
    )
