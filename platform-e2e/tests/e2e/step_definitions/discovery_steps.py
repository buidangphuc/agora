"""Steps for the ui-phase-discovery change: home, search and vouchers (`/`, `/search`, `/vouchers`).

Scenario names in the feature files echo the spec scenarios of
openspec/changes/ui-phase-discovery so `make spec-check` can match them. Seeding goes
through the gateway API; assertions use the page objects (role / testid locators).
These need the local stack (frontend + gateway + search index), so the matching
FEATURES.yaml entries stay `planned` until they have been run green.
"""

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
from src.pages import HomePage, SearchPage, VouchersPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World

SETTINGS = get_settings()

_PAGED_TOTAL = 26  # > 24 results: forces a second page
_INDEX_WAIT_SECONDS = 60
_MOBILE = {"width": 375, "height": 812}

_FABRICATED = re.compile(r"ĐÃ BÁN|MALL|FLASH SALE|-\d+%", re.IGNORECASE)


# ── helpers ──────────────────────────────────────────────────────────────
def _search(world: World) -> SearchPage:
    return world.get_page(PageName.SEARCH)  # type: ignore[return-value]


def _data_layer_events(world: World, name: str) -> list[dict]:
    events = world.page.evaluate("() => (window.dataLayer || []).filter(e => e && e.event)")
    return [e for e in events if e.get("event") == name]


def _seed_indexed_listings(world: World, count: int, prefix: str) -> str:
    """Publish `count` listings under one unique keyword and wait for the search index."""
    sf = world.service_factory
    keyword = f"{prefix}{uuid.uuid4().hex[:10]}"
    seller_name = fake.unique_username("disc_seller")
    seller_token = sf.auth.register(seller_name, SETTINGS.seed_password, "seller")
    sf.set_token(seller_token)
    for i in range(count):
        sf.listing.create_listing(
            Listing(
                title=f"{keyword} sản phẩm {i}",
                category_id="cat-electronics",
                price=100_000 + i * 1_000,
                stock=100,
                status="published",
                description="Sản phẩm seed tự động cho Discovery E2E.",
            )
        )
    world.state.seeded_seller = User(
        username=seller_name, password=SETTINGS.seed_password, role="seller", token=seller_token
    )
    read_model = BaseService(token=None)
    deadline = time.time() + _INDEX_WAIT_SECONDS
    hits = 0
    try:
        while time.time() < deadline:
            resp = read_model.post(ep.SEARCH_LISTINGS, {"query": keyword})
            # `hits` is only the first page (the backend page size is below the seeded count),
            # so count through the page total.
            hits = int((resp.get("page") or {}).get("total") or len(resp.get("hits") or []))
            if hits >= count:
                break
            time.sleep(2)
    finally:
        read_model.close()
    assert hits >= count, f"only {hits}/{count} listings indexed for {keyword!r}"
    login_via_api(world, User(fake.unique_username("disc_buyer"), SETTINGS.seed_password, "buyer"))
    world.state.extra["disc_keyword"] = keyword
    return keyword


# ── home ─────────────────────────────────────────────────────────────────
@then("the home page shows no fabricated commerce markers")
def home_has_no_fabricated_markers(world: World) -> None:
    home: HomePage = world.get_page(PageName.HOME)  # type: ignore[assignment]
    expect(home.listing_links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    body = world.page.locator("body").inner_text()
    assert not _FABRICATED.search(body), f"fabricated marker on the home page: {body[:300]!r}"
    assert world.page.locator(".line-through").count() == 0, "strike-through price rendered"
    assert world.page.get_by_test_id("countdown-clock").count() == 0, "countdown rendered"
    assert world.page.get_by_label(re.compile(r"trên 5 sao")).count() == 0, "star rating rendered"


@when("the visitor activates the first category on the home grid")
def activate_first_category(world: World) -> None:
    home: HomePage = world.get_page(PageName.HOME)  # type: ignore[assignment]
    link = home.category_links.first
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    world.state.extra["category_href"] = link.get_attribute("href")
    link.click()


@then("the browser navigates to the search page filtered by that category")
def navigated_to_category(world: World) -> None:
    expected = world.state.extra["category_href"]
    assert expected and expected.startswith("/search?category="), expected
    expect(world.page).to_have_url(re.compile(re.escape(expected)), timeout=timeouts.NAVIGATION)


@then("the home page settles without layout shift")
def home_has_no_layout_shift(world: World) -> None:
    home: HomePage = world.get_page(PageName.HOME)  # type: ignore[assignment]
    expect(home.hero_cta).to_be_visible(timeout=timeouts.DEFAULT)
    expect(home.listing_links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    cls = world.page.evaluate(
        """() => new Promise((resolve) => {
            let total = 0;
            new PerformanceObserver((list) => {
                for (const e of list.getEntries()) if (!e.hadRecentInput) total += e.value;
            }).observe({ type: 'layout-shift', buffered: true });
            setTimeout(() => resolve(total), 1500);
        })"""
    )
    assert cls < 0.01, f"cumulative layout shift {cls} after the skeletons were replaced"


# ── search: URL state, tags, sort, pagination, states ────────────────────
@then("opening the same search URL in a new tab shows the same selected facet")
def same_url_same_state(world: World) -> None:
    key = world.state.extra["selected_price_key"]
    other = world.context.new_page()
    try:
        other.goto(world.page.url, wait_until="domcontentloaded")
        selected = other.locator(f'[data-testid="facet-bucket"][data-key="{key}"]')
        expect(selected).to_have_attribute("data-active", "true", timeout=timeouts.NAVIGATION)
    finally:
        other.close()


@when("the buyer removes the price filter tag")
def remove_price_tag(world: World) -> None:
    search = _search(world)
    link = search.remove_filter_link("Giá")
    expect(link).to_be_visible(timeout=timeouts.DEFAULT)
    link.click()


@then("the search URL no longer carries a price filter")
def url_without_price(world: World) -> None:
    expect(world.page).not_to_have_url(
        re.compile(r".*(minPrice|maxPrice)="), timeout=timeouts.DEFAULT
    )
    # The keyword tag stays in the strip, so assert on the price tag rather than on the strip.
    expect(_search(world).remove_filter_link("Giá")).to_have_count(0, timeout=timeouts.DEFAULT)


@when("the buyer clears all filters")
def clear_all_filters(world: World) -> None:
    search = _search(world)
    expect(search.clear_all_link).to_be_visible(timeout=timeouts.DEFAULT)
    search.clear_all_link.click()


@then("the search URL carries only the keyword")
def url_only_keyword(world: World) -> None:
    keyword = world.state.extra["facet_keyword"]
    expect(world.page).to_have_url(
        re.compile(rf".*/search\?q={keyword}$"), timeout=timeouts.DEFAULT
    )


@then("the sort controls offer no Bán Chạy option")
def no_best_selling_sort(world: World) -> None:
    search = _search(world)
    expect(search.sort_link("Mới nhất")).to_be_visible(timeout=timeouts.DEFAULT)
    assert world.page.get_by_text("Bán Chạy", exact=False).count() == 0


@given("more than one page of listings is indexed")
def paged_listings_indexed(world: World) -> None:
    _seed_indexed_listings(world, _PAGED_TOTAL, "zpage")


@when("the buyer opens the newest-first search results for those listings")
def open_paged_results(world: World) -> None:
    keyword = world.state.extra["disc_keyword"]
    search = _search(world)
    world.page.goto(f"{search.url()}?q={keyword}&sort=newest", wait_until="domcontentloaded")
    expect(search.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the pagination links keep the query and the sort")
def pagination_links_keep_params(world: World) -> None:
    keyword = world.state.extra["disc_keyword"]
    search = _search(world)
    expect(search.page_link(1)).to_have_attribute("aria-current", "page", timeout=timeouts.DEFAULT)
    href = search.page_link(2).get_attribute("href") or ""
    assert f"q={keyword}" in href and "sort=newest" in href and "page=2" in href, href


@when("the buyer opens a results page far beyond the last page")
def open_far_page(world: World) -> None:
    keyword = world.state.extra["disc_keyword"]
    search = _search(world)
    world.page.goto(f"{search.url()}?q={keyword}&page=99", wait_until="domcontentloaded")


@then("the buyer is redirected to the last page of results")
def redirected_to_last_page(world: World) -> None:
    keyword = world.state.extra["disc_keyword"]
    # 26 results at 24 per page -> the last page is 2.
    expect(world.page).to_have_url(
        re.compile(rf".*/search\?q={keyword}&page=2$"), timeout=timeouts.NAVIGATION
    )


@given("a keyword that matches no listing")
def keyword_without_hits(world: World) -> None:
    login_via_api(world, User(fake.unique_username("disc_buyer"), SETTINGS.seed_password, "buyer"))
    world.state.extra["disc_keyword"] = f"zzzz{uuid.uuid4().hex[:12]}"


@when("the buyer opens the search results for that keyword")
def open_results_for_keyword(world: World) -> None:
    search = _search(world)
    search.navigate_query(world.state.extra["disc_keyword"])
    world.page.wait_for_load_state("networkidle")


@then("an Empty state offers a clear-filters action linking to the search page")
def zero_results_empty(world: World) -> None:
    search = _search(world)
    expect(search.empty_state.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(search.clear_filters_button).to_have_attribute("href", "/search")
    assert search.error_alert.count() == 0


@given("the search backend rejects the visitor's session")
def search_backend_rejects_session(world: World) -> None:
    """Force `searchListings` to throw: an invalid bearer makes the gateway refuse the call.

    (Server-side fetches cannot be intercepted from the browser, so a bad session cookie is
    the deterministic way to make the frontend's SearchService call fail.)
    """
    world.context.add_cookies(
        [{"name": "session", "value": "not.a.jwt", "url": world.settings.base_url}]
    )


@then("an error Alert with a retry link is shown instead of an empty result")
def search_error_alert(world: World) -> None:
    search = _search(world)
    expect(search.error_alert).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(search.retry_link).to_have_attribute("href", re.compile(r"^/search"))
    assert search.empty_state.count() == 0, "a failed search must not read as 'no results'"
    expect(search.facets_sidebar).to_be_visible()


# ── search: mobile filters (375px) ───────────────────────────────────────
@given("the viewport is a 375px wide phone")
def phone_viewport(world: World) -> None:
    world.page.set_viewport_size(_MOBILE)


@when("the buyer opens the search results with two filters active")
def open_results_two_filters(world: World) -> None:
    keyword = world.state.extra["facet_keyword"]
    search = _search(world)
    world.page.goto(
        f"{search.url()}?q={keyword}&category=cat-electronics&minPrice=1000",
        wait_until="domcontentloaded",
    )
    expect(search.filter_button).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the Bộ lọc button shows a badge with 2")
def filter_badge_two(world: World) -> None:
    expect(_search(world).filter_button).to_contain_text("2")


@when("the buyer taps the Bộ lọc button")
def tap_filter_button(world: World) -> None:
    _search(world).filter_button.click()


@then("a Drawer opens with the filters and focus moves into it")
def drawer_open_focus(world: World) -> None:
    drawer = _search(world).filter_drawer
    expect(drawer).to_be_visible(timeout=timeouts.DEFAULT)
    expect(drawer.get_by_text("Khoảng giá")).to_be_visible()
    inside = world.page.evaluate(
        "() => { const d = document.querySelector('dialog'); "
        "return !!d && d.contains(document.activeElement); }"
    )
    assert inside, "focus did not move into the filter drawer"


@when("the buyer presses Escape")
def press_escape(world: World) -> None:
    world.page.keyboard.press("Escape")


@then("the Drawer closes and focus returns to the Bộ lọc button")
def drawer_closed_focus_returns(world: World) -> None:
    search = _search(world)
    expect(search.filter_drawer).to_have_count(0, timeout=timeouts.DEFAULT)
    expect(search.filter_button).to_be_focused()


@then("the page has no horizontal scroll")
def no_horizontal_scroll(world: World) -> None:
    overflow = world.page.evaluate(
        "() => document.documentElement.scrollWidth - document.documentElement.clientWidth"
    )
    assert overflow <= 0, f"page overflows horizontally by {overflow}px"


@then("every product card is laid out in two columns")
def two_columns(world: World) -> None:
    expect(_search(world).results_wrapper).to_be_visible()
    columns = world.page.evaluate(
        """() => {
            const grid = document.querySelector('[data-testid="search-results"] .grid');
            return grid ? getComputedStyle(grid).gridTemplateColumns.split(' ').length : 0;
        }"""
    )
    assert columns == 2, f"expected 2 grid columns at 375px, got {columns}"


# ── vouchers ─────────────────────────────────────────────────────────────
@given("a voucher has been seeded via the gateway")
def voucher_seeded(world: World) -> None:
    sf = world.service_factory
    seller = fake.unique_username("voucher_seller")
    token = sf.auth.register(seller, SETTINGS.seed_password, "seller")
    code = f"E2E{uuid.uuid4().hex[:8].upper()}"
    api = BaseService(token=token)
    try:
        resp = api.post(
            "/platform.promotion.v1.VoucherService/CreateVoucher",
            {
                "code": code,
                # /vouchers lists the platform-scoped vouchers (ListVouchers with no seller id).
                "scope": "VOUCHER_SCOPE_PLATFORM",
                "discountType": "DISCOUNT_TYPE_PERCENT",
                "discountValue": "10",
                "minSpend": "0",
                "maxDiscount": "50000",
                "quota": "100",
                "startsAt": "2026-01-01T00:00:00Z",
                "endsAt": "2027-12-31T23:59:59Z",
            },
        )
    finally:
        api.close()
    assert (resp.get("voucher") or {}).get("code") == code, f"CreateVoucher failed: {resp}"
    world.state.extra["voucher_code"] = code


@then("the vouchers hub lists the seeded voucher")
def hub_lists_seeded_voucher(world: World) -> None:
    page: VouchersPage = world.get_page(PageName.VOUCHERS)  # type: ignore[assignment]
    expect(page.card_by_code(world.state.extra["voucher_code"])).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then('no voucher card offers a "Lưu mã" button')
def no_save_button(world: World) -> None:
    page: VouchersPage = world.get_page(PageName.VOUCHERS)  # type: ignore[assignment]
    expect(page.voucher_cards.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.save_voucher_buttons).to_have_count(0)
    assert world.page.get_by_text("Đã lưu", exact=True).count() == 0


@when(parsers.parse('the visitor selects the "{label}" voucher tab'))
def select_voucher_tab(world: World, label: str) -> None:
    page: VouchersPage = world.get_page(PageName.VOUCHERS)  # type: ignore[assignment]
    world.state.extra["selected_tab_label"] = label
    page.tab(label).click()


@then(
    parsers.parse(
        'the voucher URL carries type "{tab_type}" and that tab is still selected after a reload'
    )
)
def tab_survives_reload(world: World, tab_type: str) -> None:
    page: VouchersPage = world.get_page(PageName.VOUCHERS)  # type: ignore[assignment]
    expect(world.page).to_have_url(re.compile(rf".*/vouchers\?type={tab_type}$"))
    world.page.reload(wait_until="domcontentloaded")
    expect(world.page).to_have_url(re.compile(rf".*/vouchers\?type={tab_type}$"))
    selected = page.tabs.locator('[aria-current="page"]')
    expect(selected).to_have_count(1)
    expect(selected).to_contain_text(world.state.extra["selected_tab_label"])


@then("an Empty state offers a link to the product search")
def voucher_empty_state(world: World) -> None:
    page: VouchersPage = world.get_page(PageName.VOUCHERS)  # type: ignore[assignment]
    expect(page.empty_action).to_have_attribute("href", "/search", timeout=timeouts.DEFAULT)
    expect(page.voucher_cards).to_have_count(0)


# ── tracking (unchanged behaviour) ───────────────────────────────────────
@when("the first result card is scrolled into view")
def scroll_first_card(world: World) -> None:
    search = _search(world)
    link = search.results_wrapper.locator('a[href^="/listing/"]').first
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    world.state.extra["first_card_id"] = (link.get_attribute("href") or "").rsplit("/", 1)[-1]
    link.scroll_into_view_if_needed()
    world.page.wait_for_timeout(500)  # IntersectionObserver threshold 0.3


@then("exactly one view_item_list event for that card is recorded with its position")
def one_card_impression(world: World) -> None:
    card_id = world.state.extra["first_card_id"]
    mine = [
        e
        for e in _data_layer_events(world, "view_item_list")
        if len((e.get("ecommerce") or {}).get("items") or []) == 1
        and e["ecommerce"]["items"][0].get("item_id") == card_id
    ]
    assert len(mine) == 1, f"expected one card impression for {card_id}, got {len(mine)}"
    assert mine[0]["ecommerce"]["items"][0].get("index") == 1


@when("the buyer clicks the image of that card")
def click_first_card(world: World) -> None:
    card_id = world.state.extra["first_card_id"]
    world.page.locator(f'a[href="/listing/{card_id}"]').first.click()
    world.page.wait_for_url(re.compile(rf".*/listing/{card_id}"), timeout=timeouts.NAVIGATION)


@then("one select_item event for that card is recorded with its position")
def one_select_item(world: World) -> None:
    card_id = world.state.extra["first_card_id"]
    mine = [
        e
        for e in _data_layer_events(world, "select_item")
        if (e.get("ecommerce") or {}).get("items", [{}])[0].get("item_id") == card_id
    ]
    assert len(mine) == 1, f"expected one select_item for {card_id}, got {len(mine)}"
    assert mine[0]["ecommerce"]["items"][0].get("index") == 1


@then("exactly one batched search_results impression carries every rendered result")
def one_batched_search_impression(world: World) -> None:
    keyword = world.state.extra["facet_keyword"]
    batched = [
        e
        for e in _data_layer_events(world, "view_item_list")
        if any(
            it.get("item_list_id") == "search_results"
            for it in (e.get("ecommerce") or {}).get("items") or []
        )
    ]
    assert len(batched) == 1, f"expected one batched impression, got {len(batched)}"
    items = batched[0]["ecommerce"]["items"]
    assert len(items) == _search(world).result_count(), "batched impression != rendered results"
    assert [it["index"] for it in items] == list(range(1, len(items) + 1))
    assert batched[0].get("search_term") == keyword
