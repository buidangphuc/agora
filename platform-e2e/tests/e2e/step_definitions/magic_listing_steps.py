"""AI magic-listing steps (client-side AI fill on the new-listing form)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import PageName, timeouts
from src.pages import SellerNewListingPage
from tests.e2e.support.world import World


@when("the seller opens the new listing form")
def open_new_listing_form(world: World) -> None:
    page: SellerNewListingPage = world.navigate_to(PageName.SELLER_NEW_LISTING)  # type: ignore[assignment]
    expect(page.title_input).to_be_visible(timeout=timeouts.DEFAULT)


@when("the seller clicks the AI generate button")
def click_ai_generate(world: World) -> None:
    # Magic Listing needs a typed title (it never invents one) and shows a
    # suggestion card; the form changes only when the seller applies it.
    page: SellerNewListingPage = world.get_page(PageName.SELLER_NEW_LISTING)  # type: ignore[assignment]
    page.title_input.fill("Laptop Dell XPS 13 9340 Chính Hãng")
    page.magic_generate.click()
    expect(page.magic_suggestion).to_be_visible(timeout=timeouts.LONG)


@then("the description field is filled by AI")
def description_filled(world: World) -> None:
    description = world.page.locator("#description")
    expect(description).not_to_have_value("", timeout=timeouts.DEFAULT)
