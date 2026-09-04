"""Seller journey steps: login (API), create a listing through the UI."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import PageName, timeouts
from src.models import Listing
from src.pages import SellerAnalyticsPage, SellerListingsPage, SellerNewListingPage
from src.utils import data as fake
from src.utils import get_test_data_manager
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World


@given("a seeded seller is logged in")
def seeded_seller_logged_in(world: World) -> None:
    # Account seeded by the @needsSeller tag hook.
    seller = world.state.seeded_seller
    assert seller, "No seeded seller — is the scenario tagged @needsSeller?"
    login_via_api(world, seller)


@when("the seller creates a new listing")
def seller_creates_listing(world: World) -> None:
    title = f"[E2E] {fake.listing_title('Laptop')}"
    world.state.extra["new_listing_title"] = title
    page: SellerNewListingPage = world.navigate_to(PageName.SELLER_NEW_LISTING)  # type: ignore[assignment]
    expect(page.title_input).to_be_visible(timeout=timeouts.DEFAULT)
    page.fill_and_submit(
        Listing(title=title, category_id="cat-laptop", price=fake.price_vnd(), stock=50)
    )


@then("the new listing appears in the seller's listings")
def new_listing_appears(world: World) -> None:
    title = world.state.extra["new_listing_title"]
    page: SellerListingsPage = world.navigate_to(PageName.SELLER_LISTINGS)  # type: ignore[assignment]
    expect(page.listing_by_title(title).first).to_be_visible(timeout=timeouts.NAVIGATION)


@given("I am logged in as a seller via API")
def logged_in_as_seller_api(world: World) -> None:
    seller = world.state.seeded_seller or get_test_data_manager().get_user_by_role("seller")
    login_via_api(world, seller)


@then("I should see the revenue metric cards")
def verify_revenue_metrics(world: World) -> None:
    analytics: SellerAnalyticsPage = world.get_page(PageName.SELLER_ANALYTICS)  # type: ignore[assignment]
    expect(analytics.revenue_metric_card).to_be_visible(timeout=timeouts.DEFAULT)


@then("I should see the seller wallet balance")
def verify_wallet_balance(world: World) -> None:
    analytics: SellerAnalyticsPage = world.get_page(PageName.SELLER_ANALYTICS)  # type: ignore[assignment]
    expect(analytics.wallet_balance_text).to_be_visible(timeout=timeouts.DEFAULT)
