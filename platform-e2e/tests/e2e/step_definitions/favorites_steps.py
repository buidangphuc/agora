"""Favorites steps: favorite a search result, verify it on the favorites page."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import PageName, timeouts
from src.pages import FavoritesPage, HomePage, SearchPage
from tests.e2e.support.world import World


@when("the buyer favorites the first product in search results")
def favorite_first_result(world: World) -> None:
    home: HomePage = world.navigate_to(PageName.HOME)  # type: ignore[assignment]
    home.search_for("laptop")
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    expect(search.results.cards.first).to_be_visible(timeout=timeouts.DEFAULT)
    heart = world.page.get_by_role("button", name="Yêu thích").first
    search.wait_until_interactive(heart)
    heart.click()
    # Optimistic toggle flips the label to "Bỏ thích" at once; the button stays
    # disabled until the server action returns, and leaving the page earlier
    # cancels the save, so wait for it to be enabled again.
    favorited = world.page.get_by_role("button", name="Bỏ thích").first
    expect(favorited).to_be_visible(timeout=timeouts.DEFAULT)
    expect(favorited).to_be_enabled(timeout=timeouts.DEFAULT)


@then("the product appears in the buyer's favorites")
def product_in_favorites(world: World) -> None:
    page: FavoritesPage = world.navigate_to(PageName.FAVORITES)  # type: ignore[assignment]
    expect(page.items.first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the empty favorites placeholder is displayed")
def empty_favorites_placeholder_displayed(world: World) -> None:
    expect(world.page.get_by_text("chưa có sản phẩm yêu thích", exact=False)).to_be_visible(
        timeout=timeouts.DEFAULT
    )
