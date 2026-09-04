from __future__ import annotations

import re
import time
import uuid

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import BaseService
from src.constants import PageName, timeouts
from src.constants import gateway_endpoints as ep
from src.models import Listing, User
from src.pages import SearchPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World

SETTINGS = get_settings()


@then("the search results sort bar displays the sorting controls")
def search_sort_bar_visible(world: World) -> None:
    expect(world.page.locator("body")).to_contain_text("Sắp xếp theo:", timeout=timeouts.DEFAULT)
    expect(world.page.get_by_role("button", name="Mới Nhất")).to_be_visible(
        timeout=timeouts.DEFAULT
    )


@when("the buyer sorts search results by newest")
def buyer_sorts_by_newest(world: World) -> None:
    newest_btn = world.page.get_by_role("button", name="Mới Nhất")
    newest_btn.click()
    world.page.wait_for_load_state("networkidle")


@then("the search URL contains the selected sort parameter")
def search_url_has_sort(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*sort=newest.*"), timeout=timeouts.DEFAULT)


# ── Faceted search (F2) ──────────────────────────────────────────────────
# Deterministic seed: prices are chosen to land in distinct backend price-range
# facet buckets (0-100000 / 100000-500000 / 500000-1000000) across two categories,
# so bucket counts and the narrowing behaviour are exactly predictable.
_FACET_SEED: list[tuple[int, str]] = [
    (50_000, "cat-electronics"),  # 0-100000
    (60_000, "cat-electronics"),  # 0-100000
    (250_000, "cat-electronics"),  # 100000-500000
    (300_000, "cat-laptop"),  # 100000-500000
    (400_000, "cat-laptop"),  # 100000-500000
    (700_000, "cat-laptop"),  # 500000-1000000
]
_FACET_TOTAL = len(_FACET_SEED)  # 6
_LOW_BUCKET_KEY = "0-100000"
_LOW_BUCKET_COUNT = 2
_MID_BUCKET_KEY = "100000-500000"
_MID_BUCKET_COUNT = 3
_INDEX_WAIT_SECONDS = 45


@given("seeded listings across categories and price ranges are indexed")
def seed_faceted_listings(world: World) -> None:
    """Seed published listings via the gateway (seller principal), then block until
    the CQRS indexer has consumed listing.events into the OpenSearch read-model."""
    sf = world.service_factory
    keyword = f"zfacet{uuid.uuid4().hex[:10]}"

    seller_name = fake.unique_username("facet_seller")
    seller_token = sf.auth.register(seller_name, SETTINGS.seed_password, "seller")
    sf.set_token(seller_token)
    for i, (price, category_id) in enumerate(_FACET_SEED):
        sf.listing.create_listing(
            Listing(
                title=f"{keyword} phòng {i}",
                category_id=category_id,
                price=price,
                stock=100,
                status="published",
                description="Sản phẩm seed tự động cho Faceted Search E2E.",
            )
        )
    world.state.seeded_seller = User(
        username=seller_name, password=SETTINGS.seed_password, role="seller", token=seller_token
    )
    world.state.extra["facet_keyword"] = keyword

    # Poll the public read-model until every seeded listing is indexed (bounded).
    read_model = BaseService(token=None)
    hits = 0
    deadline = time.time() + _INDEX_WAIT_SECONDS
    try:
        while time.time() < deadline:
            resp = read_model.post(ep.SEARCH_LISTINGS, {"query": keyword})
            hits = len(resp.get("hits") or [])
            if hits >= _FACET_TOTAL:
                break
            time.sleep(2)
    finally:
        read_model.close()
    assert hits >= _FACET_TOTAL, (
        f"only {hits}/{_FACET_TOTAL} seeded listings indexed for {keyword!r} "
        f"within {_INDEX_WAIT_SECONDS}s"
    )

    # Open the results page as a buyer principal.
    buyer = User(
        username=fake.unique_username("facet_buyer"), password=SETTINGS.seed_password, role="buyer"
    )
    login_via_api(world, buyer)
    world.logger.info(f"Seeded + indexed {_FACET_TOTAL} listings for keyword {keyword!r}")


@when("the buyer opens the search results for the seeded listings")
def buyer_opens_faceted_search(world: World) -> None:
    keyword = world.state.extra["facet_keyword"]
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    search.navigate_query(keyword)
    search.page.wait_for_load_state("networkidle")
    expect(search.facets_sidebar).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the facet sidebar shows category and price buckets with counts")
def facet_buckets_render_with_counts(world: World) -> None:
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    expect(search.facet_group("categories")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(search.facet_group("price_ranges")).to_be_visible(timeout=timeouts.DEFAULT)

    # Price buckets carry the deterministic counts from the seed.
    low = search.facet_bucket(_LOW_BUCKET_KEY)
    mid = search.facet_bucket(_MID_BUCKET_KEY)
    expect(low).to_be_visible(timeout=timeouts.DEFAULT)
    expect(mid).to_be_visible(timeout=timeouts.DEFAULT)
    assert search.bucket_count(low) == _LOW_BUCKET_COUNT, (
        f"'{_LOW_BUCKET_KEY}' count = {search.bucket_count(low)}, expected {_LOW_BUCKET_COUNT}"
    )
    assert search.bucket_count(mid) == _MID_BUCKET_COUNT, (
        f"'{_MID_BUCKET_KEY}' count = {search.bucket_count(mid)}, expected {_MID_BUCKET_COUNT}"
    )

    # Every category bucket is a positive count and they sum to the seeded total.
    category_buckets = search.facet_group("categories").get_by_test_id("facet-bucket")
    n = category_buckets.count()
    assert n >= 2, f"expected >= 2 category buckets, got {n}"
    counts = [search.bucket_count(category_buckets.nth(i)) for i in range(n)]
    assert all(c > 0 for c in counts), f"a category bucket had a non-positive count: {counts}"
    assert sum(counts) == _FACET_TOTAL, f"category counts {counts} sum != {_FACET_TOTAL}"
    world.logger.info(f"Facets rendered: price {counts=} categories sum {sum(counts)}")


@when(parsers.parse('the buyer selects the "{price_key}" price-range facet'))
def buyer_selects_price_facet(world: World, price_key: str) -> None:
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    world.state.extra["results_before"] = search.result_count()
    world.state.extra["low_count_before"] = search.bucket_count(
        search.facet_bucket(_LOW_BUCKET_KEY)
    )
    world.state.extra["selected_price_key"] = price_key
    bucket = search.facet_bucket(price_key)
    expect(bucket).to_be_visible(timeout=timeouts.DEFAULT)
    bucket.click()
    # A facet click is an RSC navigation: wait for the URL to carry the filter, then
    # for the grid to actually re-render to the narrowed set before any assertion.
    search.page.wait_for_url(re.compile(r".*minPrice=100000.*"), timeout=timeouts.NAVIGATION)
    search.wait_for_result_count(_MID_BUCKET_COUNT, timeout=timeouts.NAVIGATION)


@then("the results narrow to the listings in that price range")
def results_narrow_to_price_range(world: World) -> None:
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    before = world.state.extra["results_before"]
    after = search.result_count()
    assert after == _MID_BUCKET_COUNT, f"expected {_MID_BUCKET_COUNT} results, got {after}"
    assert after < before, f"results did not narrow ({after} not < {before})"
    world.logger.info(f"Results narrowed {before} -> {after}")


@then("the search URL reflects the selected price filter")
def search_url_reflects_price_filter(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*minPrice=100000.*"), timeout=timeouts.DEFAULT)
    expect(world.page).to_have_url(re.compile(r".*maxPrice=500000.*"), timeout=timeouts.DEFAULT)


@then("the facet counts update to reflect the narrowed set")
def facet_counts_update(world: World) -> None:
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    active = search.facet_bucket(_MID_BUCKET_KEY)
    expect(active).to_have_attribute("data-active", "true", timeout=timeouts.DEFAULT)
    assert search.bucket_count(active) == _MID_BUCKET_COUNT, (
        f"active bucket count = {search.bucket_count(active)}, expected {_MID_BUCKET_COUNT}"
    )
    # The out-of-range low bucket is recomputed against the filtered set.
    low_after = search.bucket_count(search.facet_bucket(_LOW_BUCKET_KEY))
    low_before = world.state.extra["low_count_before"]
    assert low_after < low_before, (
        f"'{_LOW_BUCKET_KEY}' count did not update: before {low_before}, after {low_after}"
    )
    world.logger.info(f"Facet counts updated: '{_LOW_BUCKET_KEY}' {low_before} -> {low_after}")
