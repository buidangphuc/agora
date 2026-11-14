"""Steps of frontend/shop_storefront_ui.feature (change shop-page-featured-theme).

Black box: storefronts, listings and sellers are seeded through the gateway (reusing the
storefront-customisation seeding steps); the browser reads /shop/{id}.
"""

from __future__ import annotations

import time

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import ListingService
from src.constants import timeouts
from tests.e2e.step_definitions.storefront_customisation_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.storefront_customisation_steps import (
    _create_listing,
    _ids,
    _save,
    _seller_id,
)
from tests.e2e.support.world import World

_INDEX_WAIT_S = 45
_CARDS = 'a[href^="/listing/"]'
_ERROR_TEXTS = ("Something went wrong", "Đã xảy ra lỗi", "Application error")


def _hrefs(world: World, selector: str) -> list[str]:
    return world.page.eval_on_selector_all(selector, "els => els.map(e => e.getAttribute('href'))")


def _featured_hrefs(world: World) -> list[str]:
    return _hrefs(world, f'[data-testid="featured-listings"] {_CARDS}')


def _grid_hrefs(world: World) -> list[str]:
    return world.page.evaluate(
        """() => Array.from(document.querySelectorAll('a[href^="/listing/"]'))
            .filter(a => !a.closest('[data-testid="featured-listings"]'))
            .map(a => a.getAttribute('href'))"""
    )


def _href(world: World, letter: str) -> str:
    return f"/listing/{_ids(world, letter)[0]}"


def _goto(world: World, seller_id: str) -> None:
    resp = world.page.goto(f"{world.settings.base_url}/shop/{seller_id}")
    world.state.extra["shop_status"] = resp.status if resp else None
    world.page.wait_for_load_state("networkidle")


@given(parsers.parse('the seller sets the storefront theme to "{theme}"'))
def seller_sets_theme(world: World, theme: str) -> None:
    _save(world, theme=theme)


@given("a second seller with a default theme storefront")
def second_seller_default_theme(world: World) -> None:
    from config.settings import get_settings
    from src.utils import data as fake
    from tests.e2e.step_definitions.follow_seller_steps import _principal_id

    token = world.service_factory.auth.register(
        fake.unique_username("seller"), get_settings().seed_password, "seller"
    )
    _create_listing(token, "[E2E] Default theme listing")
    ListingService(token=token).upsert_full_storefront(
        {"slug": f"e2e-sf-{fake.unique_username('s')[:12]}", "theme": "default"}
    )
    world.state.extra["second_shop_id"] = _principal_id(token)


@when("a visitor opens the shop page")
def visitor_opens_shop_page(world: World) -> None:
    _goto(world, _seller_id(world))


@when(parsers.parse('a visitor opens the shop page once the shop grid lists listing "{letter}"'))
def visitor_opens_shop_when_indexed(world: World, letter: str) -> None:
    deadline = time.monotonic() + _INDEX_WAIT_S
    while True:
        _goto(world, _seller_id(world))
        if _href(world, letter) in _grid_hrefs(world) + _featured_hrefs(world):
            return
        assert time.monotonic() < deadline, f"listing {letter} never showed on the shop page"
        time.sleep(3)


@when("a visitor opens the second seller's shop page")
def visitor_opens_second_shop(world: World) -> None:
    _goto(world, world.state.extra["second_shop_id"])


def _header_bg(world: World) -> str:
    card = world.page.get_by_test_id("shop-header-card")
    expect(card).to_be_visible(timeout=timeouts.NAVIGATION)
    return card.evaluate(
        """el => {
            const t = el.querySelector('[data-testid="shop-header-strip"]') || el;
            return getComputedStyle(t).backgroundColor + '|' + getComputedStyle(el).backgroundColor;
        }"""
    )


@then(parsers.parse('the featured block shows the listings "{order}" in that order'))
def featured_block_order(world: World, order: str) -> None:
    expect(world.page.get_by_test_id("featured-listings")).to_be_visible(timeout=timeouts.DEFAULT)
    got = list(dict.fromkeys(_featured_hrefs(world)))
    assert got == [f"/listing/{i}" for i in _ids(world, order)], got


@then(parsers.parse('the main grid contains none of the listings "{letters}"'))
def grid_excludes(world: World, letters: str) -> None:
    grid = _grid_hrefs(world)
    assert grid, "the main grid is empty"
    for i in _ids(world, letters):
        assert f"/listing/{i}" not in grid, grid


@then(parsers.parse('the main grid contains all of the listings "{letters}"'))
def grid_includes(world: World, letters: str) -> None:
    grid = _grid_hrefs(world)
    for i in _ids(world, letters):
        assert f"/listing/{i}" in grid, grid


@then("the shop page responded 200 without error text")
def page_ok(world: World) -> None:
    assert world.state.extra["shop_status"] == 200
    body = world.page.locator("body").inner_text()
    for text in _ERROR_TEXTS:
        assert text not in body, text


@then(parsers.parse('listing "{letter}" appears nowhere on the shop page'))
def listing_nowhere(world: World, letter: str) -> None:
    lid = _ids(world, letter)[0]
    assert lid not in world.page.content()


@then("the shop page has no featured block")
def no_featured_block(world: World) -> None:
    expect(world.page.get_by_test_id("shop-header-card")).to_be_visible(timeout=timeouts.NAVIGATION)
    assert world.page.get_by_test_id("featured-listings").count() == 0


@then(parsers.parse('the shop header card has data-theme "{theme}"'))
def header_theme(world: World, theme: str) -> None:
    card = world.page.get_by_test_id("shop-header-card")
    expect(card).to_have_attribute("data-theme", theme, timeout=timeouts.DEFAULT)
    world.state.extra.setdefault("header_bgs", []).append(_header_bg(world))


@then("the two shop header backgrounds differ")
def backgrounds_differ(world: World) -> None:
    first, second = world.state.extra["header_bgs"][:2]
    assert first != second, first
