"""Steps for frontend/search_rating_removed.feature (changes port-search-read-model-correctness, frontend-discovery-sort-rating)."""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.pages import SearchPage
from tests.e2e.step_definitions.search_best_selling_rating_steps import (
    at,
    create_indexed,
    send_rating,
    wait_hit,
)
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World


def _search(world: World) -> SearchPage:
    return world.get_page(PageName.SEARCH)  # type: ignore[return-value]


def _open(world: World, query: str) -> SearchPage:
    search = _search(world)
    search.navigate_query(query)
    search.page.wait_for_load_state("networkidle")
    return search


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
    offending = search.legacy_rating_hrefs()
    assert not offending, f"links carry rating=: {offending}"


_PHONE = {"width": 375, "height": 812}


def _open_rating(world: World, query: str = "", *, phone: bool = False) -> SearchPage:
    c = s.ctx(world)
    search = _search(world)
    if phone:
        world.page.set_viewport_size(_PHONE)
    suffix = f"&{query}" if query else ""
    world.page.goto(f"{search.url()}?q={c.keyword}{suffix}", wait_until="domcontentloaded")
    world.page.wait_for_load_state("networkidle")
    return search


def _ids(world: World, labels: list[str]) -> set[str]:
    return {f"/listing/{s.listing(world, label).id}" for label in labels}


@given(
    parsers.parse(
        'published listings "A" and "C" matching one keyword with rating_avg {a:g} and {c:g}'
    )
)
def two_low_rated(world: World, a: float, c: float) -> None:
    for label in ("A", "C"):
        create_indexed(world, label)
    for label, avg in (("A", a), ("C", c)):
        send_rating(world, s.listing(world, label).id, avg, 1, at(1))
    for label, avg in (("A", a), ("C", c)):
        wait_hit(world, label, ratingAvg=avg, reviewCount=1)


@when("a buyer opens the search results for the rating keyword")
def open_plain(world: World) -> None:
    _open_rating(world)


@when(parsers.parse("a buyer opens the search results for the rating keyword with {param}"))
def open_with_param(world: World, param: str) -> None:
    phone = param.endswith(" on a 375 px wide viewport")
    param = param.removesuffix(" on a 375 px wide viewport")
    _open_rating(world, param.replace(" and ", "&"), phone=phone)


@when("a buyer opens the search results for the rating keyword on a 375 px wide viewport")
def open_plain_phone(world: World) -> None:
    _open_rating(world, phone=True)


@then(
    'the filter sidebar shows a rating group with "Từ 4 sao" counted 1 and "Từ 3 sao" counted 2, '
    "and no link contains rating="
)
def sidebar_with_rating(world: World) -> None:
    search = _search(world)
    search.wait_until_interactive(search.sort_link("Mới nhất"))
    group = search.rating_group(search.facets_sidebar)
    expect(group).to_be_visible(timeout=timeouts.DEFAULT)
    four = search.rating_option("Từ 4 sao", search.facets_sidebar)
    three = search.rating_option("Từ 3 sao", search.facets_sidebar)
    expect(four).to_be_visible(timeout=timeouts.DEFAULT)
    expect(three).to_be_visible(timeout=timeouts.DEFAULT)
    assert (search.bucket_count(four), search.bucket_count(three)) == (1, 2)
    offending = search.legacy_rating_hrefs()
    assert not offending, f"links carry rating=: {offending}"


@when(parsers.parse('the buyer selects "{label}" in the rating group'))
def select_rating(world: World, label: str) -> None:
    search = _search(world)
    option = search.rating_option(label, search.facets_sidebar)
    expect(option).to_be_visible(timeout=timeouts.DEFAULT)
    search.wait_until_interactive(option)
    option.click()


@then(parsers.parse('the URL contains minRating={n:d} and the results list only "{label}"'))
def url_has_rating_only(world: World, n: int, label: str) -> None:
    search = _search(world)
    expect(world.page).to_have_url(re.compile(rf".*minRating={n}"), timeout=timeouts.NAVIGATION)
    expect(search.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)
    search.wait_for_result_count(1, timeout=timeouts.NAVIGATION)
    assert set(search.result_hrefs()) == _ids(world, [label]), search.result_hrefs()


@then(parsers.parse('"{label}" is marked selected'))
def option_selected(world: World, label: str) -> None:
    search = _search(world)
    expect(search.rating_option(label, search.facets_sidebar)).to_have_attribute(
        "data-active", "true", timeout=timeouts.DEFAULT
    )


@then("opening the same URL in a new tab shows the same results and selection")
def same_rating_in_new_tab(world: World) -> None:
    search = _search(world)
    expected = search.result_hrefs()
    other = world.context.new_page()
    try:
        other.goto(world.page.url, wait_until="domcontentloaded")
        expect(other.get_by_test_id("search-results")).to_be_visible(timeout=timeouts.NAVIGATION)
        assert sorted(search.ordered_result_hrefs(other)) == expected
        selected = (
            other.get_by_test_id("facet-ratings")
            .get_by_test_id("facet-bucket")
            .filter(has_text="Từ 4 sao")
        )
        expect(selected).to_have_attribute("data-active", "true", timeout=timeouts.NAVIGATION)
    finally:
        other.close()


@then(
    'the rating group still shows "Từ 4 sao" and "Từ 3 sao" with the "Từ 3 sao" count equal to 2'
)
def group_keeps_options(world: World) -> None:
    search = _search(world)
    search.wait_until_interactive(search.sort_link("Mới nhất"))
    four = search.rating_option("Từ 4 sao", search.facets_sidebar)
    three = search.rating_option("Từ 3 sao", search.facets_sidebar)
    expect(four).to_be_visible(timeout=timeouts.DEFAULT)
    expect(three).to_be_visible(timeout=timeouts.DEFAULT)
    assert search.bucket_count(three) == 2, three.inner_text()


@then(parsers.parse("the drawer trigger shows {n:d} active filter"))
@then(parsers.parse("the drawer trigger shows {n:d} active filters"))
def drawer_count(world: World, n: int) -> None:
    search = _search(world)
    expect(search.filter_button).to_be_visible(timeout=timeouts.NAVIGATION)
    world.page.wait_for_function(
        "n => { const b = Array.from(document.querySelectorAll('button')).find("
        "x => /^Bộ lọc/.test(x.innerText.trim() || x.getAttribute('aria-label') || ''));"
        " if (!b) return false; const d = b.innerText.match(/\\d+/);"
        " return (d ? parseInt(d[d.length - 1], 10) : 0) === n; }",
        arg=n,
        timeout=timeouts.NAVIGATION,
    )
    assert search.filter_count == n


@when("the buyer removes the rating chip")
def remove_rating_chip(world: World) -> None:
    search = _search(world)
    expect(search.rating_chip).to_have_count(1, timeout=timeouts.DEFAULT)
    search.wait_until_interactive(search.rating_chip)
    search.rating_chip.click()


@then(parsers.parse('the URL no longer carries minRating and the results include "{label}"'))
def url_without_rating(world: World, label: str) -> None:
    search = _search(world)
    expect(world.page).not_to_have_url(re.compile(r".*minRating="), timeout=timeouts.NAVIGATION)
    expect(search.rating_chip).to_have_count(0, timeout=timeouts.DEFAULT)
    search.wait_for_result_count(4, timeout=timeouts.NAVIGATION)
    assert _ids(world, [label]) <= set(search.result_hrefs()), search.result_hrefs()


@when('the buyer opens the filter drawer and selects "Từ 3 sao"')
def drawer_select_three(world: World) -> None:
    search = _search(world)
    search.wait_until_interactive(search.filter_button)
    search.filter_button.click()
    expect(search.filter_drawer).to_be_visible(timeout=timeouts.DEFAULT)
    option = search.rating_option("Từ 3 sao", search.filter_drawer)
    expect(option).to_be_visible(timeout=timeouts.DEFAULT)
    option.click()


@then(
    parsers.parse(
        'the URL contains minRating={n:d} and the results list "{a}" and "{b}" but not "{c}" or "{d}"'
    )
)
def url_has_rating_listing(world: World, n: int, a: str, b: str, c: str, d: str) -> None:
    search = _search(world)
    expect(world.page).to_have_url(re.compile(rf".*minRating={n}"), timeout=timeouts.NAVIGATION)
    search.wait_for_result_count(2, timeout=timeouts.NAVIGATION)
    assert set(search.result_hrefs()) == _ids(world, [a, b]), search.result_hrefs()


@when(
    "a buyer opens the search for the rating keyword with minRating=9, with minRating=abc and "
    "without it"
)
def open_unsupported(world: World) -> None:
    c = s.ctx(world)
    plain = _open_rating(world)
    expect(plain.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)
    plain.wait_for_result_count(4, timeout=timeouts.NAVIGATION)
    c.extra["plain_hrefs"] = plain.result_hrefs()
    seen = []
    for value in ("9", "abc"):
        _open_rating(world, f"minRating={value}")
        expect(plain.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)
        plain.wait_until_interactive(plain.sort_link("Mới nhất"))
        seen.append(
            (
                value,
                plain.result_hrefs(),
                plain.error_alert.count(),
                plain.empty_state.count(),
                plain.rating_chip.count(),
            )
        )
    c.extra["unsupported"] = seen


@then(
    "each page lists the same results as the plain one, with no error, empty state or rating chip"
)
def unsupported_ignored(world: World) -> None:
    c = s.ctx(world)
    for value, hrefs, errors, empties, chips in c.extra["unsupported"]:
        assert hrefs == c.extra["plain_hrefs"], (value, hrefs, c.extra["plain_hrefs"])
        assert (errors, empties, chips) == (0, 0, 0), (value, errors, empties, chips)


@then(
    'one rating chip is shown for "Từ 4 sao", the results list only "A" and no error is shown'
)
def repeated_first_wins(world: World) -> None:
    search = _search(world)
    expect(search.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)
    search.wait_for_result_count(1, timeout=timeouts.NAVIGATION)
    assert set(search.result_hrefs()) == _ids(world, ["A"]), search.result_hrefs()
    expect(search.rating_chip).to_have_count(1)
    expect(search.rating_chip).to_contain_text("4")
    expect(search.error_alert).to_have_count(0)


@then("the search empty state is shown with the rating chip and zero results, and no error")
def rating_empty_state(world: World) -> None:
    search = _search(world)
    expect(search.empty_state).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(search.rating_chip).to_have_count(1)
    assert search.result_count() == 0
    expect(search.error_alert).to_have_count(0)
