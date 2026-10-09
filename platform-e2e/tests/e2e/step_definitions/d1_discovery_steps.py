"""Steps for frontend/discovery_ui.feature (ui-phase-discovery).

Scenario names echo the spec scenarios of openspec/changes/ui-phase-discovery. Listings are
seeded through the gateway under a unique keyword and waited for in the search index. Image
requests to the object store are intercepted so cards render real pictures (or slow ones) without
a MinIO upload. Assertions are on the rendered page.
"""

from __future__ import annotations

import base64
import re
import time
import uuid

from playwright.sync_api import Locator, expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, BaseService, ListingService
from src.constants import PageName, timeouts
from src.constants import gateway_endpoints as ep
from src.models import Listing, User
from src.pages import HomePage, SearchPage, VouchersPage
from src.pages.saved_searches_page import SavedSearchesPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.step_definitions.d1_gates_steps import FRONTEND, _run
from tests.e2e.step_definitions.pdp_streaming_steps import _record_beacons
from tests.e2e.support.world import World

SETTINGS = get_settings()
BASE = SETTINGS.base_url.rstrip("/")
_PNG = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
)
_INDEX_WAIT_S = 90
_RESULTS = '[data-testid="search-results"]'
_CARDS = f"{_RESULTS} .grid > div"
_MOBILE = {"width": 375, "height": 812}


# ── helpers ──────────────────────────────────────────────────────────────
def _search(world: World) -> SearchPage:
    return world.get_page(PageName.SEARCH)  # type: ignore[return-value]


def _install_image_route(world: World) -> None:
    """Serve every object-store image as a real PNG, after `image_delay_ms` when set."""

    def serve(route) -> None:  # noqa: ANN001
        delay = world.state.extra.get("image_delay_ms", 0)
        if delay:
            world.page.wait_for_timeout(delay)
        route.fulfill(status=200, content_type="image/png", body=_PNG)

    world.page.route("**/listing-images/**", serve)
    world.add_cleanup(lambda: world.page.unroute_all(behavior="ignoreErrors"))


def _seed_indexed(world: World, specs: list[dict], prefix: str = "zd1") -> str:
    """Create the listings as one fresh seller and wait until all are searchable."""
    keyword = f"{prefix}{uuid.uuid4().hex[:10]}"
    token = AuthService().register(
        fake.unique_username("d1_seller"), SETTINGS.seed_password, "seller"
    )
    svc = ListingService(token=token)
    for i, spec in enumerate(specs):
        listing = Listing(
            title=f"{keyword} {spec.get('title', 'san pham')} {i}",
            category_id="cat-electronics",
            price=spec.get("price", 100_000),
            stock=10,
            status="published",
            description="Sản phẩm seed tự động cho Discovery E2E.",
        )
        lid = svc.create_listing(listing)
        keys = spec.get("image_keys")
        if keys:
            current = svc.get_listing(lid)
            current["imageKeys"] = keys
            svc.post("/platform.listing.v1.ListingService/UpdateListing", {"listing": current})
    api = BaseService()
    deadline = time.time() + _INDEX_WAIT_S
    total = 0
    while time.time() < deadline:
        resp = api.post(ep.SEARCH_LISTINGS, {"query": keyword})
        total = int((resp.get("page") or {}).get("total") or len(resp.get("hits") or []))
        if total >= len(specs):
            break
        time.sleep(2)
    api.close()
    assert total >= len(specs), f"only {total}/{len(specs)} listings indexed for {keyword!r}"
    if world.state.current_user is None:
        login_via_api(
            world, User(fake.unique_username("d1_buyer"), SETTINGS.seed_password, "buyer")
        )
    world.state.extra["disc_keyword"] = keyword
    world.state.extra["facet_keyword"] = keyword
    return keyword


def _goto(world: World, route: str, wait_until: str = "domcontentloaded") -> None:
    world.page.goto(f"{BASE}{route}", wait_until=wait_until)


def _cards(world: World) -> Locator:
    return world.page.locator(_CARDS)


def _wait_cards(world: World, minimum: int = 1) -> None:
    expect(_cards(world).first).to_be_visible(timeout=timeouts.NAVIGATION)
    assert _cards(world).count() >= minimum


# ── Seeding steps ────────────────────────────────────────────────────────
@given("listings with images are indexed under one keyword")
def listings_with_images(world: World) -> None:
    _install_image_route(world)
    kw = uuid.uuid4().hex[:8]
    _seed_indexed(
        world, [{"price": 150_000, "image_keys": [f"e2e/{kw}/{i}.png"]} for i in range(8)]
    )


@given(parsers.parse("{count:d} listings with images are indexed under one keyword"))
def many_listings_with_images(world: World, count: int) -> None:
    _install_image_route(world)
    kw = uuid.uuid4().hex[:8]
    _seed_indexed(
        world,
        [{"price": 1_100_000 + i, "image_keys": [f"e2e/{kw}/{i}.png"]} for i in range(count)],
        prefix="zd1p",
    )


@given("listings without a working image are indexed under one keyword")
def listings_without_image(world: World) -> None:
    _seed_indexed(
        world,
        [
            {"price": 120_000},
            {"price": 130_000, "image_keys": ["http://127.0.0.1:9/never-served.png"]},
        ],
    )


@given(parsers.parse("listings priced {low:d} and {high:d} with a brand keyword are indexed"))
def listings_priced(world: World, low: int, high: int) -> None:
    _seed_indexed(
        world,
        [
            {"price": low, "title": "Apple ốp lưng"},
            {"price": high, "title": "Apple iPhone"},
            {"price": low, "title": "Samsung ốp lưng"},
        ],
    )
    world.state.extra["price_high"] = high


@given("the listing images load slowly")
def images_slow(world: World) -> None:
    world.state.extra["image_delay_ms"] = 2500


# ── Card and grid ────────────────────────────────────────────────────────
def _measure(world: World) -> list[dict]:
    return world.page.evaluate(
        """(sel) => [...document.querySelectorAll(sel)].map((card) => {
            const box = card.querySelector('.aspect-square');
            const r = box ? box.getBoundingClientRect() : null;
            return {h: card.getBoundingClientRect().height,
                    bw: r ? r.width : 0, bh: r ? r.height : 0};
        })""",
        _CARDS,
    )


@when("the buyer opens the search results while the images are still loading")
def open_results_images_pending(world: World) -> None:
    keyword = world.state.extra["disc_keyword"]
    _goto(world, f"/search?q={keyword}")
    _wait_cards(world, 8)
    world.state.extra["before"] = _measure(world)
    still_loading = world.page.evaluate(
        f"() => [...document.querySelectorAll('{_CARDS} img')].filter(i => !i.complete).length"
    )
    assert still_loading > 0, "the images had already loaded; the box was not measured before them"


@then("every card image container is already a 1:1 box")
def boxes_are_square(world: World) -> None:
    for box in world.state.extra["before"]:
        assert box["bw"] > 0 and abs(box["bw"] - box["bh"]) < 1, box


@then("no card changes height when its image loads")
def no_height_change(world: World) -> None:
    world.page.wait_for_function(
        f"() => [...document.querySelectorAll('{_CARDS} img')].every(i => i.complete && i.naturalWidth > 0)",
        timeout=timeouts.NAVIGATION,
    )
    after = _measure(world)
    before = world.state.extra["before"]
    assert len(after) == len(before)
    for b, a in zip(before, after, strict=True):
        assert abs(b["h"] - a["h"]) < 1, f"card height changed {b['h']} -> {a['h']}"


@then("every card shows the placeholder inside a 1:1 box")
def placeholders_in_box(world: World) -> None:
    _wait_cards(world, 2)
    world.page.wait_for_timeout(1500)  # a broken image swaps to the fallback after its error
    for i in range(_cards(world).count()):
        card = _cards(world).nth(i)
        expect(card.get_by_role("img", name="Không có ảnh")).to_be_visible()
        box = card.locator(".aspect-square").first.bounding_box()
        assert box and abs(box["width"] - box["height"]) < 1, box
        assert card.locator("img").count() == 0, "a broken <img> is still rendered"


@then("the images of cards 1-6 are not lazy and the images of cards 7-24 are lazy")
def lazy_split(world: World) -> None:
    _wait_cards(world, 24)
    loading = world.page.evaluate(
        f"""() => [...document.querySelectorAll('{_CARDS}')].map(
            (c) => (c.querySelector('img') || {{getAttribute: () => 'none'}}).getAttribute('loading'))"""
    )
    assert len(loading) == 24, len(loading)
    assert all(v != "lazy" for v in loading[:6]), loading[:6]
    assert all(v == "lazy" for v in loading[6:24]), loading[6:24]


@then("an Empty block says nothing was found and suggests changing keywords or filters")
def empty_block(world: World) -> None:
    expect(world.page.get_by_text("Không tìm thấy sản phẩm")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(world.page.get_by_text("Hãy thử từ khóa khác hoặc bỏ bớt bộ lọc.")).to_be_visible()


@then("no card shows a rating, a rating number or a sold count")
def no_rating_or_sold(world: World) -> None:
    _wait_cards(world, 3)
    for i in range(_cards(world).count()):
        card = _cards(world).nth(i)
        text = card.inner_text()
        assert not re.search(r"đã bán", text, re.I), text
        assert not re.search(r"\b[0-5][.,]\d\b", text), f"rating number in {text!r}"
        assert card.get_by_label(re.compile(r"sao")).count() == 0, "a star rating is rendered"


@then(
    "every card shows exactly one price and no strike-through price, discount badge or MALL badge"
)
def one_price_only(world: World) -> None:
    _wait_cards(world, 3)
    for i in range(_cards(world).count()):
        card = _cards(world).nth(i)
        text = card.inner_text()
        assert text.count("₫") == 1, f"expected one price, got {text!r}"
        assert card.locator(".line-through, s, del").count() == 0, text
        assert not re.search(r"-\s?\d+\s?%", text), text
        assert not re.search(r"mall", text, re.I), text


@then(
    "the card of the listing above 5000000 and the card with the brand keyword show no MALL badge"
)
def no_mall(world: World) -> None:
    _wait_cards(world, 3)
    high = world.state.extra["price_high"]
    shown = f"{high:,}".replace(",", ".")
    for needle in (shown, "Apple"):
        card = _cards(world).filter(has_text=needle).first
        expect(card).to_be_visible()
        assert not re.search(r"mall", card.inner_text(), re.I), card.inner_text()
        assert card.get_by_text(re.compile(r"mall", re.I)).count() == 0
    assert world.page.get_by_text(re.compile(r"^\s*mall\s*$", re.I)).count() == 0


# ── Unsupported features / tokens / hubs ─────────────────────────────────
@then(parsers.parse('no voucher card has a "{label}" button'))
def no_voucher_save_button(world: World, label: str) -> None:
    page: VouchersPage = world.get_page(PageName.VOUCHERS)  # type: ignore[assignment]
    expect(page.voucher_cards.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("button", name=label)).to_have_count(0)


@when("the buyer opens the search results page")
def open_search_page(world: World) -> None:
    if world.state.current_user is None:
        login_via_api(
            world, User(fake.unique_username("d1_buyer"), SETTINGS.seed_password, "buyer")
        )
    _goto(world, "/search")
    expect(_search(world).results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the sort options exclude "{label}"'))
def sort_excludes(world: World, label: str) -> None:
    sort_bar = world.page.get_by_text("Sắp xếp theo:").locator("xpath=..")
    expect(sort_bar).to_be_visible(timeout=timeouts.NAVIGATION)
    for option in ("Liên quan", "Mới nhất", "Giá thấp đến cao", "Giá cao đến thấp"):
        expect(sort_bar.get_by_text(option, exact=True).first).to_be_attached()
    assert world.page.get_by_text(label, exact=False).count() == 0


@when("the token lint runs on the discovery files")
def lint_discovery(world: World) -> None:
    prefixes = [
        "src/app/(shop)/(home)",
        "src/app/(shop)/search",
        "src/app/(shop)/vouchers",
        "src/features/listing",
        "src/features/search",
        "src/features/home",
        "src/features/voucher",
    ]
    world.state.extra["lint"] = _run(["node", "scripts/check-tokens.mjs", *prefixes], FRONTEND)


@then("the token lint reports no violation in the discovery files")
def lint_clean(world: World) -> None:
    result = world.state.extra["lint"]
    assert result.returncode == 0 and "0 violation(s)" in result.stdout, result.stdout[-1500:]


_HUBS = [
    "Kho voucher",
    "Freeship",
    "Giảm tiền",
    "Giảm theo %",
    "Tất cả sản phẩm",
    "Đơn mua",
    "Yêu thích",
    "Shop theo dõi",
]


@then("the eight service hub tiles share one neutral surface")
def hubs_neutral(world: World) -> None:
    home: HomePage = world.get_page(PageName.HOME)  # type: ignore[assignment]
    expect(home.hero_cta).to_be_visible(timeout=timeouts.NAVIGATION)
    surfaces = world.page.evaluate(
        """(labels) => labels.map((label) => {
            const link = [...document.querySelectorAll('main a')]
              .find((a) => (a.textContent || '').includes(label) && a.getAttribute('href'));
            if (!link) return {label, missing: true};
            let el = link;
            while (el && getComputedStyle(el).backgroundColor === 'rgba(0, 0, 0, 0)') el = el.parentElement;
            return {label, bg: el ? getComputedStyle(el).backgroundColor : 'none', cls: link.className};
        })""",
        _HUBS,
    )
    missing = [s["label"] for s in surfaces if s.get("missing")]
    assert not missing, f"hub tiles not found: {missing}"
    backgrounds = {s["bg"] for s in surfaces}
    assert len(backgrounds) == 1, f"hub tiles use different surfaces: {surfaces}"
    (bg,) = backgrounds
    channels = [int(c) for c in re.findall(r"\d+", bg)[:3]]
    assert max(channels) - min(channels) < 20, f"hub surface {bg} is not neutral"


@then("the home page has no flash-sale heading, countdown, sold bar or discount badge")
def home_no_flash_sale(world: World) -> None:
    home: HomePage = world.get_page(PageName.HOME)  # type: ignore[assignment]
    expect(home.listing_links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    body = world.page.locator("body").inner_text()
    assert not re.search(r"flash\s*sale", body, re.I), "a flash-sale heading is rendered"
    assert world.page.get_by_test_id("countdown-clock").count() == 0
    assert world.page.locator("main [role=progressbar]").count() == 0
    assert not re.search(r"-\d+\s?%", body), "a discount badge is rendered"
    assert not re.search(r"đã bán", body, re.I), "a sold bar is rendered"


# ── Category, filters, pagination ────────────────────────────────────────
@when(
    parsers.parse(
        'the buyer opens the search results for that keyword in the "{category}" category'
    )
)
def open_results_in_category(world: World, category: str) -> None:
    keyword = world.state.extra["disc_keyword"]
    _goto(world, f"/search?q={keyword}&category={category}")
    expect(_search(world).results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)


@then('the current category pill is marked aria-current and the "Tất cả" pill is not')
def category_pill_current(world: World) -> None:
    all_pill = world.page.get_by_role("link", name="Tất cả", exact=True)
    current = world.page.locator('a[href*="category=cat-electronics"][aria-current]')
    assert all_pill.count() >= 1, "no CategoryBar pills with a Tất cả pill above the results"
    assert current.count() >= 1
    assert all_pill.first.get_attribute("aria-current") in (None, "false")


@when(parsers.parse('the buyer opens page 2 of the results and selects the "{label}" price filter'))
def open_page2_and_filter(world: World, label: str) -> None:
    keyword = world.state.extra["disc_keyword"]
    _goto(world, f"/search?q={keyword}&page=2")
    search = _search(world)
    expect(search.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)
    bucket = world.page.get_by_test_id("facet-bucket").filter(has_text=label).first
    expect(bucket).to_be_visible()
    (bucket if bucket.evaluate("e => e.tagName") == "A" else bucket.locator("a")).first.click()


@then("the new URL carries the filter and no page")
def url_filter_no_page(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*minPrice="), timeout=timeouts.NAVIGATION)
    assert "page=" not in world.page.url, world.page.url


@then("no Pagination is rendered")
def no_pagination(world: World) -> None:
    _wait_cards(world, 3)
    expect(world.page.get_by_role("navigation", name="Phân trang")).to_have_count(0)


@when(parsers.parse("the buyer enters a minimum of {low:d} and a maximum of {high:d} and applies"))
def enter_price_range(world: World, low: int, high: int) -> None:
    world.state.extra["url_before"] = world.page.url
    world.page.locator("input[name=minPrice]").fill(str(low))
    world.page.locator("input[name=maxPrice]").fill(str(high))
    world.page.get_by_role("button", name="Áp dụng").first.click()


@then(
    "the maximum field shows an error, the URL does not change and the apply button is not loading"
)
def price_range_rejected(world: World) -> None:
    maximum = world.page.locator("input[name=maxPrice]")
    expect(maximum).to_have_attribute("aria-invalid", "true", timeout=timeouts.DEFAULT)
    expect(
        world.page.get_by_text("Giá tối đa phải lớn hơn hoặc bằng giá tối thiểu.")
    ).to_be_visible()
    assert world.page.url == world.state.extra["url_before"], world.page.url
    apply_button = world.page.get_by_role("button", name="Áp dụng").first
    expect(apply_button).not_to_have_attribute("aria-busy", "true")
    expect(apply_button).to_be_enabled()


# ── Saved search ─────────────────────────────────────────────────────────
@when(parsers.parse('the buyer activates "{label}"'))
def activate_save(world: World, label: str) -> None:
    button = SavedSearchesPage(world.page).save_button
    SavedSearchesPage(world.page).wait_until_interactive(button)
    button.click()


@then(
    "the button is pending and disabled, then a success toast appears and the saved list contains the keyword"
)
def save_pending_then_toast(world: World) -> None:
    button = SavedSearchesPage(world.page).save_button
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.SHORT)
    expect(button).to_be_disabled()
    expect(world.page.get_by_role("status").first).to_be_visible(timeout=timeouts.NAVIGATION)
    keyword = world.state.extra["disc_keyword"]
    expect(SavedSearchesPage(world.page).saved_item(keyword)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('"{label}" is disabled so no request can be sent and nothing is saved'))
def save_disabled_without_query(world: World, label: str) -> None:
    posts: list[str] = []
    world.page.on(
        "request",
        lambda r: (
            posts.append(r.url) if r.method == "POST" and r.headers.get("next-action") else None
        ),
    )
    button = SavedSearchesPage(world.page).save_button
    expect(button).to_be_disabled(timeout=timeouts.NAVIGATION)
    button.click(force=True, no_wait_after=True)
    world.page.wait_for_timeout(500)
    assert posts == [], posts
    expect(SavedSearchesPage(world.page).empty_state).to_be_visible()


# ── Vouchers ─────────────────────────────────────────────────────────────
@given("the browser storage is observed")
def storage_observed(world: World) -> None:
    world.page.add_init_script("""(() => {
          window.__lsLog = [];
          for (const m of ['getItem', 'setItem', 'removeItem']) {
            const orig = Storage.prototype[m];
            Storage.prototype[m] = function (k, ...rest) {
              window.__lsLog.push(m + ':' + k);
              return orig.call(this, k, ...rest);
            };
          }
        })();""")


@when("the visitor opens the vouchers page")
def visitor_opens_vouchers(world: World) -> None:
    page: VouchersPage = world.navigate_to(PageName.VOUCHERS)  # type: ignore[assignment]
    expect(page.voucher_cards.first).to_be_visible(timeout=timeouts.NAVIGATION)


@then(
    'no card has a "Lưu mã" or "Đã lưu" control and no voucher key was read from or written to localStorage'
)
def no_voucher_wallet(world: World) -> None:
    assert world.page.get_by_role("button", name=re.compile(r"Lưu mã|Đã lưu")).count() == 0
    log = world.page.evaluate("() => window.__lsLog || []")
    assert not [e for e in log if re.search(r"voucher|wallet|saved", e, re.I)], log


def _switch_session(world: World, role: str) -> None:
    username = fake.unique_username(f"d1_{role}")
    token = AuthService().register(username, SETTINGS.seed_password, role)
    world.context.clear_cookies()
    world.context.add_cookies([{"name": "session", "value": token, "url": BASE}])


@when("a buyer opens the vouchers page")
def buyer_opens_vouchers(world: World) -> None:
    _switch_session(world, "buyer")
    _goto(world, "/vouchers")
    expect(VouchersPage(world.page).voucher_cards.first).to_be_visible(timeout=timeouts.NAVIGATION)


@when("a seller opens the vouchers page")
def seller_opens_vouchers(world: World) -> None:
    _switch_session(world, "seller")
    _goto(world, "/vouchers")


@then("the voucher manager is not rendered")
def manager_absent(world: World) -> None:
    assert world.page.get_by_text("Tạo Voucher", exact=False).count() == 0
    assert world.page.get_by_text("Danh sách Voucher", exact=False).count() == 0


@then("the voucher manager is rendered")
def manager_present(world: World) -> None:
    expect(world.page.get_by_text("Tạo Voucher", exact=False).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("voucher cards are one per row and the page does not scroll sideways")
def vouchers_one_per_row(world: World) -> None:
    cards = VouchersPage(world.page).voucher_cards
    assert cards.count() >= 2
    lefts = {round(cards.nth(i).bounding_box()["x"]) for i in range(min(cards.count(), 6))}
    assert len(lefts) == 1, f"voucher cards sit in several columns at 375px: {lefts}"
    width = world.page.evaluate("() => document.documentElement.scrollWidth")
    assert width <= _MOBILE["width"], f"page is {width}px wide at 375px"
    overflow = world.page.evaluate(
        "() => { const u = document.querySelector('nav[aria-label=Tabs] ul');"
        " return u ? getComputedStyle(u).overflowX : 'none'; }"
    )
    assert overflow in ("auto", "scroll"), f"voucher tabs do not scroll horizontally ({overflow})"


# ── Search bar ───────────────────────────────────────────────────────────
def _searchbox(world: World) -> Locator:
    return world.page.get_by_role("combobox", name="Tìm kiếm sản phẩm")


@when(parsers.parse('the visitor types "{text}" in the search bar and presses Enter'))
def type_and_enter(world: World, text: str) -> None:
    box = _searchbox(world)
    SavedSearchesPage(world.page).wait_until_interactive(box)
    box.fill(text)
    box.press("Enter")


@then(parsers.parse('the browser navigates to "{route}"'))
def navigated_to(world: World, route: str) -> None:
    world.page.wait_for_url(re.compile(re.escape(BASE + route) + r"$"), timeout=timeouts.NAVIGATION)


@when("the visitor presses Enter in the empty search bar")
def enter_empty(world: World) -> None:
    box = _searchbox(world)
    SavedSearchesPage(world.page).wait_until_interactive(box)
    world.state.extra["url_before"] = world.page.url
    box.click()
    box.press("Enter")
    world.page.wait_for_timeout(800)


@then("the browser stays on the home page")
def stays_home(world: World) -> None:
    assert world.page.url == world.state.extra["url_before"], world.page.url


@given("the suggestion endpoint fails")
def suggest_fails(world: World) -> None:
    world.page.route("**/api/suggest**", lambda route: route.fulfill(status=500, body="boom"))
    world.add_cleanup(lambda: world.page.unroute_all(behavior="ignoreErrors"))


@when(parsers.parse('the visitor types "{text}" in the search bar'))
def type_in_search_bar(world: World, text: str) -> None:
    box = _searchbox(world)
    SavedSearchesPage(world.page).wait_until_interactive(box)
    box.click()
    box.press_sequentially(text, delay=50)
    world.page.wait_for_timeout(700)  # 200ms debounce + the failing request


@then("the suggestion list is hidden, no toast appears and the input keeps working")
def suggestions_hidden(world: World) -> None:
    assert world.page.get_by_role("option").count() == 0
    expect(world.page.get_by_role("status")).to_have_count(0)
    box = _searchbox(world)
    box.press_sequentially("x")
    expect(box).to_have_value("aox")


@when("the visitor types the keyword in the search bar and presses ArrowDown then Enter")
def keyboard_select(world: World) -> None:
    keyword = world.state.extra["disc_keyword"]
    _goto(world, "/")
    box = _searchbox(world)
    SavedSearchesPage(world.page).wait_until_interactive(box)
    box.click()
    box.press_sequentially(keyword, delay=30)
    options = world.page.get_by_role("option")
    expect(options.first).to_be_visible(timeout=timeouts.NAVIGATION)
    world.state.extra["first_suggestion"] = options.first.inner_text().split("\n")[0].strip()
    box.press("ArrowDown")
    box.press("Enter")


@then("the first suggestion is submitted as the query")
def first_suggestion_submitted(world: World) -> None:
    suggestion = world.state.extra["first_suggestion"]
    world.page.wait_for_url(re.compile(r".*/search\?q="), timeout=timeouts.NAVIGATION)
    query = re.search(r"[?&]q=([^&]*)", world.page.url).group(1)
    from urllib.parse import unquote_plus

    assert unquote_plus(query) == suggestion, (query, suggestion)


# ── Tracking attribution on the streamed home row ────────────────────────
@given("a buyer with the home page open while recording tracking beacons")
def home_recording_beacons(world: World) -> None:
    login_via_api(world, User(fake.unique_username("d1_buyer"), SETTINGS.seed_password, "buyer"))
    _record_beacons(world)
    _goto(world, "/")


@when("the recommendations row has streamed in, the buyer scrolls to it and clicks its first card")
def click_recommendation_card(world: World) -> None:
    row = world.page.locator("[data-recs-request-id]").first
    expect(row).to_be_visible(timeout=timeouts.LONG)
    card = row.locator('a[href^="/listing/"]').first
    card.scroll_into_view_if_needed()
    deadline = time.monotonic() + 15
    beacons: list[dict] = world.state.extra["beacons"]
    while time.monotonic() < deadline and not any(
        b.get("type") == "impression" and b.get("placementId") == "home_feed" for b in beacons
    ):
        world.page.wait_for_timeout(500)
    card.click()
    world.page.wait_for_url(re.compile(r".*/listing/"), timeout=timeouts.NAVIGATION)


@then('both the impression and the click beacon carry placementId "home_feed"')
def beacons_attributed(world: World) -> None:
    beacons: list[dict] = world.state.extra["beacons"]
    deadline = time.monotonic() + 12
    while time.monotonic() < deadline and not any(
        b.get("type") == "click" and b.get("placementId") == "home_feed" for b in beacons
    ):
        world.page.wait_for_timeout(500)
    for kind in ("impression", "click"):
        mine = [b for b in beacons if b.get("type") == kind and b.get("placementId") == "home_feed"]
        assert mine, f"no {kind} beacon with placementId home_feed in {beacons}"
