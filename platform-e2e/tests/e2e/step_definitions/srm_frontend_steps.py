"""Steps for frontend/search_rating_removed.feature (change port-search-read-model-correctness)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import PageName, timeouts
from src.pages import SearchPage
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World


def _search(world: World) -> SearchPage:
    return world.get_page(PageName.SEARCH)  # type: ignore[return-value]


def _open(world: World, query: str) -> SearchPage:
    search = _search(world)
    search.navigate_query(query)
    search.page.wait_for_load_state("networkidle")
    return search


@then(
    "the filter sidebar shows the category and price groups and no rating filter, and no sort "
    "or pagination link contains rating="
)
def sidebar_without_rating(world: World) -> None:
    search = _search(world)
    expect(search.facet_group("categories")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(search.facet_group("price_ranges")).to_be_visible(timeout=timeouts.DEFAULT)
    search.wait_until_interactive(search.sort_link("Mới nhất"))
    expect(search.facet_group("ratings")).to_have_count(0)
    sidebar_text = search.facets_sidebar.inner_text()
    assert "Đánh giá" not in sidebar_text, f"rating group rendered: {sidebar_text!r}"
    offending = search.hrefs_containing("rating=")
    assert not offending, f"links carry rating=: {offending}"


@given("a published listing whose title carries a unique keyword is indexed for the rating link")
def keyword_listing(world: World) -> None:
    listed = s.create_listing(world, "R", stock=5, title_suffix="ratinglink")
    s.wait_indexed(world, "R")
    s.ctx(world).extra["query"] = s.ctx(world).keyword
    assert listed.id


@when("a buyer opens the search for that keyword with rating=4 and without it")
def open_both(world: World) -> None:
    c = s.ctx(world)
    plain = _open(world, c.keyword)
    expect(plain.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)
    c.extra["plain_hrefs"] = plain.result_hrefs()
    world.page.goto(f"{plain.url()}?q={c.keyword}&rating=4", wait_until="domcontentloaded")
    world.page.wait_for_load_state("networkidle")


@then(
    "the rating link lists that listing exactly as the plain one does, with no error, empty "
    "state or rating chip, and its sort links do not contain rating="
)
def rating_link_same_as_plain(world: World) -> None:
    c = s.ctx(world)
    search = _search(world)
    expect(search.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)
    search.wait_until_interactive(search.sort_link("Mới nhất"))
    listed = s.listing(world, "R")
    hrefs = search.result_hrefs()
    assert hrefs == c.extra["plain_hrefs"], (hrefs, c.extra["plain_hrefs"])
    assert f"/listing/{listed.id}" in hrefs, hrefs
    expect(search.error_alert).to_have_count(0)
    expect(search.empty_state).to_have_count(0)
    expect(search.rating_chip).to_have_count(0)
    offending = search.hrefs_containing("rating=")
    assert not offending, f"links carry rating=: {offending}"
