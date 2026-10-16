"""Steps of order/advertised_price.feature (change order-advertised-price).

Black box through the gateway: a seller's listing, an owner-seeded flash campaign, buyers with an
address and a cart, CreateOrder / CancelOrder / GetCart, and the campaign meters
(`support/flash_support`). A sale ends by moving its ends_at into the past (promotion_db).
"""

from __future__ import annotations

import json
import uuid
from concurrent.futures import ThreadPoolExecutor
from threading import Barrier
from typing import Any

from pytest_bdd import given, parsers, then, when

from src.api.services import CartService, ListingService, OrderService
from src.api.services.base_service import GatewayError
from src.models import Listing
from tests.e2e.flows.oic_inv_flow import Actor, make_buyer, poll, register
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support import flash_support as fs
from tests.e2e.support.world import World

_DEFAULT = "_default"


def _x(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("adv", {"buyers": {}, "orders": {}, "errors": {}})


def _svc(world: World, cls, token: str):
    svc = cls(token=token)
    world.add_cleanup(svc.close)
    return svc


def _buyer(world: World, label: str = _DEFAULT) -> Actor:
    return _x(world)["buyers"][label]


def _error_parts(exc: GatewayError) -> tuple[str, str]:
    """(connect code, message) of a gateway error body."""
    try:
        body = json.loads(exc.body)
    except ValueError:
        return "", exc.body
    return str(body.get("code", "")), str(body.get("message", ""))


def _place(world: World, label: str) -> None:
    """CreateOrder (COD) for the buyer's cart; the orders or the error are recorded per buyer."""
    st = _x(world)
    buyer = _buyer(world, label)
    st["errors"].pop(label, None)
    try:
        res = _svc(world, OrderService, buyer.token).create_order(
            {"paymentMethod": "PAYMENT_METHOD_COD"}
        )
    except GatewayError as exc:
        st["errors"][label] = exc
        return
    st["orders"][label] = res.get("orders", [])


def _cart_lines(world: World, label: str = _DEFAULT) -> list[dict[str, Any]]:
    cart = _svc(world, CartService, _buyer(world, label).token).get_cart().get("cart", {})
    return cart.get("items", [])


def _assert_refused_price_changed(exc: GatewayError | None) -> None:
    assert exc is not None, "the checkout succeeded but was expected to be refused"
    code, message = _error_parts(exc)
    assert (code, message) == ("failed_precondition", "price changed"), (
        f"expected failed_precondition 'price changed', got HTTP {exc.status}: {exc.body[:300]}"
    )


def _address(world: World, buyer: Actor) -> None:
    from src.api.services import AddressService

    _svc(world, AddressService, buyer.token).create_address(
        recipient_name="Nguyen Van A",
        phone="0912345678",
        street="29 Lieu Giai",
        city="Ha Noi",
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=True,
    )


# ── Given ────────────────────────────────────────────────────────────────
@given(parsers.parse("a seller's listing priced {price:d} with stock {stock:d}"))
def seller_listing(world: World, price: int, stock: int) -> None:
    seller = register(world, "seller")
    listing = Listing(title=f"[E2E][AdvPrice] {uuid.uuid4().hex[:8]}", price=price, stock=stock)
    listing_id = _svc(world, ListingService, seller.token).create_listing(listing)
    assert listing_id, "listing was not created"
    _x(world).update(seller=seller, listing=listing_id, regular=price)


@given(
    parsers.parse(
        "an active flash campaign on the listing with sale price {price:d} and stock cap {cap:d}"
    )
)
def flash_campaign(world: World, price: int, cap: int) -> None:
    st = _x(world)
    campaign = fs.create_campaign(st["seller"].token, st["listing"], price, cap)
    st.update(campaign=campaign, sale=price, cap=cap, sold_before=fs.flash_sold(campaign))


@given("a buyer with a saved address")
def buyer_with_address(world: World) -> None:
    _x(world)["buyers"][_DEFAULT] = make_buyer(world)


@given(parsers.parse("the buyer has {qty:d} of the listing in the cart"))
def buyer_has_in_cart(world: World, qty: int) -> None:
    buyer = _buyer(world)
    cart = _svc(world, CartService, buyer.token)
    cart.clear_cart()
    cart.add_to_cart(_x(world)["listing"], qty)


@given(
    parsers.parse('a buyer "{label}" with a saved address and {qty:d} of the listing in the cart')
)
def named_buyer_with_cart(world: World, label: str, qty: int) -> None:
    buyer = register(world, "buyer")
    _address(world, buyer)
    _x(world)["buyers"][label] = buyer
    _svc(world, CartService, buyer.token).add_to_cart(_x(world)["listing"], qty)


# ── When ─────────────────────────────────────────────────────────────────
@when(parsers.parse("the buyer adds {qty:d} of the listing to the cart"))
def buyer_adds(world: World, qty: int) -> None:
    _svc(world, CartService, _buyer(world).token).add_to_cart(_x(world)["listing"], qty)


@when("the flash campaign ends")
def campaign_ends(world: World) -> None:
    fs.end_campaign(_x(world)["campaign"])


@when("the buyer places the order")
def buyer_places(world: World) -> None:
    _place(world, _DEFAULT)


@when(parsers.parse('"{label}" places the order'))
def named_places(world: World, label: str) -> None:
    _place(world, label)


@when(parsers.parse('"{a}" and "{b}" place their orders at the same time'))
def both_place(world: World, a: str, b: str) -> None:
    barrier = Barrier(2)

    def go(label: str) -> None:
        barrier.wait(timeout=30)
        _place(world, label)

    with ThreadPoolExecutor(max_workers=2) as pool:
        for fut in [pool.submit(go, a), pool.submit(go, b)]:
            fut.result()


@when(parsers.parse('"{label}" cancels the order'))
def named_cancels(world: World, label: str) -> None:
    st = _x(world)
    orders = st["orders"].get(label)
    assert orders, f"{label} has no order to cancel (error: {st['errors'].get(label)})"
    _svc(world, OrderService, _buyer(world, label).token).cancel_order(orders[0]["id"], "adv-price")


@when("the buyer calls ReserveFlashSaleStock directly through the gateway")
def buyer_reserves_directly(world: World) -> None:
    st = _x(world)
    resp = fh.gateway_hold_call(
        _buyer(world).token,
        "ReserveFlashSaleStock",
        {
            "reservationId": f"adv-{uuid.uuid4().hex[:10]}:{st['listing']}:",
            "campaignId": st["campaign"],
            "listingId": st["listing"],
            "quantity": 1,
            "expectedSalePrice": st["sale"],
        },
    )
    st["direct"] = resp


# ── Then ─────────────────────────────────────────────────────────────────
@then(parsers.parse("the buyer's cart line has unit price {price:d} and the cart subtotal is {subtotal:d}"))
def cart_line_and_subtotal(world: World, price: int, subtotal: int) -> None:
    cart = _svc(world, CartService, _buyer(world).token).get_cart().get("cart", {})
    lines = cart.get("items", [])
    assert len(lines) == 1, f"expected one cart line, got {lines}"
    assert int(lines[0].get("unitPrice", 0)) == price, f"cart line {lines[0]}"
    assert int(cart.get("subtotal", 0)) == subtotal, f"cart {cart}"


@then(parsers.parse("the buyer's cart line has unit price {price:d}"))
def cart_line_price(world: World, price: int) -> None:
    lines = _cart_lines(world)
    assert len(lines) == 1, f"expected one cart line, got {lines}"
    assert int(lines[0].get("unitPrice", 0)) == price, f"cart line {lines[0]}"


@then('the checkout is refused with failed_precondition "price changed"')
def checkout_refused(world: World) -> None:
    _assert_refused_price_changed(_x(world)["errors"].get(_DEFAULT))


@then("the buyer has no orders")
def buyer_has_no_orders(world: World) -> None:
    res = _svc(world, OrderService, _buyer(world).token).list_buyer_orders()
    assert not res.get("orders"), f"the refused checkout left orders: {res}"


def _item_price(orders: list[dict[str, Any]]) -> int:
    assert len(orders) == 1 and len(orders[0].get("items", [])) == 1, f"orders: {orders}"
    return int(orders[0]["items"][0].get("unitPrice", 0))


@then(parsers.parse("the buyer's order item has unit price {price:d}"))
def order_item_price(world: World, price: int) -> None:
    st = _x(world)
    assert _DEFAULT in st["orders"], f"CreateOrder failed: {st['errors'].get(_DEFAULT)}"
    assert _item_price(st["orders"][_DEFAULT]) == price, st["orders"][_DEFAULT]


@then(parsers.parse('the order of "{label}" has item unit price {price:d}'))
def named_order_item_price(world: World, label: str, price: int) -> None:
    st = _x(world)
    assert label in st["orders"], f"CreateOrder of {label} failed: {st['errors'].get(label)}"
    assert _item_price(st["orders"][label]) == price, st["orders"][label]


@then(parsers.parse("exactly one order is created and its item has unit price {price:d}"))
def exactly_one_order(world: World, price: int) -> None:
    st = _x(world)
    created = {k: v for k, v in st["orders"].items() if v}
    assert len(created) == 1, f"orders {created}, errors { {k: str(e) for k, e in st['errors'].items()} }"
    st["winner"] = next(iter(created))
    assert _item_price(next(iter(created.values()))) == price, created


@then('the other buyer\'s checkout is refused with failed_precondition "price changed"')
def other_refused(world: World) -> None:
    st = _x(world)
    losers = [k for k in ("b1", "b2") if k != st.get("winner")]
    assert len(losers) == 1, st
    _assert_refused_price_changed(st["errors"].get(losers[0]))


@then(parsers.parse("the campaign's stock_sold is {n:d} higher without a payment"))
def sold_grows_unpaid(world: World, n: int) -> None:
    assert _DEFAULT in _x(world)["orders"], f"CreateOrder failed: {_x(world)['errors'].get(_DEFAULT)}"
    sold_grows(world, n)


@then(parsers.parse("the campaign's stock_sold is {n:d} higher"))
def sold_grows(world: World, n: int) -> None:
    st = _x(world)
    sold = poll(lambda: fs.flash_sold(st["campaign"]) == st["sold_before"] + n, timeout_s=5)
    assert sold, f"stock_sold {fs.flash_sold(st['campaign'])}, expected {st['sold_before'] + n}"


@then(parsers.parse("the campaign's stock_sold decreases by {n:d} and the flash stock remaining is {remaining:d}"))
def sold_returns(world: World, n: int, remaining: int) -> None:
    st = _x(world)
    ok = poll(
        lambda: fs.flash_sold(st["campaign"]) == st["sold_before"]
        and fs.flash_remaining(st["campaign"]) == remaining,
        timeout_s=10,
    )
    assert ok, (
        f"stock_sold {fs.flash_sold(st['campaign'])} (before the order {st['sold_before']}), "
        f"remaining {fs.flash_remaining(st['campaign'])}, expected {remaining} after returning {n}"
    )


@then(parsers.parse("the flash stock remaining is {remaining:d}"))
def remaining_is(world: World, remaining: int) -> None:
    assert fs.flash_remaining(_x(world)["campaign"]) == remaining


@then("the call is refused with permission_denied, not_found or unimplemented")
def direct_refused(world: World) -> None:
    resp = _x(world)["direct"]
    assert resp.status_code >= 400, f"the buyer's direct reserve was accepted: {resp.text}"
    try:
        code = str(resp.json().get("code", ""))
    except ValueError:
        code = ""
    assert code in {"permission_denied", "not_found", "unimplemented"} or resp.status_code in {
        403,
        404,
        501,
    }, f"HTTP {resp.status_code}: {resp.text[:300]}"
