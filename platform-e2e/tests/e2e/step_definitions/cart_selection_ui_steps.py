"""Steps for frontend/cart_selection_ui.feature (change frontend-cart-selection).

Black box: shops, listings, buyers and carts are seeded through the gateway; line ids come from
the buyer's own `GetCart`, placed orders and the remaining cart are read back from the gateway.
The selection checkboxes are located by role inside each shop group (first checkbox of a group =
the shop's select-all, the following ones = its lines in cart order).
"""

from __future__ import annotations

import re
from typing import Any
from urllib.parse import parse_qs, unquote, urlencode, urlsplit

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.pages import CheckoutPage
from tests.e2e.step_definitions.d2_cart_steps import cart_page_not_client
from tests.e2e.support import cart_selection_support as cs
from tests.e2e.support import checkout_coherence_support as cc
from tests.e2e.support import d2_support as d2
from tests.e2e.support.world import World

SHOP_LAYOUT = [["A1", "A2"], ["B1"]]
ORDER_DONE = re.compile(r".*/(account/orders|checkout/pay).*")


def keys(text: str) -> list[str]:
    return [k.strip() for k in text.split(",") if k.strip()]


def base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


def checkout_page(world: World) -> CheckoutPage:
    return world.get_page(PageName.CHECKOUT)  # type: ignore[return-value]


def items_href(ids: list[str]) -> str:
    return "/checkout?" + urlencode({"items": ",".join(ids)})


def open_checkout(world: World, ids: list[str]) -> None:
    world.page.goto(f"{base(world)}{items_href(ids)}", wait_until="domcontentloaded")


def seed_two_lines(world: World) -> dict[str, Any]:
    user = d2.seed_buyer(world, address_city="Đà Nẵng")
    shop_a = d2.seed_shop(price=300_000, stock=50)
    shop_b = d2.seed_shop(price=100_000, stock=50)
    for shop in (shop_a, shop_b):
        cs.add(user.token, shop["listing_id"])
    c = cs.ctx(world)
    c.update(
        buyer=user,
        listings={"A1": shop_a["listing_id"], "B1": shop_b["listing_id"]},
        titles={"A1": shop_a["title"], "B1": shop_b["title"]},
        shop_a=shop_a,
        shop_b=shop_b,
        ids={
            "A1": cs.line_id(user.token, shop_a["listing_id"]),
            "B1": cs.line_id(user.token, shop_b["listing_id"]),
        },
    )
    return c


def token(world: World) -> str:
    return cs.ctx(world)["buyer"].token


def group_checkboxes(world: World, group: int):  # noqa: ANN201
    groups = world.page.get_by_test_id("cart-shop-group")
    return groups.nth(group).get_by_role("checkbox")


def place_from_checkout(world: World) -> None:
    page = checkout_page(world)
    expect(page.stepper).to_be_visible(timeout=timeouts.NAVIGATION)
    page.continue_to("confirm")
    button = page.place_order_button
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    world.page.wait_for_timeout(500)
    button.click()
    world.page.wait_for_url(ORDER_DONE, timeout=timeouts.NAVIGATION, wait_until="commit")


# ── Given ────────────────────────────────────────────────────────────────
@given("a cs buyer has 3 cart lines, two from shop A and one from shop B")
def three_lines(world: World) -> None:
    c = cs.seed_three_lines(world)
    c["listings"] = {
        "A1": c["shop_a"]["listing_id"],
        "A2": c["a2"]["listing_id"],
        "B1": c["shop_b"]["listing_id"],
    }


@given("a cs buyer has 2 cart lines from two shops")
def two_lines(world: World) -> None:
    seed_two_lines(world)


@given("a cs buyer has a cart line and another cs buyer has a cart line")
def two_buyers(world: World) -> None:
    other = d2.seed_buyer(world, address_city="Đà Nẵng")
    other_shop = d2.seed_shop(price=120_000, stock=50)
    cs.add(other.token, other_shop["listing_id"])
    other_line = cs.line_id(other.token, other_shop["listing_id"])
    user = d2.seed_buyer(world, address_city="Đà Nẵng")
    shop = d2.seed_shop(price=300_000, stock=50)
    cs.add(user.token, shop["listing_id"])
    cs.ctx(world).update(
        buyer=user,
        own_line=cs.line_id(user.token, shop["listing_id"]),
        other=other,
        other_line=other_line,
        other_qty=1,
    )


@given("a cs listing of a third shop that is not in the cart")
def third_shop(world: World) -> None:
    c = cs.ctx(world)
    c["shop_c"] = d2.seed_shop(price=150_000, stock=50)
    c["listings"]["C"] = c["shop_c"]["listing_id"]
    c["titles"]["C"] = c["shop_c"]["title"]


@given("the cs cart already holds the third shop's listing with quantity 2")
def third_shop_in_cart(world: World) -> None:
    c = cs.ctx(world)
    c["shop_c"] = d2.seed_shop(price=150_000, stock=50)
    c["listings"]["C"] = c["shop_c"]["listing_id"]
    c["titles"]["C"] = c["shop_c"]["title"]
    cs.add(token(world), c["shop_c"]["listing_id"], 2)
    c["ids"]["C"] = cs.line_id(token(world), c["shop_c"]["listing_id"])


# ── When ─────────────────────────────────────────────────────────────────
@when("the cs buyer opens the cart")
def open_cart(world: World) -> None:
    world.page.goto(f"{base(world)}/cart", wait_until="domcontentloaded")
    world.page.wait_for_load_state("networkidle")
    world.page.wait_for_timeout(500)


@when(parsers.parse('the cs buyer ticks only the line "{key}"'))
def tick_only(world: World, key: str) -> None:
    for g, lines in enumerate(SHOP_LAYOUT):
        boxes = group_checkboxes(world, g)
        expect(boxes).to_have_count(1 + len(lines), timeout=timeouts.DEFAULT)
        for j, line in enumerate(lines):
            box = boxes.nth(1 + j)
            if line == key:
                expect(box).to_be_checked()
            else:
                box.uncheck()
                expect(box).not_to_be_checked()


@when("the cs buyer unticks the select-all of shop B")
def untick_shop_b(world: World) -> None:
    boxes = group_checkboxes(world, 1)
    expect(boxes).to_have_count(2, timeout=timeouts.DEFAULT)
    expect(boxes.first).to_be_checked()
    boxes.first.uncheck()
    expect(boxes.nth(1)).not_to_be_checked()


@when("the cs buyer follows the buy action and places the order")
def follow_buy(world: World) -> None:
    link = world.page.locator('a[href^="/checkout"]:visible').first
    expect(link).to_be_visible(timeout=timeouts.DEFAULT)
    link.click()
    world.page.wait_for_url(re.compile(r".*/checkout\?"), timeout=timeouts.NAVIGATION)
    place_from_checkout(world)


@when("the cs buyer places the order from the confirm step")
def place_confirm(world: World) -> None:
    place_from_checkout(world)


@when("the cs buyer opens the checkout of their own line")
def open_own(world: World) -> None:
    open_checkout(world, [cs.ctx(world)["own_line"]])


@when("the cs buyer opens the checkout of the other buyer's line")
def open_other(world: World) -> None:
    open_checkout(world, [cs.ctx(world)["other_line"]])


@when(parsers.parse('the cs buyer opens the checkout of the line "{key}"'))
def open_line(world: World, key: str) -> None:
    open_checkout(world, [cs.ctx(world)["ids"][key]])
    expect(checkout_page(world).stepper).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the cs visitor is anonymous and opens the checkout of the line "{key}"'))
def anonymous_open(world: World, key: str) -> None:
    world.context.clear_cookies()
    open_checkout(world, [cs.ctx(world)["ids"][key]])


@when("the cs buyer requests the cart page")
def request_cart(world: World) -> None:
    resp = world.context.request.get(f"{base(world)}/cart")
    assert resp.ok, resp.status
    cs.ctx(world)["cart_html"] = resp.text()


@when("the cs buyer opens that listing and presses Mua ngay")
def press_buy_now(world: World) -> None:
    c = cs.ctx(world)
    world.page.goto(
        f"{base(world)}/listing/{c['shop_c']['listing_id']}", wait_until="domcontentloaded"
    )
    world.page.wait_for_load_state("networkidle")
    button = world.page.get_by_role("button", name=re.compile(r"Mua ngay", re.IGNORECASE)).first
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    world.page.wait_for_timeout(500)
    button.click()


# ── Then: cart page ──────────────────────────────────────────────────────
@then(parsers.parse('the cs buy action links to the checkout of the lines "{lines}"'))
def buy_links(world: World, lines: str) -> None:
    ids = cs.ctx(world)["ids"]
    want = items_href([ids[k] for k in keys(lines)])
    link = world.page.locator('a[href^="/checkout"]:visible').first
    expect(link).to_have_attribute("href", want, timeout=timeouts.DEFAULT)


@then("the cs first HTML response holds the 3 line rows and a checkbox per line and per shop")
def first_response(world: World) -> None:
    c = cs.ctx(world)
    html = c["cart_html"]
    for key in ("A1", "A2", "B1"):
        assert c["titles"][key] in html, f"line {key} missing from the server HTML"
    boxes = re.findall(r'<input[^>]*type="checkbox"|role="checkbox"', html)
    assert len(boxes) == 5, (
        f"expected 3 line and 2 shop checkboxes in the first HTML, got {len(boxes)}"
    )


@then('the cs cart page source has no "use client" directive')
def not_client(world: World) -> None:
    cart_page_not_client(world)


# ── Then: orders and cart ────────────────────────────────────────────────
def placed_items(world: World, count: int) -> list[dict[str, Any]]:
    orders = cs.wait_orders(token(world), count)
    return [cc.get_order(token(world), o["id"]) for o in orders]


@then(parsers.parse('the cs buyer has exactly 1 order and it holds only the line "{key}"'))
def one_order_line(world: World, key: str) -> None:
    (order,) = placed_items(world, 1)
    got = {i["listingId"] for i in order.get("items", [])}
    assert got == {cs.ctx(world)["listings"][key]}, order


@then("the cs buyer has exactly 1 order and it holds only the new line")
def one_order_new(world: World) -> None:
    (order,) = placed_items(world, 1)
    got = {i["listingId"] for i in order.get("items", [])}
    assert got == {cs.ctx(world)["listings"]["C"]}, order


@then("the cs buyer has exactly 1 order and it holds the merged line with quantity 3")
def one_order_merged(world: World) -> None:
    (order,) = placed_items(world, 1)
    items = order.get("items", [])
    assert [(i["listingId"], int(i["quantity"])) for i in items] == [
        (cs.ctx(world)["listings"]["C"], 3)
    ], order


@then(
    parsers.parse(
        'every cs order of the buyer belongs to shop A and holds only the lines "{lines}"'
    )
)
def orders_shop_a(world: World, lines: str) -> None:
    c = cs.ctx(world)
    orders = placed_items(world, 1)
    for order in orders:
        assert order["sellerId"] == c["shop_a"]["seller_id"], order
    got = {i["listingId"] for o in orders for i in o.get("items", [])}
    assert got == {c["listings"][k] for k in keys(lines)}, orders


@then(parsers.parse('the cs cart afterwards lists the lines "{lines}" only'))
def cart_lists(world: World, lines: str) -> None:
    want = {cs.ctx(world)["listings"][k] for k in keys(lines)}
    got = {i["listingId"] for i in cs.cart_items(token(world))}
    assert got == want, (got, want)
    open_cart(world)
    cart = world.page.get_by_test_id("cart-shop-group")
    for key in keys(lines):
        expect(cart.get_by_text(cs.ctx(world)["titles"][key]).first).to_be_visible(
            timeout=timeouts.DEFAULT
        )


# ── Then: isolation and login ────────────────────────────────────────────
def expect_back_link(world: World, line: str) -> None:
    link = world.page.get_by_role("link", name="Quay lại giỏ hàng").first
    expect(link).to_have_attribute(
        "href", "/cart?" + urlencode({"items": line}), timeout=timeouts.NAVIGATION
    )


@then("the cs browser stays on the checkout of that line and its back link keeps the selection")
def stays_on_checkout(world: World) -> None:
    own = cs.ctx(world)["own_line"]
    expect(checkout_page(world).stepper).to_be_visible(timeout=timeouts.NAVIGATION)
    assert "/checkout" in world.page.url and own in unquote(world.page.url), world.page.url
    expect_back_link(world, own)


@then(parsers.parse('the cs back link to the cart keeps the line "{key}"'))
def back_link_keeps(world: World, key: str) -> None:
    expect_back_link(world, cs.ctx(world)["ids"][key])


@then("the cs browser is redirected to the cart")
def redirected_to_cart(world: World) -> None:
    world.page.wait_for_url(re.compile(r".*/cart(\?.*)?$"), timeout=timeouts.NAVIGATION)
    assert "/checkout" not in world.page.url, world.page.url


@then("the cs buyer has no order and the other buyer's cart is unchanged")
def nothing_placed(world: World) -> None:
    c = cs.ctx(world)
    assert cc.buyer_orders(c["buyer"].token) == []
    assert cc.buyer_orders(c["other"].token) == []
    items = cs.cart_items(c["other"].token)
    assert [(i["id"], int(i["quantity"])) for i in items] == [(c["other_line"], c["other_qty"])]


@then(parsers.parse('the cs checkout lists the line "{a}" and not the line "{b}"'))
def lists_only(world: World, a: str, b: str) -> None:
    titles = cs.ctx(world)["titles"]
    checkout_page(world).continue_to("confirm")
    expect(world.page.get_by_text(titles[a]).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text(titles[b])).to_have_count(0)


@then('the cs checkout lists the merged line and not the line "A1"')
def lists_merged(world: World) -> None:
    titles = cs.ctx(world)["titles"]
    checkout_page(world).continue_to("confirm")
    expect(world.page.get_by_text(titles["C"]).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text(titles["A1"])).to_have_count(0)


@then(
    parsers.parse(
        'the cs browser is sent to login with the return path "{path}" and the line "{key}"'
    )
)
def sent_to_login(world: World, path: str, key: str) -> None:
    world.page.wait_for_url(re.compile(r".*/login"), timeout=timeouts.NAVIGATION)
    query = parse_qs(urlsplit(world.page.url).query)
    ret = unquote(query.get("returnUrl", [""])[0])
    assert ret == path + cs.ctx(world)["ids"][key], world.page.url
    assert world.page.get_by_test_id("cart-shop-group").count() == 0


# ── Then: Mua ngay ───────────────────────────────────────────────────────
def lands_on_items(world: World, line: str) -> None:
    world.page.wait_for_url(re.compile(r".*/checkout\?"), timeout=timeouts.NAVIGATION)
    query = parse_qs(urlsplit(world.page.url).query)
    assert query.get("items") == [line], world.page.url


@then("the cs browser opens the checkout of the new line only")
def opens_new_line(world: World) -> None:
    c = cs.ctx(world)
    new_id = cs.line_id(token(world), c["listings"]["C"])
    c["ids"]["C"] = new_id
    lands_on_items(world, new_id)


@then("the cs browser opens the checkout of the merged line only")
def opens_merged_line(world: World) -> None:
    c = cs.ctx(world)
    lands_on_items(world, c["ids"]["C"])
    merged = [i for i in cs.cart_items(token(world)) if i["id"] == c["ids"]["C"]]
    assert merged and int(merged[0]["quantity"]) == 3, merged
