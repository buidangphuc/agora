"""Steps of order/address_eta.feature (change order-address-eta).

Black box through the gateway: CalculateShippingFee, AddressService, CartService, CreateOrder.
"""

from __future__ import annotations

import json
from datetime import datetime, timedelta
from typing import Any
from zoneinfo import ZoneInfo

from pytest_bdd import given, parsers, then, when

from src.api.services import CartService, OrderService
from src.api.services.base_service import GatewayError
from tests.e2e.flows import oic_inv_flow as flow
from tests.e2e.support import address_support as addr
from tests.e2e.support.world import World

_QUOTE = "/platform.order.v1.OrderService/CalculateShippingFee"
_HCM = ZoneInfo("Asia/Ho_Chi_Minh")


def _x(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("addr_eta", {"buyers": {}, "errors": {}, "confirmed": {}})


def _buyer(world: World, label: str):
    return _x(world)["buyers"][label]


def _order_svc(world: World, label: str) -> OrderService:
    svc = OrderService(token=_buyer(world, label).token)
    world.add_cleanup(svc.close)
    return svc


def _error_parts(exc: GatewayError) -> tuple[str, str]:
    try:
        body = json.loads(exc.body)
    except ValueError:
        return "", exc.body
    return str(body.get("code", "")), str(body.get("message", ""))


def _place(world: World, label: str, **extra: Any) -> None:
    st = _x(world)
    st["errors"].pop(label, None)
    body = {"paymentMethod": "PAYMENT_METHOD_COD", **extra}
    try:
        res = _order_svc(world, label).create_order(body)
    except GatewayError as exc:
        st["errors"][label] = exc
        return
    st.setdefault("placed", {})[label] = res.get("orders", [])


def _orders(world: World, label: str) -> list[dict[str, Any]]:
    return _order_svc(world, label).list_buyer_orders().get("orders", [])


@given("a signed-in buyer")
def signed_in_buyer(world: World) -> None:
    _x(world)["buyers"]["quote"] = flow.make_buyer(world)


@given("a seller who owns a listing with stock 5")
def seller_listing(world: World) -> None:
    seller = flow.register(world, "seller")
    _x(world)["listing"] = flow.create_listing(world, seller, 5, title="addr-eta")


@given(parsers.parse('a buyer "{label}" with a default address and 1 of the listing in the cart'))
def buyer_with_cart(world: World, label: str) -> None:
    buyer = flow.register(world, "buyer")
    st = _x(world)
    st["buyers"][label] = buyer
    st.setdefault("addresses", {})[label] = addr.create_address(world, buyer, "Hà Nội")
    flow.fill_cart(world, buyer, [(st["listing"], 1)])


@given(parsers.parse('a buyer "{label}" with a default address'))
def buyer_with_address(world: World, label: str) -> None:
    buyer = flow.register(world, "buyer")
    st = _x(world)
    st["buyers"][label] = buyer
    st.setdefault("addresses", {})[label] = addr.create_address(world, buyer, "Hà Nội")


@given(parsers.parse('"{label}" has confirmed their address'))
def confirmed(world: World, label: str) -> None:
    st = _x(world)
    st["confirmed"][label] = dict(st["addresses"][label])


@when(parsers.parse('the buyer asks for a shipping quote for city "{city}" and items subtotal {sub:d}'))
def ask_quote(world: World, city: str, sub: int) -> None:
    svc = OrderService(token=_buyer(world, "quote").token)
    world.add_cleanup(svc.close)
    before = datetime.now(_HCM).date()
    _x(world)["quote"] = svc.post(_QUOTE, {"city": city, "itemsSubtotal": sub})
    _x(world)["quote_days"] = {before, datetime.now(_HCM).date()}


@when(parsers.parse('"{label}" places the order with the address id of "{other}"'))
def place_with_other_address(world: World, label: str, other: str) -> None:
    _place(world, label, addressId=_x(world)["addresses"][other]["id"])


@when(parsers.parse('"{label}" edits the confirmed address to city "{city}"'))
def edit_confirmed(world: World, label: str, city: str) -> None:
    st = _x(world)
    addr.edit_address(world, _buyer(world, label), st["addresses"][label], city=city)


@when(parsers.parse('"{label}" places the order with the confirmed address'))
def place_with_confirmed(world: World, label: str) -> None:
    st = _x(world)
    confirmed_address = st["confirmed"][label]
    _place(world, label, addressId=confirmed_address["id"], confirmedAddress=confirmed_address)


@when(parsers.parse('"{label}" refreshes the confirmed address'))
def refresh_confirmed(world: World, label: str) -> None:
    st = _x(world)
    mine = [a for a in addr.list_addresses(world, _buyer(world, label)) if a["id"] == st["addresses"][label]["id"]]
    assert mine, "the edited address is missing from the buyer's list"
    st["confirmed"][label] = mine[0]


@then(parsers.parse("the quote has shipping fee {fee:d}"))
def quote_fee(world: World, fee: int) -> None:
    assert int(_x(world)["quote"].get("shippingFee", 0) or 0) == fee, _x(world)["quote"]


@then(parsers.parse("the quote delivery date is {days:d} calendar days after today in Asia/Ho_Chi_Minh"))
def quote_date(world: World, days: int) -> None:
    quote = _x(world)["quote"]
    stamp = quote.get("estimatedDeliveryAt")
    assert stamp, f"the quote carries no estimatedDeliveryAt: {quote}"
    at = datetime.fromisoformat(stamp.replace("Z", "+00:00")).astimezone(_HCM).date()
    expected = {d + timedelta(days=days) for d in _x(world)["quote_days"]}
    assert at in expected, f"delivery date {at} not in {expected}"


@then(parsers.parse('the order is refused with "{code}" and message "{message}"'))
def refused(world: World, code: str, message: str) -> None:
    errors = _x(world)["errors"]
    assert errors, "the order was placed but was expected to be refused"
    exc = next(iter(errors.values()))
    assert _error_parts(exc) == (code, message), f"HTTP {exc.status}: {exc.body[:300]}"


@then(parsers.parse('"{label}" has {n:d} orders'))
def has_orders(world: World, label: str, n: int) -> None:
    assert len(_orders(world, label)) == n


@then(parsers.parse('the cart of "{label}" still holds 1 of the listing'))
def cart_intact(world: World, label: str) -> None:
    svc = CartService(token=_buyer(world, label).token)
    world.add_cleanup(svc.close)
    items = svc.get_cart().get("cart", {}).get("items", [])
    assert [(i.get("listingId"), int(i.get("quantity", 0))) for i in items] == [(_x(world)["listing"], 1)], items


@then(parsers.parse('"{label}" can place the order with their own address id'))
def own_address_works(world: World, label: str) -> None:
    st = _x(world)
    _place(world, label, addressId=st["addresses"][label]["id"])
    assert label not in st["errors"], st["errors"].get(label)
    assert len(st["placed"][label]) == 1


@then(parsers.parse('"{label}" has 1 order whose shipping address city is "{city}"'))
def order_city(world: World, label: str, city: str) -> None:
    st = _x(world)
    assert label not in st["errors"], f"retry refused: {st['errors'].get(label).body}"
    orders = _orders(world, label)
    assert len(orders) == 1, orders
    shipping = orders[0].get("shippingAddress", {})
    assert shipping.get("city") == city, shipping
    assert shipping.get("id") == st["addresses"][label]["id"], shipping
