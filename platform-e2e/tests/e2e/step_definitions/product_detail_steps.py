"""Steps for frontend/product_detail.feature (ui-phase-product-detail).

Variant listings are seeded through the gateway by the `@needsSeller` seller; the
server assigns the variant ids, which are read back so URL assertions can compare
`?variant=<id>`. UI assertions go through `ListingDetailPage`.
"""

from __future__ import annotations

import re
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import ListingService
from src.constants import PageName, timeouts
from src.models import Listing
from src.pages import ListingDetailPage
from src.utils import data as fake
from tests.e2e.support.world import World

_OUT_OF_STOCK_NAME = "512GB"


def _detail(world: World) -> ListingDetailPage:
    page: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    return page


def _listing_id(world: World) -> str:
    listing = world.state.listing
    assert listing and listing.listing_id, "No seeded listing in state"
    return listing.listing_id


def _variant_id(world: World, name: str) -> str:
    ids: dict[str, str] = world.state.extra["variant_ids"]
    assert name in ids, f"variant {name!r} was not seeded (have {sorted(ids)})"
    return ids[name]


def _variant_param(url: str) -> str | None:
    values = parse_qs(urlparse(url).query).get("variant")
    return values[0] if values else None


# ── Seeding ──────────────────────────────────────────────────────────────
@given(
    parsers.parse(
        'a seeded listing with variants "{first}" at {first_price:d} and "{second}" at {second_price:d}'
    )
)
def seed_variant_listing(
    world: World, first: str, first_price: int, second: str, second_price: int
) -> None:
    seller = world.state.seeded_seller
    assert seller and seller.token, "No seeded seller in state (needs @needsSeller)"
    listing = Listing(
        title=f"[E2E][PDP] {fake.price_vnd():d}",
        price=first_price,
        stock=10,
        status="published",
        description="Sản phẩm có phân loại, seed tự động cho PDP E2E.",
    )
    svc = ListingService(token=seller.token)
    listing_id = svc.create_listing(
        listing,
        variants=[
            {"name": first, "sku": "SKU-A", "price": first_price, "stock": 10},
            {"name": second, "sku": "SKU-B", "price": second_price, "stock": 3},
            {"name": _OUT_OF_STOCK_NAME, "sku": "SKU-C", "price": second_price * 2, "stock": 0},
        ],
    )
    assert listing_id, "variant listing was not created"
    stored = svc.get_listing(listing_id)
    world.state.extra["variant_ids"] = {v["name"]: v["id"] for v in stored.get("variants", [])}
    world.state.listing = listing


# ── Navigation ───────────────────────────────────────────────────────────
@when("the buyer opens the variant listing")
def open_variant_listing(world: World) -> None:
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=_listing_id(world))
    expect(_detail(world).add_to_cart_button).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the buyer opens the variant listing with the "{name}" variant in the URL'))
def open_variant_listing_with_variant(world: World, name: str) -> None:
    url = f"{world.settings.base_url.rstrip('/')}/listing/{_listing_id(world)}?variant={_variant_id(world, name)}"
    world.page.goto(url, wait_until="domcontentloaded")
    expect(_detail(world).add_to_cart_button).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the buyer opens the variant listing with an unknown variant in the URL")
def open_variant_listing_unknown_variant(world: World) -> None:
    url = (
        f"{world.settings.base_url.rstrip('/')}/listing/{_listing_id(world)}?variant=does-not-exist"
    )
    world.page.goto(url, wait_until="domcontentloaded")
    expect(_detail(world).add_to_cart_button).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the visitor opens the unknown listing "{listing_id}"'))
def open_unknown_listing(world: World, listing_id: str) -> None:
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=listing_id)


@when(parsers.parse("the viewport is {width:d} pixels wide"))
def set_viewport_width(world: World, width: int) -> None:
    world.page.set_viewport_size({"width": width, "height": 812})


@when("the buyer scrolls to the reviews section")
def scroll_to_reviews(world: World) -> None:
    world.page.locator("#reviews").scroll_into_view_if_needed()


# ── Variant selection ────────────────────────────────────────────────────
@when(parsers.parse('the buyer selects the variant "{name}"'))
def select_variant(world: World, name: str) -> None:
    world.state.extra["history_before"] = world.page.evaluate("() => history.length")
    _detail(world).variant_chip(name).click()


@then(parsers.parse('the URL carries the "{name}" variant id without an extra history entry'))
def url_carries_variant(world: World, name: str) -> None:
    expected = _variant_id(world, name)
    world.page.wait_for_function(
        "(id) => new URL(location.href).searchParams.get('variant') === id",
        arg=expected,
        timeout=timeouts.DEFAULT,
    )
    assert _variant_param(world.page.url) == expected
    after = world.page.evaluate("() => history.length")
    assert after == world.state.extra["history_before"], "variant selection added a history entry"


@then(parsers.parse('the price shows "{text}"'))
def price_shows(world: World, text: str) -> None:
    expect(_detail(world).price).to_contain_text(text, timeout=timeouts.DEFAULT)


@then(parsers.parse('the stock line shows "{text}"'))
def stock_line_shows(world: World, text: str) -> None:
    expect(_detail(world).stock_line).to_contain_text(text, timeout=timeouts.DEFAULT)


@then(parsers.parse('the "{name}" variant is selected'))
def variant_selected(world: World, name: str) -> None:
    expect(_detail(world).variant_radio(name).first).to_be_checked(timeout=timeouts.DEFAULT)


@then(parsers.parse('the "{name}" variant is disabled with an out-of-stock tag'))
def variant_disabled(world: World, name: str) -> None:
    radio = _detail(world).variant_radio(name)
    expect(radio).to_be_disabled(timeout=timeouts.DEFAULT)
    expect(radio).to_have_attribute("aria-disabled", "true")
    expect(_detail(world).variant_chip(name)).to_contain_text("Hết hàng")


# ── Purchase actions ─────────────────────────────────────────────────────
@when("the add-to-cart request is slowed down")
def slow_add_to_cart(world: World) -> None:
    def delay(route) -> None:  # noqa: ANN001
        # Server Actions are POSTs carrying the Next-Action header.
        if route.request.method == "POST" and route.request.headers.get("next-action"):
            world.page.wait_for_timeout(800)
        route.continue_()

    world.page.route("**/listing/**", delay)


@when(parsers.parse("the buyer increases the quantity {times:d} times"))
def increase_quantity(world: World, times: int) -> None:
    plus = _detail(world).quantity_increase
    for _ in range(times):
        if plus.is_enabled():
            plus.click()


@when("the buyer clicks Thêm vào giỏ")
def click_add_to_cart(world: World) -> None:
    _detail(world).add_to_cart_button.click()


@when("the buyer clicks Mua ngay")
def click_buy_now(world: World) -> None:
    _detail(world).buy_now_button.click()


@then("both purchase buttons are disabled and the clicked one is busy")
def buttons_busy(world: World) -> None:
    detail = _detail(world)
    expect(detail.add_to_cart_button).to_have_attribute("aria-busy", "true", timeout=timeouts.SHORT)
    expect(detail.add_to_cart_button).to_be_disabled()
    expect(detail.buy_now_button).to_be_disabled()


@then(parsers.parse('a toast "{text}" appears'))
def toast_appears(world: World, text: str) -> None:
    expect(_detail(world).toast(text)).to_be_visible(timeout=timeouts.DEFAULT)


@then("both purchase buttons are enabled again")
def buttons_enabled(world: World) -> None:
    detail = _detail(world)
    expect(detail.add_to_cart_button).to_be_enabled(timeout=timeouts.DEFAULT)
    expect(detail.buy_now_button).to_be_enabled(timeout=timeouts.DEFAULT)


@then(parsers.parse("the quantity is {value:d} and the increase control is disabled"))
def quantity_at_max(world: World, value: int) -> None:
    detail = _detail(world)
    expect(detail.quantity_input).to_have_value(str(value))
    expect(detail.quantity_increase).to_be_disabled()


@then(parsers.parse('the browser navigates to "{path}"'))
def browser_navigates_to(world: World, path: str) -> None:
    expect(world.page).to_have_url(
        re.compile(rf".*{re.escape(path)}(\?.*)?$"), timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the cart contains the "{name}" variant'))
def cart_contains_variant(world: World, name: str) -> None:
    cart = world.service_factory.cart.get_cart()
    assert _variant_id(world, name) in str(cart), f"cart does not hold variant {name!r}: {cart}"


# ── Mobile buy bar ───────────────────────────────────────────────────────
@then(parsers.parse('the buy bar is visible with the price "{text}"'))
def buy_bar_visible(world: World, text: str) -> None:
    bar = _detail(world).buy_bar
    expect(bar).to_be_visible(timeout=timeouts.DEFAULT)
    expect(bar).to_contain_text(text)


@then("no buy bar is visible")
def no_buy_bar(world: World) -> None:
    expect(_detail(world).buy_bar).to_be_hidden(timeout=timeouts.DEFAULT)


@then("exactly one Thêm vào giỏ button is visible")
def exactly_one_add_button(world: World) -> None:
    expect(_detail(world).add_to_cart_button).to_have_count(1, timeout=timeouts.DEFAULT)
    expect(_detail(world).add_to_cart_button).to_be_visible()


# ── Anchor nav and sections ──────────────────────────────────────────────
@then("the anchor nav links to the specs, reviews and Q&A sections")
def anchor_links_present(world: World) -> None:
    detail = _detail(world)
    expect(detail.anchor_nav).to_be_visible(timeout=timeouts.DEFAULT)
    for label, anchor in (("Chi tiết", "#specs"), ("Đánh giá", "#reviews"), ("Hỏi đáp", "#qa")):
        expect(detail.anchor_link(label)).to_have_attribute("href", anchor)
        expect(world.page.locator(anchor)).to_have_count(1)


@when(parsers.parse('the buyer follows the anchor link "{label}"'))
def follow_anchor_link(world: World, label: str) -> None:
    _detail(world).anchor_link(label).click()


@then("the reviews section is in view")
def reviews_in_view(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*#reviews$"), timeout=timeouts.DEFAULT)
    expect(world.page.locator("#reviews")).to_be_in_viewport(timeout=timeouts.DEFAULT)


# ── Real data only ───────────────────────────────────────────────────────
@then(parsers.parse('the rating row reads "{text}"'))
def rating_row_reads(world: World, text: str) -> None:
    row = _detail(world).rating_row
    expect(row).to_contain_text(text, timeout=timeouts.DEFAULT)
    expect(row.get_by_role("img")).to_have_count(0)


@then("no sold count, Mall badge or strike-through price is shown")
def no_invented_values(world: World) -> None:
    body = world.page.locator("body")
    for fake_text in ("Đã Bán", "Đã bán", "Official Store Partner", "Tỉ Lệ Phản Hồi"):
        expect(body).not_to_contain_text(fake_text)
    expect(world.page.get_by_text("Mall", exact=True)).to_have_count(0)
    expect(_detail(world).price.locator(".line-through")).to_have_count(0)


# ── Not found ────────────────────────────────────────────────────────────
@then("the product not-found result offers the home page and search")
def product_not_found(world: World) -> None:
    detail = _detail(world)
    expect(detail.not_found_title).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("link", name="Về trang chủ")).to_have_attribute("href", "/")
    expect(world.page.get_by_role("link", name="Tìm sản phẩm khác")).to_have_attribute(
        "href", "/search"
    )
