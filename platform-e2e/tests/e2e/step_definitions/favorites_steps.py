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
    heart.click()
    # Optimistic toggle flips the label to "Bỏ thích" once the action fires.
    expect(world.page.get_by_role("button", name="Bỏ thích").first).to_be_visible(
        timeout=timeouts.DEFAULT
    )


@then("the product appears in the buyer's favorites")
def product_in_favorites(world: World) -> None:
    page: FavoritesPage = world.navigate_to(PageName.FAVORITES)  # type: ignore[assignment]
    expect(page.items.first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the empty favorites placeholder is displayed")
def empty_favorites_placeholder_displayed(world: World) -> None:
    page: FavoritesPage = world.get_page(PageName.FAVORITES)  # type: ignore[assignment]
    expect(world.page.get_by_text("chưa có sản phẩm yêu thích", exact=False)).to_be_visible(
        timeout=timeouts.DEFAULT
    )

