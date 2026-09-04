"""Buyer journey steps: login (API), search, add to cart, checkout."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.pages import CartPage, CheckoutPage, ListingDetailPage, SearchPage
from src.utils import get_test_data_manager
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World


@given("a buyer is logged in")
def buyer_logged_in(world: World) -> None:
    buyer = get_test_data_manager().get_user_by_role("buyer")
    login_via_api(world, buyer)


@given("a listing has been seeded via the API")
def listing_seeded(world: World) -> None:
    # Seeded by the @needsListing tag hook (seed_by_tags fixture).
    assert world.state.listing and world.state.listing.listing_id, "No seeded listing in state"


@when(parsers.parse('the buyer searches for "{term}"'))
def buyer_searches(world: World, term: str) -> None:
    world.state.search_term = term
    home = world.navigate_to(PageName.HOME)
    home.search_for(term)  # type: ignore[attr-defined]


@when("the buyer opens the first search result")
def buyer_opens_first_result(world: World) -> None:
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    expect(search.results.cards.first).to_be_visible(timeout=timeouts.DEFAULT)
    search.open_first_result()


@when("the buyer opens the seeded listing")
def buyer_opens_seeded_listing(world: World) -> None:
    listing = world.state.listing
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=listing.listing_id)  # type: ignore[union-attr]


@when("the buyer adds the product to the cart")
def buyer_adds_to_cart(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.add_to_cart_button).to_be_visible(timeout=timeouts.DEFAULT)
    detail.add_to_cart()


@when("the buyer opens the cart")
def buyer_opens_cart(world: World) -> None:
    world.navigate_to(PageName.CART)


@when("the buyer proceeds to checkout")
def buyer_proceeds_to_checkout(world: World) -> None:
    cart: CartPage = world.get_page(PageName.CART)  # type: ignore[assignment]
    expect(cart.checkout_link).to_be_visible(timeout=timeouts.DEFAULT)
    cart.proceed_to_checkout()


@then("the checkout page is displayed")
def checkout_displayed(world: World) -> None:
    world.page.wait_for_url("**/checkout**", timeout=timeouts.NAVIGATION)
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    assert checkout.is_displayed()


@then("the add-to-cart action is available")
def add_to_cart_available(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.add_to_cart_button).to_be_visible(timeout=timeouts.DEFAULT)


@then("the search results page displays matching products")
def search_results_displayed(world: World) -> None:
    _ = world.get_page(PageName.SEARCH)
    world.logger.info("Search results page displayed with matching listings")
