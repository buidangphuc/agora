"""Presentational shell steps (home landing + global header)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import PageName, timeouts
from src.pages import HomePage
from tests.e2e.support.world import World


@then("the home landing shows the category bar and products")
def home_landing_renders(world: World) -> None:
    home: HomePage = world.get_page(PageName.HOME)  # type: ignore[assignment]
    expect(home.category_heading.first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(home.listing_links.first).to_be_visible(timeout=timeouts.DEFAULT)


@then("the global header shows search and cart")
def header_shows_search_and_cart(world: World) -> None:
    home: HomePage = world.get_page(PageName.HOME)  # type: ignore[assignment]
    expect(home.header.search_input).to_be_visible(timeout=timeouts.DEFAULT)
    expect(home.cart_link.first).to_be_visible(timeout=timeouts.DEFAULT)


@then("the notification center is displayed")
def notification_center_displayed(world: World) -> None:
    from src.pages import NotificationsPage

    page: NotificationsPage = world.get_page(PageName.NOTIFICATIONS)  # type: ignore[assignment]
    expect(page.heading).to_be_visible(timeout=timeouts.DEFAULT)


@then("the chat page is displayed")
def chat_page_displayed(world: World) -> None:
    from src.pages import ChatPage

    page: ChatPage = world.get_page(PageName.CHAT)  # type: ignore[assignment]
    expect(page.heading).to_be_visible(timeout=timeouts.DEFAULT)


@when("the buyer types a partial search keyword")
def type_partial_keyword(world: World) -> None:
    home: HomePage = world.get_page(PageName.HOME)  # type: ignore[assignment]
    home.header.search_input.fill("lap")


@then("a search suggestion appears")
def search_suggestion_appears(world: World) -> None:
    expect(world.page.locator("ul li").first).to_be_visible(timeout=timeouts.DEFAULT)
