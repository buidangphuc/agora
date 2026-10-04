"""Storefront steps for ui-phase-product-detail: shared header card and ?sort=.

Adds to shop_storefront.feature without touching shop_storefront_steps.py; the
display-name and follow steps are reused from their own modules.
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import ListingService
from src.constants import timeouts
from src.models import Listing
from src.utils import data as fake
from tests.e2e.support.world import World


def _seller_token(world: World) -> str:
    seller = world.state.seeded_seller
    assert seller and seller.token, "No seeded seller in state (needs @needsSeller)"
    return seller.token


@given(parsers.parse("the seller has published listings priced {high:d} and {low:d}"))
def seller_has_two_priced_listings(world: World, high: int, low: int) -> None:
    svc = ListingService(token=_seller_token(world))
    ids: dict[str, str] = {}
    for label, price in (("high", high), ("low", low)):
        listing = Listing(
            title=f"[E2E][Shop sort {label}] {fake.price_vnd():d}",
            price=price,
            stock=10,
            status="published",
            description="Sản phẩm seed cho thứ tự giá của gian hàng.",
        )
        ids[label] = svc.create_listing(listing)
    world.state.extra["shop_sort_ids"] = ids


@then(parsers.parse('the shared shop header card shows "{name}"'))
def shared_header_card_shows(world: World, name: str) -> None:
    card = world.page.get_by_test_id("shop-header-card")
    expect(card).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(card.get_by_test_id("shop-name")).to_have_text(name, timeout=timeouts.DEFAULT)
    # Real data only: the rating state is shown, never an invented 99% / Mall.
    expect(card.get_by_test_id("shop-rating-summary")).to_be_visible()
    expect(card).not_to_contain_text("Tỉ Lệ Phản Hồi")


@when(parsers.parse('the buyer sorts the shop by "{label}"'))
def sort_shop(world: World, label: str) -> None:
    world.page.get_by_role("link", name=label).click()


def _ordered_ids(world: World) -> list[str]:
    hrefs = world.page.locator('main a[href^="/listing/"]').evaluate_all(
        "els => els.map(e => e.getAttribute('href') || '')"
    )
    seen: list[str] = []
    for href in hrefs:
        listing_id = href.rsplit("/", 1)[-1]
        if listing_id not in seen:
            seen.append(listing_id)
    return seen


def _assert_cheaper_first(world: World) -> None:
    ids = world.state.extra["shop_sort_ids"]
    order = _ordered_ids(world)
    assert ids["low"] in order and ids["high"] in order, f"seeded listings missing from {order}"
    assert order.index(ids["low"]) < order.index(
        ids["high"]
    ), f"expected ascending price order, got {order}"


@then("the URL contains sort=price_asc and the cheaper listing comes first")
def url_has_sort_and_ascending(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*[?&]sort=price_asc"), timeout=timeouts.DEFAULT)
    _assert_cheaper_first(world)


@then("reloading the shop keeps the ascending price order")
def reload_keeps_sort(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")
    expect(world.page).to_have_url(re.compile(r".*[?&]sort=price_asc"))
    _assert_cheaper_first(world)
