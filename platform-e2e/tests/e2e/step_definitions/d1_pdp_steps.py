"""Steps for frontend/product_detail_ui.feature (ui-phase-product-detail).

Scenario names echo the spec scenarios of openspec/changes/ui-phase-product-detail. Sellers,
listings, variants, images (object-store keys served as PNG through an intercepted route), reviews
and shop names are seeded through the gateway; the buyer's session cookie is injected.
"""

from __future__ import annotations

import base64
import re
import time
import uuid

from playwright.sync_api import Locator, expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, ListingService
from src.api.services.engagement_service import EngagementService
from src.constants import PageName, timeouts
from src.models import Listing, User
from src.pages import ListingDetailPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.step_definitions.d1_gates_steps import FRONTEND, _run
from tests.e2e.step_definitions.follow_seller_steps import _principal_id
from tests.e2e.step_definitions.pdp_streaming_steps import _record_beacons
from tests.e2e.support.world import World

SETTINGS = get_settings()
BASE = SETTINGS.base_url.rstrip("/")
_PNG = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
)
_VARIANTS = [
    {"name": "128GB", "sku": "SKU-A", "price": 100_000, "stock": 10},
    {"name": "256GB", "sku": "SKU-B", "price": 150_000, "stock": 3},
    {"name": "512GB", "sku": "SKU-C", "price": 200_000, "stock": 0},
]
_PHONE = {"width": 375, "height": 812}
_DESKTOP = {"width": 1280, "height": 900}


# ── helpers ──────────────────────────────────────────────────────────────
def _detail(world: World) -> ListingDetailPage:
    return world.get_page(PageName.LISTING_DETAIL)  # type: ignore[return-value]


def _x(world: World) -> dict:
    return world.state.extra


def _listing_id(world: World) -> str:
    return _x(world)["pdp_listing_id"]


def _toast(world: World, text: str | None = None) -> Locator:
    toasts = world.page.get_by_role("status")
    return toasts.filter(has_text=text) if text else toasts


def _seed(
    world: World,
    *,
    title: str | None = None,
    price: int = 100_000,
    stock: int = 12,
    variants: list[dict] | None = None,
    images: int = 0,
    shop_name: str | None = None,
) -> str:
    username = fake.unique_username("pdp_seller")
    token = AuthService().register(username, SETTINGS.seed_password, "seller")
    svc = ListingService(token=token)
    if shop_name:
        svc.upsert_storefront(f"e2e-shop-{uuid.uuid4().hex[:8]}", shop_name)
    listing = Listing(
        title=title or f"[E2E][PDP] {fake.price_vnd():d}",
        category_id="cat-electronics",
        price=price,
        stock=stock,
        status="published",
        description="Mô tả sản phẩm seed cho PDP E2E.",
    )
    listing_id = svc.create_listing(listing, variants=variants)
    if images:
        current = svc.get_listing(listing_id)
        current["imageKeys"] = [f"e2e/pdp/{uuid.uuid4().hex[:6]}-{i}.png" for i in range(images)]
        svc.post("/platform.listing.v1.ListingService/UpdateListing", {"listing": current})
        world.page.route(
            "**/listing-images/**",
            lambda route: route.fulfill(status=200, content_type="image/png", body=_PNG),
        )
        world.add_cleanup(lambda: world.page.unroute_all(behavior="ignoreErrors"))
    stored = svc.get_listing(listing_id)
    _x(world).update(
        pdp_listing_id=listing_id,
        pdp_seller_token=token,
        pdp_seller_id=_principal_id(token),
        pdp_variant_ids={v["name"]: v["id"] for v in stored.get("variants", [])},
        pdp_title=listing.title,
    )
    listing.listing_id = listing_id
    world.state.listing = listing
    return listing_id


def _sign_in(world: World) -> None:
    login_via_api(world, User(fake.unique_username("pdp_buyer"), SETTINGS.seed_password, "buyer"))


def _open(
    world: World,
    *,
    width: int | None = None,
    record: bool = False,
    query: str = "",
) -> None:
    if width:
        world.page.set_viewport_size({"width": width, "height": 812 if width < 768 else 900})
    if record:
        _record_beacons(world)
    world.page.goto(f"{BASE}/listing/{_listing_id(world)}{query}", wait_until="domcontentloaded")
    expect(_detail(world).add_to_cart_button.first).to_be_visible(timeout=timeouts.NAVIGATION)


# ── Givens ───────────────────────────────────────────────────────────────
@given(parsers.parse("a published listing with stock {stock:d} and a buyer on its page at 1280px"))
def listing_stock_desktop(world: World, stock: int) -> None:
    _seed(world, stock=stock)
    _sign_in(world)
    _open(world, width=1280)


@given(
    parsers.parse(
        "a published listing with stock {stock:d} and a buyer on its page while recording beacons"
    )
)
def listing_stock_recording(world: World, stock: int) -> None:
    _seed(world, stock=stock)
    _sign_in(world)
    _open(world, record=True)


@given("a published listing with variants and a buyer on its page")
def listing_variants(world: World) -> None:
    _seed(world, variants=_VARIANTS)
    _sign_in(world)
    _open(world)


@given("a published listing with variants and a buyer on its page at 375px")
def listing_variants_phone(world: World) -> None:
    _seed(world, variants=_VARIANTS)
    _sign_in(world)
    _open(world, width=375)


@given("a published listing with variants and a buyer on its page while recording beacons")
def listing_variants_recording(world: World) -> None:
    _seed(world, variants=_VARIANTS)
    _sign_in(world)
    _open(world, record=True)


@given(
    "a published listing with variants and a buyer on its page with the out-of-stock variant in the URL"
)
def listing_variants_oos_url(world: World) -> None:
    _seed(world, variants=_VARIANTS)
    _sign_in(world)
    _open(world, query=f"?variant={_x(world)['pdp_variant_ids']['512GB']}")


@given(parsers.parse("a published listing with {count:d} images and a buyer on its page"))
def listing_with_images(world: World, count: int) -> None:
    _seed(world, images=count)
    _sign_in(world)
    _open(world, width=1280)


@given(parsers.parse("a published listing priced {price:d} and a buyer on its page"))
def listing_priced(world: World, price: int) -> None:
    _seed(world, price=price)
    _sign_in(world)
    _open(world)


@given(
    parsers.parse(
        "a published listing priced {price:d} whose title has a brand keyword and a buyer on its page"
    )
)
def listing_brand(world: World, price: int) -> None:
    _seed(world, price=price, title="[E2E][PDP] Apple iPhone Samsung Nike chính hãng")
    _sign_in(world)
    _open(world)


@given(parsers.parse("a published listing with {count:d} reviews and a buyer on its page"))
def listing_with_reviews(world: World, count: int) -> None:
    _seed(world)
    for i in range(count):
        author = AuthService().register(
            fake.unique_username("pdp_rev"), SETTINGS.seed_password, "buyer"
        )
        EngagementService(token=author).add_review(_listing_id(world), 1 + i % 5, f"Nhận xét #{i}")
    _sign_in(world)
    _open(world)


@given("a published listing with one review by another buyer and a buyer on its page")
def listing_one_review(world: World) -> None:
    _seed(world)
    author = AuthService().register(
        fake.unique_username("pdp_rev"), SETTINGS.seed_password, "buyer"
    )
    EngagementService(token=author).add_review(_listing_id(world), 4, "Nhận xét để bấm hữu ích")
    _sign_in(world)
    _open(world)


@given("a published listing with stock 12 and a guest on its page")
def listing_guest(world: World) -> None:
    _seed(world)
    _open(world)


@given(parsers.parse('a seller named "{name}" with a published listing and a buyer on its page'))
def seller_named(world: World, name: str) -> None:
    _seed(world, shop_name=name)
    _sign_in(world)
    _open(world, width=1280)


@given("a seller without a shop name with a published listing and a buyer on its page")
def seller_unnamed(world: World) -> None:
    _seed(world)
    _sign_in(world)
    _open(world, width=1280)


@given(
    parsers.parse('a seller named "{name}" with two priced listings and a buyer on the shop page')
)
def seller_two_listings(world: World, name: str) -> None:
    username = fake.unique_username("pdp_shop")
    token = AuthService().register(username, SETTINGS.seed_password, "seller")
    svc = ListingService(token=token)
    svc.upsert_storefront(f"e2e-shop-{uuid.uuid4().hex[:8]}", name)
    ids: dict[str, str] = {}
    for label, price in (("high", 900_000), ("low", 300_000)):
        ids[label] = svc.create_listing(
            Listing(
                title=f"[E2E][Shop sort {label}] {fake.price_vnd():d}",
                price=price,
                stock=10,
                status="published",
                description="Sản phẩm seed cho thứ tự giá của gian hàng.",
            )
        )
    _x(world)["shop_sort_ids"] = ids
    _sign_in(world)
    deadline = time.monotonic() + 45
    page = world.page
    while True:
        page.goto(f"{BASE}/shop/{_principal_id(token)}", wait_until="domcontentloaded")
        if page.locator(f'a[href="/listing/{ids["low"]}"]').count() or time.monotonic() > deadline:
            break
        page.wait_for_timeout(2000)


@given("the buyer's session token is corrupted behind the page's back")
def session_corrupted(world: World) -> None:
    token = world.state.current_user.token
    world.context.add_cookies([{"name": "session", "value": token[:-5] + "AAAAA", "url": BASE}])


@given("the listing is deleted behind the page's back")
def listing_deleted(world: World) -> None:
    ListingService(token=_x(world)["pdp_seller_token"]).delete_listing(_listing_id(world))


# ── Anatomy ──────────────────────────────────────────────────────────────
def _box(world: World, selector: str) -> dict:
    box = world.page.locator(selector).first.bounding_box()
    assert box, f"{selector} has no layout box"
    return box


@then(
    "the page shows the breadcrumb, the gallery beside the info column, the shop card, the anchor "
    "nav and the three sections in that vertical order"
)
def desktop_order(world: World) -> None:
    crumb = _box(world, "nav[aria-label=Breadcrumb]")
    gallery = _box(world, '[data-testid="gallery-stage"]')
    info = _box(world, "h1")
    shop = _box(world, '[data-testid="shop-header-card"]')
    anchor = _box(world, 'nav[aria-label="Nội dung sản phẩm"]')
    specs, reviews, qa = _box(world, "#specs"), _box(world, "#reviews"), _box(world, "#qa")
    assert gallery["x"] + gallery["width"] <= info["x"] + 1, "gallery is not beside the info column"
    assert abs(gallery["y"] - info["y"]) < 60, (gallery, info)
    order = [crumb["y"], gallery["y"], shop["y"], anchor["y"], specs["y"], reviews["y"], qa["y"]]
    assert order == sorted(order) and len(set(order)) == len(order), order
    recs = world.page.get_by_text("Gợi ý cho bạn", exact=False)
    if recs.count():
        assert recs.first.bounding_box()["y"] > qa["y"], "similar-items row is above the Q&A"


@then("exactly one h1 contains the listing title")
def one_h1(world: World) -> None:
    h1 = world.page.locator("h1")
    expect(h1).to_have_count(1)
    expect(h1).to_contain_text(_x(world)["pdp_title"])


@then(
    'the Chi tiết section lists "Kho hàng" "12 sản phẩm" and the category, followed by the description'
)
def specs_block(world: World) -> None:
    specs = world.page.locator("#specs")
    expect(specs.locator("dl")).to_be_visible()
    text = specs.inner_text()
    for needle in ("Danh mục", "Điện tử & Công nghệ", "Kho hàng", "12 sản phẩm"):
        assert needle in text, f"{needle!r} missing from {text!r}"
    assert text.index("12 sản phẩm") < text.index("Mô tả sản phẩm seed cho PDP E2E."), text
    assert specs.locator("dl").evaluate("e => e.tagName") == "DL"


@when("the token lint runs on the product detail files")
def lint_pdp(world: World) -> None:
    prefixes = [
        "src/app/(shop)/listing",
        "src/app/(shop)/shop",
        "src/features/listing",
        "src/features/review",
        "src/features/qa",
        "src/features/shop",
        "src/features/recommendations",
    ]
    _x(world)["lint"] = _run(["node", "scripts/check-tokens.mjs", *prefixes], FRONTEND)


@then("the token lint reports no violation in the product detail files")
def lint_pdp_clean(world: World) -> None:
    result = _x(world)["lint"]
    assert result.returncode == 0 and "0 violation(s)" in result.stdout, result.stdout[-1500:]


# ── Real data ────────────────────────────────────────────────────────────
@then(
    'no star rating is rendered for the listing, "Chưa có đánh giá" is shown and no sold count '
    "appears in the header or the shop card"
)
def no_stars_no_sold(world: World) -> None:
    detail = _detail(world)
    expect(detail.rating_row).to_contain_text("Chưa có đánh giá")
    assert (
        detail.rating_row.get_by_role("img").count() == 0
    ), "stars rendered for an unrated listing"
    card = world.page.get_by_test_id("shop-header-card")
    assert card.get_by_role("img", name=re.compile(r"trên 5 sao")).count() == 0
    for area in (world.page.locator("main").first,):
        assert not re.search(r"đã bán", area.inner_text(), re.I), "a sold count is shown"


@then("no Mall badge is rendered")
def no_mall(world: World) -> None:
    expect(_detail(world).price).to_be_visible(timeout=timeouts.NAVIGATION)
    body = world.page.locator("main").inner_text()
    assert not re.search(r"\bmall\b", body, re.I), "a Mall badge is rendered"
    assert not re.search(r"official store", body, re.I)


@then(
    "the strike-through shows the regular price, the price shows the campaign price and the "
    "discount badge is computed from them"
)
def flash_sale_discount(world: World) -> None:
    detail = _detail(world)
    expect(detail.price).to_contain_text("499.000", timeout=timeouts.NAVIGATION)
    strike = world.page.locator("main .line-through").first
    expect(strike).to_contain_text("5.000.000")
    badge = re.search(r"(\d+)\s?%", world.page.locator("main").inner_text())
    assert badge, "no discount badge"
    exact = (1 - 499_000 / 5_000_000) * 100
    assert int(badge.group(1)) in {int(exact), round(exact)}, (badge.group(0), exact)


# ── Gallery ──────────────────────────────────────────────────────────────
@when("the buyer clicks the third thumbnail")
def click_third_thumbnail(world: World) -> None:
    stage = world.page.get_by_test_id("gallery-stage")
    _x(world)["stage_before"] = stage.bounding_box()
    _x(world)["stage_src_before"] = stage.locator("img").first.get_attribute("src")
    world.page.get_by_role("button", name="Ảnh 3").click()


@then("the stage shows the third image and its bounding box is unchanged")
def stage_changed(world: World) -> None:
    stage = world.page.get_by_test_id("gallery-stage")
    expect(world.page.get_by_role("button", name="Ảnh 3")).to_have_attribute("aria-current", "true")
    src = stage.locator("img").first.get_attribute("src")
    assert src != _x(world)["stage_src_before"] and src.endswith("-2.png"), src
    assert stage.bounding_box() == _x(world)["stage_before"], "the stage moved or resized"


@then("the stage shows the placeholder in a 1:1 box and no external image host was requested")
def stage_placeholder(world: World) -> None:
    stage = world.page.get_by_test_id("gallery-stage")
    expect(stage.get_by_role("img", name="Không có ảnh")).to_be_visible()
    box = stage.locator(".aspect-square").first.bounding_box()
    assert box and abs(box["width"] - box["height"]) < 1, box
    hosts = world.page.evaluate(
        "() => [...new Set(performance.getEntriesByType('resource').map(r => new URL(r.name).hostname))]"
    )
    external = [h for h in hosts if h not in ("localhost", "127.0.0.1")]
    assert not external, f"external hosts requested: {external}"


@then("the stage image is not lazy and each thumbnail image is lazy")
def stage_eager_thumbs_lazy(world: World) -> None:
    stage = world.page.get_by_test_id("gallery-stage").locator("img").first
    expect(stage).to_be_attached()
    assert stage.get_attribute("loading") != "lazy", "the stage image is lazy"
    thumbs = world.page.get_by_role("list", name="Ảnh sản phẩm").locator("img")
    assert thumbs.count() == 4
    for i in range(4):
        assert thumbs.nth(i).get_attribute("loading") == "lazy", f"thumbnail {i + 1} is not lazy"


# ── Variants and purchase ────────────────────────────────────────────────
@then(
    "the out-of-stock variant is disabled, tagged Hết hàng and cannot be selected by click or keyboard"
)
def oos_variant_blocked(world: World) -> None:
    detail = _detail(world)
    radio = detail.variant_radio("512GB")
    expect(radio).to_be_disabled()
    expect(radio).to_have_attribute("aria-disabled", "true")
    expect(detail.variant_chip("512GB")).to_contain_text("Hết hàng")
    detail.variant_chip("512GB").click(force=True)
    detail.variant_radio("128GB").focus()
    world.page.keyboard.press("ArrowRight")
    world.page.keyboard.press("ArrowRight")
    world.page.keyboard.press("Space")
    world.page.wait_for_timeout(300)
    assert not radio.is_checked(), "the out-of-stock variant became selected"
    assert (
        "variant=" not in world.page.url
        or _x(world)["pdp_variant_ids"]["512GB"] not in world.page.url
    )


@then(
    "both purchase buttons and the quantity picker are disabled and a warning says the variant is "
    "out of stock"
)
def oos_disables_actions(world: World) -> None:
    detail = _detail(world)
    expect(detail.add_to_cart_button.first).to_be_disabled()
    expect(detail.buy_now_button.first).to_be_disabled()
    expect(world.page.get_by_text("Phân loại này đã hết hàng")).to_be_visible()
    expect(detail.quantity_increase).to_be_disabled()
    expect(world.page.get_by_role("button", name="Giảm số lượng")).to_be_disabled()


@when("the buyer clicks Thêm vào giỏ")
def click_add(world: World) -> None:
    detail = _detail(world)
    LoginPageWait.wait(world, detail.add_to_cart_button.first)
    _x(world)["add_events_before"] = _data_layer(world, "add_to_cart")
    detail.add_to_cart_button.first.click()


class LoginPageWait:
    @staticmethod
    def wait(world: World, locator: Locator) -> None:
        from src.pages import LoginPage

        LoginPage(world.page).wait_until_interactive(locator)


def _data_layer(world: World, name: str) -> list[dict]:
    events = world.page.evaluate("() => (window.dataLayer || []).filter(e => e && e.event)")
    return [e for e in events if e.get("event") == name]


@then(
    "an error toast is shown, no add_to_cart event was fired and both purchase buttons are enabled again"
)
def add_failed(world: World) -> None:
    alert = world.page.get_by_role("alert").filter(has_text=re.compile(r"\S"))
    expect(alert.first).to_be_visible(timeout=timeouts.NAVIGATION)
    detail = _detail(world)
    expect(detail.add_to_cart_button.first).to_be_enabled(timeout=timeouts.DEFAULT)
    expect(detail.buy_now_button.first).to_be_enabled()
    assert _data_layer(world, "add_to_cart") == [], "an add_to_cart event was fired"


@when(parsers.parse("the buyer adds quantity {quantity:d} to the cart"))
def add_quantity(world: World, quantity: int) -> None:
    detail = _detail(world)
    LoginPageWait.wait(world, detail.add_to_cart_button.first)
    for _ in range(quantity - 1):
        detail.quantity_increase.click()
    detail.add_to_cart_button.first.click()
    expect(_toast(world, "Đã thêm")).to_be_visible(timeout=timeouts.NAVIGATION)


@then(
    "one add_to_cart event carries value 200000, currency VND and the item id, name, price, "
    "quantity and category"
)
def add_event(world: World) -> None:
    events = _data_layer(world, "add_to_cart")
    assert len(events) == 1, events
    ecommerce = events[0]["ecommerce"]
    assert ecommerce["value"] == 200_000 and ecommerce["currency"] == "VND", ecommerce
    (item,) = ecommerce["items"]
    assert item["item_id"] == _listing_id(world), item
    assert item["item_name"] == _x(world)["pdp_title"], item
    assert item["price"] == 100_000 and item["quantity"] == 2, item
    assert item.get("item_category"), item


@when('the buyer selects the variant "256GB" and uses the anchor nav')
def variant_and_anchor(world: World) -> None:
    detail = _detail(world)
    LoginPageWait.wait(world, detail.variant_chip("256GB"))
    detail.variant_chip("256GB").click()
    expect(detail.price).to_contain_text("150.000")
    detail.anchor_link("Đánh giá").click()
    world.page.wait_for_timeout(500)


@then(
    "exactly one view_item event was sent and the variant change and the anchor navigation sent none"
)
def one_view_item(world: World) -> None:
    world.page.wait_for_timeout(4000)  # the beacon queue flushes on a short timer
    views = _data_layer(world, "view_item")
    assert len(views) == 1, f"expected one view_item, got {len(views)}"
    assert views[0]["ecommerce"]["items"][0]["item_id"] == _listing_id(world)
    beacon_views = [
        b
        for b in _x(world)["beacons"]
        if b.get("type") == "view" and b.get("listingId") == _listing_id(world)
    ]
    assert len(beacon_views) == 1, beacon_views


@when('the buyer scrolls to the reviews section and selects the variant "256GB"')
def scroll_reviews_select(world: World) -> None:
    detail = _detail(world)
    LoginPageWait.wait(world, detail.variant_chip("256GB"))
    detail.variant_chip("256GB").click()
    world.page.locator("#reviews").scroll_into_view_if_needed()


@then(parsers.parse('the buy bar is visible with the price "{price}"'))
def bar_price(world: World, price: str) -> None:
    bar = _detail(world).buy_bar
    expect(bar).to_be_in_viewport()
    expect(bar).to_contain_text(price)


@when("the buyer clicks Thêm vào giỏ in the buy bar")
def click_bar_add(world: World) -> None:
    world.page.get_by_test_id("bar-add-to-cart").click()


@then(parsers.parse('a toast "{text}" appears and the cart counter shows {count:d}'))
def toast_and_counter(world: World, text: str, count: int) -> None:
    expect(_toast(world, text)).to_be_visible(timeout=timeouts.NAVIGATION)
    cart = world.page.locator('header a[href="/cart"]').first
    expect(cart).to_contain_text(str(count), timeout=timeouts.DEFAULT)


def _visible_add_buttons(world: World) -> int:
    buttons = world.page.get_by_role("button", name=re.compile(r"Thêm vào giỏ", re.I))
    return sum(1 for i in range(buttons.count()) if buttons.nth(i).is_visible())


@then("exactly one Thêm vào giỏ button is visible and the buy bar is rendered")
def one_button_with_bar(world: World) -> None:
    assert _visible_add_buttons(world) == 1
    expect(_detail(world).buy_bar).to_be_visible()


@when(parsers.parse("the viewport becomes {width:d} pixels wide"))
def viewport_becomes(world: World, width: int) -> None:
    world.page.set_viewport_size({"width": width, "height": 812 if width < 768 else 900})
    world.page.wait_for_timeout(300)


@then("exactly one Thêm vào giỏ button is visible and no buy bar is rendered")
def one_button_no_bar(world: World) -> None:
    assert _visible_add_buttons(world) == 1
    assert not _detail(world).buy_bar.is_visible()


@when("the buyer scrolls to the bottom of the page")
def scroll_bottom(world: World) -> None:
    world.page.evaluate("() => window.scrollTo(0, document.documentElement.scrollHeight)")
    world.page.wait_for_timeout(400)


@then("the last element of the page is fully visible above the buy bar")
def last_element_clear(world: World) -> None:
    bar = _detail(world).buy_bar.bounding_box()
    assert bar, "buy bar not rendered"
    last = world.page.evaluate("""() => {
          const els = [...document.querySelectorAll('body *')].filter((e) => {
            const r = e.getBoundingClientRect(); return r.height > 0 && r.width > 0
              && getComputedStyle(e).position !== 'fixed' && !e.closest('[data-testid=buy-bar]'); });
          const bottomMost = els.reduce((a, b) =>
            (a.getBoundingClientRect().bottom >= b.getBoundingClientRect().bottom ? a : b));
          const r = bottomMost.getBoundingClientRect();
          return {bottom: r.bottom, tag: bottomMost.tagName};
        }""")
    assert last["bottom"] <= bar["y"] + 1, f"content ({last}) is covered by the buy bar at {bar}"


@then("the anchor nav is sticky")
def anchor_sticky(world: World) -> None:
    nav = _detail(world).anchor_nav
    position = nav.evaluate(
        "e => { let el = e; while (el) { const p = getComputedStyle(el).position;"
        " if (p === 'sticky') return p; el = el.parentElement; } return getComputedStyle(e).position; }"
    )
    assert position == "sticky", position


@then("the anchor nav is not sticky and scrolls horizontally without page overflow")
def anchor_not_sticky(world: World) -> None:
    nav = _detail(world).anchor_nav
    chain = nav.evaluate(
        "e => { const out = []; let el = e; while (el && el !== document.body) {"
        " out.push(getComputedStyle(el).position); el = el.parentElement; } return out; }"
    )
    assert "sticky" not in chain, chain
    scrolls = nav.evaluate(
        "e => [e, ...e.querySelectorAll('*')].some(n => ['auto','scroll'].includes(getComputedStyle(n).overflowX))"
    )
    assert scrolls, "the anchor nav does not scroll horizontally"
    assert world.page.evaluate("() => document.documentElement.scrollWidth") <= _PHONE["width"]


# ── Reviews ──────────────────────────────────────────────────────────────
@when("the buyer opens the third page of reviews")
def open_third_review_page(world: World) -> None:
    world.page.goto(f"{BASE}/listing/{_listing_id(world)}?rpage=3", wait_until="domcontentloaded")


@then("3 reviews are listed and the pagination marks page 3 as current")
def third_page(world: World) -> None:
    expect(_detail(world).review_items).to_have_count(3, timeout=timeouts.NAVIGATION)
    current = world.page.locator("#reviews").get_by_role("link", name="Trang 3")
    expect(current).to_have_attribute("aria-current", "page")


def _helpful_count(world: World) -> int:
    text = _detail(world).review_helpful_buttons.first.inner_text()
    found = re.search(r"\d+", text)
    return int(found.group(0)) if found else 0


@when("the buyer marks that review as helpful")
def mark_helpful(world: World) -> None:
    button = _detail(world).review_helpful_buttons.first
    LoginPageWait.wait(world, button)
    _x(world)["helpful_before"] = _helpful_count(world)
    button.click()


@then("the helpful count increases by one and the button is disabled")
def helpful_increased(world: World) -> None:
    button = _detail(world).review_helpful_buttons.first
    expect(button).to_be_disabled(timeout=timeouts.NAVIGATION)
    assert _helpful_count(world) == _x(world)["helpful_before"] + 1


@then("the new count comes from the server without a reload and survives a reload")
def helpful_revalidated(world: World) -> None:
    expect(_detail(world).review_helpful_buttons.first).to_be_disabled(timeout=timeouts.NAVIGATION)
    expected = _x(world)["helpful_before"] + 1
    assert _helpful_count(world) == expected
    world.page.reload(wait_until="domcontentloaded")
    expect(_detail(world).review_items.first).to_be_visible(timeout=timeouts.NAVIGATION)
    assert _helpful_count(world) == expected, "the stored count differs from the displayed one"


@when("the buyer picks 5 stars, types a comment and submits the review modal")
def submit_review(world: World) -> None:
    write = world.page.get_by_test_id("write-review").first
    LoginPageWait.wait(world, write)
    write.click()
    dialog = world.page.get_by_role("dialog", name="Đánh giá sản phẩm")
    expect(dialog).to_be_visible()
    dialog.locator('label:has(input[aria-label="5 sao"])').click()
    dialog.get_by_label("Nhận xét chi tiết").fill("Sản phẩm rất tốt, giao nhanh")
    _x(world)["review_dialog"] = dialog
    dialog.get_by_role("button", name="Hoàn thành").click()


@then("the submit button is pending and the modal controls are disabled")
def review_pending(world: World) -> None:
    dialog = _x(world)["review_dialog"]
    submit = dialog.get_by_role("button", name="Hoàn thành")
    expect(submit).to_have_attribute("aria-busy", "true", timeout=timeouts.SHORT)
    expect(dialog.get_by_label("Nhận xét chi tiết")).to_be_disabled()


@then("a success toast shows, the modal closes and the new review is listed")
def review_done(world: World) -> None:
    expect(_toast(world).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.NAVIGATION)
    expect(
        _detail(world).review_items.filter(has_text="Sản phẩm rất tốt, giao nhanh")
    ).to_have_count(1, timeout=timeouts.NAVIGATION)


# ── Q&A ──────────────────────────────────────────────────────────────────
@when("the buyer types a question and submits")
def ask_question(world: World) -> None:
    form = world.page.get_by_test_id("qa-ask-form")
    area = form.get_by_label("Câu hỏi của bạn")
    LoginPageWait.wait(world, area)
    _x(world)["question"] = f"Sản phẩm này bảo hành bao lâu? {uuid.uuid4().hex[:6]}"
    area.fill(_x(world)["question"])
    _x(world)["ask_button"] = form.get_by_role("button", name="Đặt câu hỏi")
    _x(world)["ask_button"].click()


@then(
    "the submit button is pending, then a success toast appears, the textarea clears and the "
    "question is listed"
)
def question_asked(world: World) -> None:
    button = _x(world)["ask_button"]
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.SHORT)
    expect(_toast(world).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_test_id("qa-ask-form").get_by_label("Câu hỏi của bạn")).to_have_value(
        ""
    )
    expect(
        world.page.get_by_test_id("qa-item").filter(has_text=_x(world)["question"])
    ).to_have_count(1, timeout=timeouts.NAVIGATION)


@then("the ask button is disabled while the textarea is empty and no request is made")
def ask_disabled(world: World) -> None:
    posts: list[str] = []
    world.page.on(
        "request",
        lambda r: (
            posts.append(r.url) if r.method == "POST" and r.headers.get("next-action") else None
        ),
    )
    form = world.page.get_by_test_id("qa-ask-form")
    button = form.get_by_role("button", name="Đặt câu hỏi")
    expect(button).to_be_disabled()
    button.click(force=True, no_wait_after=True)
    world.page.wait_for_timeout(500)
    assert posts == [], posts


@then("the login prompt links to the login page with the return URL and no ask form renders")
def guest_prompt(world: World) -> None:
    prompt = world.page.get_by_test_id("qa-login-prompt")
    expect(prompt).to_be_visible(timeout=timeouts.NAVIGATION)
    link = prompt.get_by_role("link")
    expect(link.first).to_have_attribute("href", f"/login?returnUrl=/listing/{_listing_id(world)}")
    expect(world.page.get_by_test_id("qa-ask-form")).to_have_count(0)


@then("an error toast and an inline alert show a readable message and the page does not crash")
def question_failed(world: World) -> None:
    alerts = world.page.get_by_role("alert").filter(has_text=re.compile(r"\S"))
    expect(alerts.first).to_be_visible(timeout=timeouts.NAVIGATION)
    assert alerts.count() >= 2, "expected both an error toast and an inline alert"
    assert "Đã có lỗi xảy ra" not in world.page.content()
    expect(world.page.get_by_test_id("qa-ask-form")).to_be_visible()


# ── Shop card ────────────────────────────────────────────────────────────
@then(parsers.parse('the PDP shop card shows "{name}" and not "Shop #"'))
def pdp_shop_name(world: World, name: str) -> None:
    card = world.page.get_by_test_id("shop-header-card")
    expect(card.get_by_test_id("shop-name")).to_have_text(name, timeout=timeouts.NAVIGATION)
    assert "Shop #" not in card.inner_text()


@then('the PDP shop card shows "Shop #" and the first 6 characters of the seller id')
def pdp_shop_fallback(world: World) -> None:
    seller_id = _x(world)["pdp_seller_id"]
    expect(world.page.get_by_test_id("shop-header-card").get_by_test_id("shop-name")).to_have_text(
        f"Shop #{seller_id[:6]}", timeout=timeouts.NAVIGATION
    )


@when("the buyer opens the shop")
def open_shop(world: World) -> None:
    world.page.get_by_test_id("shop-header-card").get_by_role("link", name="Xem Shop").click()
    world.page.wait_for_url(re.compile(r".*/shop/"), timeout=timeouts.NAVIGATION)


@then(parsers.parse('the storefront header shows "{name}"'))
def storefront_name(world: World, name: str) -> None:
    card = world.page.get_by_test_id("shop-header-card")
    expect(card.get_by_test_id("shop-name")).to_have_text(name, timeout=timeouts.NAVIGATION)


@then("the browser is on the seller's storefront and its header is the hero shop card")
def storefront_hero(world: World) -> None:
    seller_id = _x(world)["pdp_seller_id"]
    expect(world.page).to_have_url(re.compile(rf".*/shop/{seller_id}$"))
    card = world.page.get_by_test_id("shop-header-card")
    expect(card).to_be_visible(timeout=timeouts.NAVIGATION)
    variant = card.get_attribute("data-variant")
    assert variant in (None, "hero"), f"storefront header variant is {variant!r}"
    pdp_box = None  # the PDP card was the compact one; the storefront card is taller
    assert card.bounding_box()["height"] > 100, pdp_box


@then('the shop card shows "Chưa có đánh giá" instead of "0.0 / 5.0"')
def shop_no_rating(world: World) -> None:
    summary = world.page.get_by_test_id("shop-header-card").get_by_test_id("shop-rating-summary")
    expect(summary).to_contain_text("Chưa có đánh giá", timeout=timeouts.NAVIGATION)
    assert "0.0" not in summary.inner_text()


# ── Not found, loading, similar items ────────────────────────────────────
@when(parsers.parse('the visitor requests the unknown listing "{listing_id}"'))
def request_unknown(world: World, listing_id: str) -> None:
    _x(world)["last_response"] = world.page.goto(
        f"{BASE}/listing/{listing_id}", wait_until="domcontentloaded"
    )


@then(
    parsers.parse('the response status is 404 and the Result offers "{primary}" and "{secondary}"')
)
def not_found_result(world: World, primary: str, secondary: str) -> None:
    assert _x(world)["last_response"].status == 404
    expect(world.page.get_by_role("heading", name="Không tìm thấy sản phẩm")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(world.page.get_by_role("link", name=primary)).to_have_attribute("href", "/")
    expect(world.page.get_by_role("link", name=secondary)).to_have_attribute("href", "/search")


@when("the buyer loads the listing over a throttled network")
def load_throttled(world: World) -> None:
    cdp = world.context.new_cdp_session(world.page)
    cdp.send("Network.enable")
    cdp.send(
        "Network.emulateNetworkConditions",
        {
            "offline": False,
            "latency": 300,
            "downloadThroughput": 200_000,
            "uploadThroughput": 100_000,
        },
    )
    cdp.send("Emulation.setCPUThrottlingRate", {"rate": 4})
    # Shifts are attributed to the page content (<main>); the global disclaimer banner above it
    # re-wraps once its web font loads, which is outside this route and recorded separately.
    world.page.add_init_script("""window.__cls = 0; window.__shifts = [];
        new PerformanceObserver((l) => { for (const e of l.getEntries()) {
          if (e.hadRecentInput) continue;
          const inMain = e.sources.some((s) => { const n = s.node && (s.node.nodeType === 3
            ? s.node.parentElement : s.node); return n && n.closest && n.closest('main'); });
          window.__shifts.push({value: e.value, inMain, sources: e.sources.map((s) =>
            (s.node ? s.node.nodeName + ' ' + (s.node.textContent || '').slice(0, 30) : '?') +
            ' y ' + Math.round(s.previousRect.y) + '->' + Math.round(s.currentRect.y))});
          if (inMain) window.__cls += e.value; } })
          .observe({type: 'layout-shift', buffered: true});""")
    world.page.goto(f"{BASE}/listing/{_listing_id(world)}", wait_until="load")
    expect(_detail(world).add_to_cart_button.first).to_be_visible(timeout=timeouts.NAVIGATION)
    world.page.wait_for_timeout(3000)


@then("the measured layout shift of the page is 0")
def cls_zero(world: World) -> None:
    cls = world.page.evaluate("() => window.__cls")
    shifts = world.page.evaluate("() => window.__shifts")
    assert cls == 0, f"cumulative layout shift is {cls}, expected 0: {shifts}"


@when("the similar-items row scrolls into view and the buyer clicks the second card")
def click_second_similar(world: World) -> None:
    titles = world.page.locator("[data-recs-request-id] h3 a")
    expect(titles.nth(1)).to_be_visible(timeout=timeouts.LONG)
    second_href = titles.nth(1).get_attribute("href")
    # The image link of the card (the first anchor with this href) carries the attribution.
    cards = world.page.locator(f'[data-recs-request-id] a[href="{second_href}"]')
    cards.first.scroll_into_view_if_needed()
    beacons: list[dict] = _x(world)["beacons"]
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline and not any(
        b.get("type") == "impression" and b.get("placementId") == "similar_items" for b in beacons
    ):
        world.page.wait_for_timeout(500)
    cards.first.click()
    world.page.wait_for_url(
        re.compile(r".*/listing/(?!" + _listing_id(world) + ")"), timeout=timeouts.NAVIGATION
    )


@then(
    "similar_items impressions for positions 1..n and a similar_items click for position 2 were sent"
)
def similar_beacons(world: World) -> None:
    beacons: list[dict] = _x(world)["beacons"]
    deadline = time.monotonic() + 12
    while time.monotonic() < deadline and not any(
        b.get("type") == "click" and b.get("placementId") == "similar_items" for b in beacons
    ):
        world.page.wait_for_timeout(500)
    positions = sorted(
        {
            b["position"]
            for b in beacons
            if b.get("type") == "impression" and b.get("placementId") == "similar_items"
        }
    )
    assert positions and positions == list(range(1, len(positions) + 1)), (positions, beacons)
    clicks = [
        b for b in beacons if b.get("type") == "click" and b.get("placementId") == "similar_items"
    ]
    assert [c.get("position") for c in clicks] == [2], (
        clicks,
        [b for b in beacons if b.get("type") != "impression"],
    )
