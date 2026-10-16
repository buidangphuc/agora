"""Steps of frontend/advertised_price_ui.feature (change frontend-advertised-price).

Black box: the browser drives the PDP and the checkout wizard; accounts, listings, flash
campaigns and carts are seeded through the gateway, and the expected figures are read from the
real CalculateShippingFee and ListBuyerOrders, never recomputed from a copy of the rule.
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.flows.oic_inv_flow import poll
from tests.e2e.step_definitions.d2_checkout_steps import summary_rows
from tests.e2e.support import advertised_price_ui_support as sup
from tests.e2e.support import d2_support as d2
from tests.e2e.support import flash_support as fs
from tests.e2e.support.world import World

ALL_CITIES = ("Hà Nội", "Đà Nẵng", "Cần Thơ", "Hồ Chí Minh")


# ── Given ────────────────────────────────────────────────────────────────
@given(parsers.parse("an advertised-price listing priced {price:d}"))
def listing(world: World, price: int) -> None:
    sup.state(world)["shop"] = d2.seed_shop(price=price)


@given(parsers.parse('an advertised-price buyer "{label}" with the default address city "{city}"'))
def named_buyer(world: World, label: str, city: str) -> None:
    sup.state(world)["buyers"][label] = sup.register_buyer(city)


def _cart_buyer(world: World, city: str, regular: int) -> dict:
    """A buyer (browser logged in) whose cart holds one unit of a fresh listing at `regular`."""
    st = sup.state(world)
    d2.seed_buyer(world, address_city=city)
    shop = d2.seed_shop(price=regular)
    d2.add_to_cart(world, shop)
    st.update(shop=shop, city=city, regular=regular)
    st["buyer_token"] = world.state.extra["d2_buyer"].token
    return shop


@given(
    parsers.parse(
        'an advertised-price buyer in "{city}" has a cart with one flash-priced item '
        "regular {regular:d} and sale {sale:d}"
    )
)
def buyer_with_flash_cart(world: World, city: str, regular: int, sale: int) -> None:
    st = sup.state(world)
    # The campaign exists before the add, so the cart line is priced at the sale price.
    d2.seed_buyer(world, address_city=city)
    shop = d2.seed_shop(price=regular)
    st["campaign"] = fs.create_campaign(shop["token"], shop["listing_id"], sale, 10)
    d2.add_to_cart(world, shop)
    st.update(shop=shop, city=city, regular=regular, sale=sale)
    st["buyer_token"] = world.state.extra["d2_buyer"].token
    lines = d2.buyer_cart(world).get_cart().get("cart", {}).get("items", [])
    assert [int(line.get("unitPrice", 0)) for line in lines] == [sale], (
        f"the cart line is not at the flash price: {lines}"
    )


@given(
    parsers.parse(
        'an advertised-price buyer in "{city}" has a cart with one regular-priced item '
        "priced {regular:d}"
    )
)
def buyer_with_regular_cart(world: World, city: str, regular: int) -> None:
    _cart_buyer(world, city, regular)


# ── When ─────────────────────────────────────────────────────────────────
def _open_listing(world: World) -> None:
    shop = sup.state(world)["shop"]
    world.page.goto(
        f"{world.settings.base_url}/listing/{shop['listing_id']}", wait_until="domcontentloaded"
    )


@when(parsers.parse('"{label}" opens the advertised-price listing page'))
def buyer_opens_pdp(world: World, label: str) -> None:
    buyer = sup.state(world)["buyers"][label]
    sup.sign_in_browser(world, buyer["token"])
    _open_listing(world)


@when("an anonymous visitor opens the advertised-price listing page")
def anonymous_opens_pdp(world: World) -> None:
    sup.sign_in_browser(world, None)
    _open_listing(world)


@when(parsers.parse('the real shipping fee is asked for subtotal {subtotal:d} in "{city}"'))
def ask_fee(world: World, subtotal: int, city: str) -> None:
    token = sup.state(world)["buyers"]["hn"]["token"]
    sup.state(world)["fee"] = sup.shipping_fee(token, city, subtotal)


@when("the flash campaign of the cart item is switched off")
def campaign_off(world: World) -> None:
    fs.end_campaign(sup.state(world)["campaign"])


@when(parsers.parse("a flash campaign with sale price {sale:d} starts on the cart item"))
def campaign_starts(world: World, sale: int) -> None:
    st = sup.state(world)
    st["campaign"] = fs.create_campaign(st["shop"]["token"], st["shop"]["listing_id"], sale, 10)
    st["sale"] = sale


@when("the browser is signed out and requests the checkout page")
def signed_out_requests_checkout(world: World) -> None:
    sup.sign_in_browser(world, None)
    world.page.goto(f"{world.settings.base_url}/checkout", wait_until="domcontentloaded")


# ── Then: PDP ────────────────────────────────────────────────────────────
def _row_text(world: World) -> str:
    row = sup.shipping_row(world.page)
    expect(row).to_contain_text("500.000", timeout=timeouts.DEFAULT)
    return row.inner_text()


@then(
    parsers.parse(
        'the shipping row states the threshold 500.000, the city "{city}" and the server fee '
        'for it, and not the city "{other}"'
    )
)
def row_states_city_fee(world: World, city: str, other: str) -> None:
    st = sup.state(world)
    label = "hn" if city == "Hà Nội" else "dn"
    token = st["buyers"][label]["token"]
    text = _row_text(world)  # the page first: it must state the threshold
    fee = sup.shipping_fee(token, city, 0)["fee"]
    assert fee > 0, f"the server returned no below-threshold fee for {city}"
    assert city in text, text
    assert other not in text, f"another buyer's city leaked into the row: {text}"
    assert sup.vnd(fee) in text, f"server fee {fee} not in the row: {text}"
    for removed in sup.REMOVED_PDP_TEXTS:
        assert removed not in world.page.inner_text("body")
    # a known city gets its own fee, not the whole range
    if fee == 20000:
        assert sup.vnd(35000) not in text, text
    if fee == 35000:
        assert sup.vnd(20000) not in text, text


@then(
    "the shipping row states the threshold 500.000 and the range 20.000 to 35.000 and names no city"
)
def row_states_range(world: World) -> None:
    text = _row_text(world)
    assert sup.vnd(20000) in text and sup.vnd(35000) in text, text
    for city in ALL_CITIES:
        assert city not in text, f"anonymous row names {city}: {text}"


@then("none of the removed shipping and insurance texts is on the page")
def removed_texts_absent(world: World) -> None:
    body = world.page.inner_text("body")
    for removed in sup.REMOVED_PDP_TEXTS:
        assert removed not in body, f"removed text rendered: {removed}"


# ── Then: parity with the server rule ────────────────────────────────────
@then("the server answers free shipping with a fee of 0")
def server_free(world: World) -> None:
    got = sup.state(world)["fee"]
    assert got["free"] and got["fee"] == 0, got


@then(parsers.parse("the server answers a fee of {fee:d} that is not free"))
def server_fee(world: World, fee: int) -> None:
    got = sup.state(world)["fee"]
    assert not got["free"] and got["fee"] == fee, got


# ── Then: checkout ───────────────────────────────────────────────────────
@then(parsers.parse("the summary shows subtotal {sub:d}, shipping {ship:d} and total {total:d}"))
def summary_sub_ship_total(world: World, sub: int, ship: int, total: int) -> None:
    rows = summary_rows(world.page)
    assert (rows["subtotal"], rows["shipping"], rows["total"]) == (sub, ship, total), rows


def _orders(world: World) -> list[dict]:
    return sup.buyer_orders(sup.state(world)["buyer_token"])


@then("the buyer has no orders")
def no_orders(world: World) -> None:
    orders = _orders(world)
    assert not orders, f"the refused checkout left orders: {orders}"


@then(
    parsers.re(
        r"the buyer ends up with exactly one order whose total_amount is (?P<total>\d+|the shown total)"
    )
)
def one_order_total(world: World, total: str) -> None:
    st = sup.state(world)
    expected = st["shown_total"] if total == "the shown total" else int(total)
    got = poll(lambda: len(_orders(world)) == 1, timeout_s=20)
    orders = _orders(world)
    assert got, f"expected exactly one order, got {orders}"
    assert int(orders[0].get("totalAmount", 0)) == expected, orders[0]


@then("the checkout alert shows the price-changed message")
def price_changed_alert(world: World) -> None:
    alert = world.page.get_by_role("alert").filter(has_text=sup.PRICE_CHANGED_MESSAGE)
    expect(alert.first).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse("the summary total is the subtotal {subtotal:d} plus the server shipping fee"))
def total_is_subtotal_plus_fee(world: World, subtotal: int) -> None:
    st = sup.state(world)
    expected = sup.total_with_shipping(st["buyer_token"], st["city"], subtotal)
    # the page refreshed after the refusal: wait for the re-priced summary
    ok = poll(lambda: summary_rows(world.page).get("total") == expected, timeout_s=15)
    rows = summary_rows(world.page)
    assert ok, f"summary {rows}, expected subtotal {subtotal} and total {expected}"
    assert rows["subtotal"] == subtotal, rows
    st["shown_total"] = rows["total"]


# ── Then: anonymous ──────────────────────────────────────────────────────
@then("the response redirects to the login page")
def redirects_to_login(world: World) -> None:
    # Next streams the page, so the redirect is observable as the navigation the browser ends on
    world.page.wait_for_url(re.compile(r".*/login(\?.*)?$"), timeout=timeouts.NAVIGATION)


@then("the checkout page shows no cart, shipping fee or price-changed message")
def checkout_shows_nothing(world: World) -> None:
    body = world.page.inner_text("body")
    for text in ("Tạm tính", "Phí vận chuyển", "Tổng cộng", sup.PRICE_CHANGED_MESSAGE):
        assert text not in body, f"{text!r} rendered for an anonymous visitor"
