"""Saved searches (F: saved_searches) — a logged-in buyer saves the current
search on `/search` and sees it persist. Mirrors the search_facets step style:
API login (fast) + real UI assertions selected by visible Vietnamese text/role.
"""

from __future__ import annotations

import uuid

from playwright.sync_api import expect
from pytest_bdd import given, scenarios, then, when

from config.settings import get_settings
from src.constants import timeouts
from src.models import User
from src.pages.saved_searches_page import SavedSearchesPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World

SETTINGS = get_settings()

scenarios("../features/frontend/saved_searches.feature")


def _page(world: World) -> SavedSearchesPage:
    return SavedSearchesPage(world.page)


@given("a logged-in buyer viewing the search results for a fresh keyword")
def buyer_on_search_results(world: World) -> None:
    buyer = User(
        username=fake.unique_username("saved_buyer"),
        password=SETTINGS.seed_password,
        role="buyer",
    )
    login_via_api(world, buyer)
    keyword = f"zsaved{uuid.uuid4().hex[:10]}"
    world.state.extra["saved_keyword"] = keyword

    page = _page(world)
    page.navigate_query(keyword)
    world.page.wait_for_load_state("networkidle")
    # The panel only enables its save button when a query is present.
    expect(page.panel_header).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.save_button).to_be_enabled(timeout=timeouts.DEFAULT)


@then('the "Tìm kiếm đã lưu" panel shows no saved searches yet')
def panel_is_empty(world: World) -> None:
    expect(_page(world).empty_state).to_be_visible(timeout=timeouts.DEFAULT)


@when("the buyer saves the current search")
def buyer_saves_search(world: World) -> None:
    page = _page(world)
    page.save_button.click()
    # Wait for the optimistic list item (the query) to render before moving on.
    keyword = world.state.extra["saved_keyword"]
    expect(page.saved_item(keyword)).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the buyer reloads the search results page")
def buyer_reloads(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")
    world.page.wait_for_load_state("networkidle")


@then("the saved search for that keyword appears in the saved list")
def saved_search_listed(world: World) -> None:
    keyword = world.state.extra["saved_keyword"]
    page = _page(world)
    expect(page.panel_header).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.saved_item(keyword)).to_be_visible(timeout=timeouts.NAVIGATION)
    world.logger.info(f"Saved search '{keyword}' is listed in the panel")
