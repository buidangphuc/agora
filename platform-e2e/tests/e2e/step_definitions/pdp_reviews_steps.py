"""Steps for the URL-driven review filter on the PDP (ui-phase-product-detail).

Extends review_ratings_filter.feature without touching the existing bindings in
review_ratings_steps.py; it reuses that module's seeding helpers.
"""

from __future__ import annotations

import re
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.pages import ListingDetailPage
from tests.e2e.flows import login_via_api
from tests.e2e.step_definitions.review_ratings_steps import (
    _create_review,
    _register_buyer,
    _seed_seller_and_listing,
)
from tests.e2e.support.world import World

# One review per star value 5, 4, 4, 3: none is 2 stars (the empty-filter case).
_SEEDED_STARS = (5, 4, 4, 3)


def _detail(world: World) -> ListingDetailPage:
    return world.get_page(PageName.LISTING_DETAIL)  # type: ignore[return-value]


def _rating_param(url: str) -> str | None:
    values = parse_qs(urlparse(url).query).get("rating")
    return values[0] if values else None


def _listed_ratings(world: World) -> list[int]:
    """Star value of every listed review (from the read-only Rate's accessible name)."""
    labels = (
        world.page.get_by_test_id("review-item")
        .get_by_role("img")
        .evaluate_all("els => els.map(e => e.getAttribute('aria-label') || '')")
    )
    return [int(m.group(1)) for label in labels if (m := re.match(r"(\d+) trên 5 sao", label))]


@given("a buyer is viewing a listing with reviews of several star values")
def buyer_viewing_listing_with_reviews(world: World) -> None:
    _seller, listing = _seed_seller_and_listing(world, "pdpfilter")
    for i, stars in enumerate(_SEEDED_STARS):
        author = _register_buyer(world, f"pdpfilter{i}")
        _create_review(
            world,
            author.token,
            listing.listing_id,
            rating=stars,
            comment=f"Nhận xét {stars} sao #{i}",
        )
    viewer = _register_buyer(world, "pdpfilter_view")
    login_via_api(world, viewer)
    world.state.current_user = viewer
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=listing.listing_id)
    expect(_detail(world).review_items.first).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the buyer clicks the "{label}" review filter'))
def click_review_filter(world: World, label: str) -> None:
    _detail(world).review_filter_buttons.filter(has_text=label).first.click()


@when(parsers.parse("the buyer opens the reviews filtered by {stars:d} stars"))
def open_filtered_reviews(world: World, stars: int) -> None:
    listing = world.state.listing
    assert listing and listing.listing_id
    url = f"{world.settings.base_url.rstrip('/')}/listing/{listing.listing_id}?rating={stars}"
    world.page.goto(url, wait_until="domcontentloaded")


@then("the URL contains rating=4 and only 4-star reviews are listed")
def url_has_rating_and_only_four_star(world: World) -> None:
    world.page.wait_for_function(
        "() => new URL(location.href).searchParams.get('rating') === '4'",
        timeout=timeouts.DEFAULT,
    )
    expect(_detail(world).review_items).to_have_count(2, timeout=timeouts.DEFAULT)
    assert set(_listed_ratings(world)) == {4}, _listed_ratings(world)


@then("reloading the page shows the same filtered list")
def reload_keeps_filter(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")
    assert _rating_param(world.page.url) == "4"
    expect(_detail(world).review_items).to_have_count(2, timeout=timeouts.NAVIGATION)
    assert set(_listed_ratings(world)) == {4}, _listed_ratings(world)


@then(parsers.parse('an empty state "{text}" is shown'))
def reviews_empty_state(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the buyer follows "{label}" in the reviews empty state'))
def follow_empty_state_action(world: World, label: str) -> None:
    world.page.locator("#reviews").get_by_role("link", name=label).click()


@then("the URL no longer contains a rating filter")
def url_without_rating(world: World) -> None:
    world.page.wait_for_function(
        "() => !new URL(location.href).searchParams.has('rating')",
        timeout=timeouts.DEFAULT,
    )
    expect(_detail(world).review_items.first).to_be_visible(timeout=timeouts.DEFAULT)
