"""Steps for order-listing-sales: ListingSalesChanged on order.events, observed black box.

Actors and orders go through the gateway; the sold count is read from the absolute values the
relayer publishes on order.events (key = listing_id). Absence and "exactly" assertions drain the
topic for a settle window after the expected number of messages has been seen.
"""

from __future__ import annotations

import time
from typing import Any

import pytest
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from tests.e2e.flows.tracking_flow import _fields
from tests.e2e.support import oic_order_support as o
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_rma_support as r
from tests.e2e.support.oic_order_support import OicWorld, ok, post

SALES_TYPE = "platform.order.v1.ListingSalesChanged"
WAIT_S = 30.0
SETTLE_S = 4.0
SOLD_COUNT_PROBE = 999


@pytest.fixture
def ols():
    return OicWorld()


def _text(v: Any) -> str:
    return bytes(v).decode("utf-8", errors="replace") if isinstance(v, (bytes, bytearray)) else ""


def _decode(raw: bytes, key: bytes | None) -> dict[str, Any]:
    out: dict[str, Any] = {"raw": raw, "key": (key or b"").decode("utf-8", errors="replace")}
    payload = b""
    for n, wire, v in _fields(raw):
        if n == 1 and wire == 2:
            out["event_id"] = _text(v)
        elif n == 2 and wire == 2:
            out["type"] = _text(v)
        elif n == 7 and wire == 2:
            payload = bytes(v)  # type: ignore[arg-type]
    out["payload_fields"] = []
    for n, wire, v in _fields(payload):
        out["payload_fields"].append(n)
        if n == 1 and wire == 2:
            out["listing_id"] = _text(v)
        elif n == 2 and wire == 0:
            out["sold_count"] = int(v)  # type: ignore[arg-type]
        elif n == 3 and wire == 2:
            out["changed_at"] = bytes(v)  # type: ignore[arg-type]
    return out


def sales_events(listing_id: str, expected: int, wait_s: float = WAIT_S) -> list[dict[str, Any]]:
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    c = Consumer(
        {
            "bootstrap.servers": get_settings().kafka_brokers,
            "group.id": f"e2e-listing-sales-{time.time()}",
            "auto.offset.reset": "earliest",
            "enable.auto.commit": False,
        }
    )
    c.assign([TopicPartition("order.events", 0, OFFSET_BEGINNING)])
    ol._require_broker(c)
    found: list[dict[str, Any]] = []
    deadline = time.time() + wait_s
    reached = 0.0
    try:
        while time.time() < deadline:
            if len(found) >= expected and time.time() - reached >= SETTLE_S:
                break
            msg = c.poll(0.5)
            if msg is None or msg.error():
                continue
            raw = msg.value() or b""
            if listing_id.encode() not in raw:
                continue
            d = _decode(raw, msg.key())
            if d.get("type") == SALES_TYPE and d.get("listing_id") == listing_id:
                found.append(d)
                if len(found) >= expected and not reached:
                    reached = time.time()
                elif len(found) > expected:
                    reached = time.time()
    finally:
        c.close()
    return found


def _lid(w: OicWorld, name: str) -> str:
    return w.listings[name]["id"]


def _seller(w: OicWorld) -> o.Actor:
    return w.actors["seller"]


def _ensure_seller(w: OicWorld) -> o.Actor:
    return w.actors.get("seller") or o.register(w, "seller", "seller")


def _place_multi(w: OicWorld, buyer: o.Actor, lines: list[tuple[str, int]]) -> str:
    post(w, buyer, o.CART, "ClearCart", {})
    o.ensure_address(w, buyer)
    for listing, qty in lines:
        o.add_to_cart(w, buyer, listing, qty)
    orders = ok(o.checkout(w, buyer)).get("orders", [])
    assert len(orders) == 1, orders
    return orders[0]["id"]


@given(parsers.parse('a seller with a listing "{name}"'))
def seller_with_listing(ols, name):
    o.create_listing(ols, name, _ensure_seller(ols), 200)


@given(parsers.parse('a seller with listings "{a}" and "{b}"'))
def seller_with_listings(ols, a, b):
    seller = _ensure_seller(ols)
    o.create_listing(ols, a, seller, 200)
    o.create_listing(ols, b, seller, 200)


@given(parsers.parse('a buyer "{name}"'))
def a_buyer(ols, name):
    p.buyer(ols, name)


def _pay_new(w: OicWorld, buyer: str, label: str, qty: int, listing: str) -> None:
    order_id, _ = p.place_and_pay(w, w.actors[buyer], listing, qty)
    w.orders[label] = order_id
    w.data.setdefault("order_buyer", {})[label] = buyer


@given(parsers.parse('"{buyer}" has paid an order "{label}" of {qty:d} of "{listing}"'))
def has_paid(ols, buyer, label, qty, listing):
    _pay_new(ols, buyer, label, qty, listing)


@when(parsers.parse('"{buyer}" pays an order "{label}" of {qty:d} of "{listing}"'))
def pays_order(ols, buyer, label, qty, listing):
    _pay_new(ols, buyer, label, qty, listing)


@given(parsers.parse('"{buyer}" has a pending order "{label}" of {qty:d} of "{listing}"'))
def has_pending(ols, buyer, label, qty, listing):
    ols.orders[label] = o.place_order(ols, ols.actors[buyer], listing, qty)
    ols.data.setdefault("order_buyer", {})[label] = buyer


@when(parsers.parse('"{buyer}" pays one order "{label}" of {q1:d} of "{l1}" and {q2:d} of "{l2}"'))
def pays_multi(ols, buyer, label, q1, l1, q2, l2):
    b = ols.actors[buyer]
    order_id = _place_multi(ols, b, [(l1, q1), (l2, q2)])
    p.pay(ols, b, order_id)
    o.wait_status(ols, b, order_id, o.PAID)
    ols.orders[label] = order_id


def _cancel(w: OicWorld, buyer: str, label: str):
    return post(
        w, w.actors[buyer], o.ORDER, "CancelOrder", {"id": w.orders[label], "reason": "e2e"}
    )


@given(parsers.parse('"{buyer}" cancels the order "{label}"'))
@when(parsers.parse('"{buyer}" cancels the order "{label}"'))
def cancels(ols, buyer, label):
    ok(_cancel(ols, buyer, label))
    o.wait_status(ols, ols.actors[buyer], ols.orders[label], o.CANCELLED)


@when(parsers.parse('"{buyer}" cancels the order "{label}" again'))
def cancels_again(ols, buyer, label):
    ols.responses["last"] = _cancel(ols, buyer, label)


@when(parsers.parse('"{buyer}" requests a return of the order "{label}" and the seller refunds it'))
def return_and_refund(ols, buyer, label):
    ret = r.new_return(ols, ols.actors[buyer], ols.orders[label], 100_000)
    assert ret.get("id"), "the return request was refused"
    r.approve_and_refund(ols, _seller(ols), ret["id"])


@when(parsers.parse('"{buyer}" requests a return of the order "{label}"'))
def request_return(ols, buyer, label):
    ols.responses["last"] = r.request_return(ols, ols.actors[buyer], ols.orders[label], 100_000)


@then("the return request is refused")
def return_refused(ols):
    resp = ols.responses["last"]
    assert not resp.is_success, f"a return of a cancelled order was accepted: {resp.text}"


@when(
    parsers.parse(
        '"{buyer}" sends cart and order requests carrying a sold count of 999 for "{listing}"'
    )
)
def probe_sold_count(ols, buyer, listing):
    b = ols.actors[buyer]
    post(ols, b, o.CART, "ClearCart", {"soldCount": SOLD_COUNT_PROBE})
    post(
        ols,
        b,
        o.CART,
        "AddToCart",
        {"listingId": _lid(ols, listing), "quantity": 1, "soldCount": SOLD_COUNT_PROBE},
    )
    post(
        ols,
        b,
        o.ORDER,
        "CreateOrder",
        {"paymentMethod": "PAYMENT_METHOD_COD", "soldCount": SOLD_COUNT_PROBE},
    )


@then(parsers.parse('the ListingSalesChanged values for "{listing}" are exactly {values}'))
def exact_values(ols, listing, values):
    want = [int(v) for v in values.split(",")]
    got = sales_events(_lid(ols, listing), len(want))
    ols.data[f"events:{listing}"] = got
    assert [e["sold_count"] for e in got] == want, (
        f"ListingSalesChanged for {listing}: got {[e.get('sold_count') for e in got]}, want {want}"
    )


@then(
    parsers.parse(
        'the ListingSalesChanged message for "{listing}" is keyed by the listing id and holds only '
        "listing_id, sold_count and changed_at"
    )
)
def only_three_fields(ols, listing):
    lid = _lid(ols, listing)
    got = sales_events(lid, 1)
    ols.data[f"events:{listing}"] = got
    assert len(got) == 1, f"expected one ListingSalesChanged for {listing}, got {len(got)}"
    msg = got[0]
    assert msg["key"] == lid, f"record key {msg['key']!r} is not the listing id {lid!r}"
    assert msg["payload_fields"] == [1, 2, 3], msg["payload_fields"]
    assert msg["sold_count"] == 2 and msg.get("changed_at"), msg


@then(
    parsers.parse(
        'the ListingSalesChanged record for "{listing}" mentions neither the order, the buyer nor '
        "the seller"
    )
)
def mentions_no_party(ols, listing):
    got = ols.data.get(f"events:{listing}") or sales_events(_lid(ols, listing), 1)
    assert got, "no ListingSalesChanged record to inspect"
    raw = got[0]["raw"]
    forbidden = {"order": ols.orders["O"], "buyer": ols.actors["b1"].user_id}
    forbidden["seller"] = _seller(ols).user_id
    for what, value in forbidden.items():
        if value:
            assert value.encode() not in raw, f"the record carries the {what} id {value}"
