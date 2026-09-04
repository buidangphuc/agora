"""Seller edit-listing steps (API-seed a listing, then rename it via the UI)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import PageName, timeouts
from src.models import Listing
from src.pages import SellerEditListingPage, SellerListingsPage
from src.utils import data as fake
from tests.e2e.flows import seed_listing
from tests.e2e.support.world import World


@given("the seller has a listing")
def seller_has_listing(world: World) -> None:
    seller = world.state.seeded_seller
    assert seller, "No seeded seller — scenario must be tagged @needsSeller"
    listing = Listing(title=f"[E2E] {fake.listing_title('Laptop')}", category_id="cat-laptop")
    seed_listing(world, listing, seller)


@when("the seller renames the listing")
def seller_renames_listing(world: World) -> None:
    new_title = f"[E2E] Renamed {fake.listing_title('Laptop')}"
    world.state.extra["renamed_title"] = new_title
    listing = world.state.listing
    page: SellerEditListingPage = world.navigate_to(  # type: ignore[assignment]
        PageName.SELLER_EDIT_LISTING, listing_id=listing.listing_id
    )
    expect(page.title_input).to_be_visible(timeout=timeouts.DEFAULT)
    page.update_title(new_title)


@then("the renamed listing appears in the seller's listings")
def renamed_listing_appears(world: World) -> None:
    title = world.state.extra["renamed_title"]
    page: SellerListingsPage = world.navigate_to(PageName.SELLER_LISTINGS)  # type: ignore[assignment]
    expect(page.listing_by_title(title).first).to_be_visible(timeout=timeouts.NAVIGATION)
