"""Steps for order-cod-sales-count: a completed COD order raises the sold count in team-search.

Actors and orders go through the gateway; the sold count is read as `soldCount` of the listing hit
from SearchListings. The sweep scenario waits out the auto-complete window configured by
compose/order-inventory.override.yaml.
"""

from __future__ import annotations

import time
import uuid

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import oic_order_support as o
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import plp_support as p
from tests.e2e.support import srm_support as srm
from tests.e2e.support.oic_order_support import OicWorld, ok, post

SETTLE_S = 8.0
COD = "PAYMENT_METHOD_COD"


@pytest.fixture
def cod_world():
    return OicWorld()


def _count(w: OicWorld, name: str) -> int:
    hit = srm.hit_of(srm.search(w.data["query"][name]), w.listings[name]["id"])
    return int((hit or {}).get("soldCount", 0))


def _wait_count(w: OicWorld, name: str, want: int, timeout: float = srm.VISIBLE_S) -> None:
    srm.eventually(
        lambda: _count(w, name) == want,
        f"the search sold count of {name} to read {want}, last read {_count(w, name)}",
        timeout,
    )


@given(parsers.parse('a seller with a listing "{name}" indexed in search'))
def seller_with_indexed_listing(cod_world, name):
    w = cod_world
    seller = w.actors.get("seller") or o.register(w, "seller", "seller")
    token = f"codsales{uuid.uuid4().hex[:10]}"
    resp = post(
        w,
        seller,
        o.LISTING,
        "CreateListing",
        {
            "listing": {
                "title": token,
                "categoryId": "cat-electronics",
                "price": 100_000,
                "stock": 200,
                "status": "LISTING_STATUS_PUBLISHED",
                "currency": "VND",
                "description": "cod sales count e2e",
            }
        },
    )
    listing = ok(resp).get("listing") or {}
    assert listing.get("id"), resp.text
    w.listings[name] = {"id": listing["id"], "seller": seller, "initial": 200}
    w.data.setdefault("query", {})[name] = token
    srm.eventually(
        lambda: srm.hit_of(srm.search(token), listing["id"]) is not None,
        f"listing {name} to be indexed in search",
        srm.VISIBLE_S,
    )
    w.data.setdefault("base", {})[name] = _count(w, name)


@given(parsers.parse('a buyer "{name}"'))
def a_buyer(cod_world, name):
    p.buyer(cod_world, name)


@given(parsers.parse('"{buyer}" has a shipped COD order "{label}" of {qty:d} of "{listing}"'))
def has_shipped_cod(cod_world, buyer, label, qty, listing):
    w = cod_world
    order_id = ol.place(w, w.actors[buyer], listing, qty, COD)
    ol.ship(w, order_id, w.listings[listing]["seller"])
    w.orders[label] = order_id
    w.data.setdefault("order_listing", {})[label] = listing
    w.data["shipped_t"] = time.time()


@given(parsers.parse('"{buyer}" has paid an order "{label}" of {qty:d} of "{listing}"'))
def has_paid(cod_world, buyer, label, qty, listing):
    w = cod_world
    order_id, _ = p.place_and_pay(w, w.actors[buyer], listing, qty)
    w.orders[label] = order_id
    w.data.setdefault("order_listing", {})[label] = listing


@given(parsers.parse('the seller ships the order "{label}"'))
def seller_ships(cod_world, label):
    w = cod_world
    listing = w.data["order_listing"][label]
    ol.ship(w, w.orders[label], w.listings[listing]["seller"])


@when(parsers.parse('"{buyer}" completes the order "{label}" by confirming receipt'))
def buyer_completes(cod_world, buyer, label):
    w = cod_world
    ok(post(w, w.actors[buyer], o.ORDER, "ConfirmReceipt", {"id": w.orders[label]}))


@when(parsers.parse('"{buyer}" completes the order "{label}" by confirming receipt again'))
def buyer_completes_again(cod_world, buyer, label):
    w = cod_world
    w.responses["last"] = post(
        w, w.actors[buyer], o.ORDER, "ConfirmReceipt", {"id": w.orders[label]}
    )


@when(parsers.parse('the seller completes the order "{label}"'))
def seller_completes(cod_world, label):
    w = cod_world
    listing = w.data["order_listing"][label]
    seller = w.listings[listing]["seller"]
    ok(post(w, seller, o.ORDER, "UpdateOrderStatus", {"id": w.orders[label], "status": o.COMPLETED}))


@when(parsers.parse('the auto-complete sweep completes the order "{label}"'))
def sweep_completes(cod_world, label):
    w = cod_world
    buyer = next(a for n, a in w.actors.items() if n != "seller")
    o.wait_status(w, buyer, w.orders[label], o.COMPLETED, timeout=ol.STATUS_WAIT_S)


@then(parsers.parse('the second completion is refused with a status conflict'))
def second_refused(cod_world):
    resp = cod_world.responses["last"]
    assert not resp.is_success, f"a repeated completion was accepted: {resp.text}"
    assert o.code_of(resp) == "failed_precondition", (o.code_of(resp), resp.text[:300])


@then(parsers.parse('the search sold count of "{name}" rises by {n:d}'))
def count_rises(cod_world, name, n):
    w = cod_world
    _wait_count(w, name, w.data["base"][name] + n)


@then(parsers.parse('the search sold count of "{name}" stays risen by {n:d}'))
def count_stays(cod_world, name, n):
    w = cod_world
    want = w.data["base"][name] + n
    end = time.monotonic() + SETTLE_S
    while True:
        got = _count(w, name)
        assert got == want, f"search sold count of {name} is {got}, want {want}"
        if time.monotonic() >= end:
            return
        time.sleep(1)
