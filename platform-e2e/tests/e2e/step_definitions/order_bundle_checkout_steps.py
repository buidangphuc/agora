"""Steps of order/bundle_checkout.feature (change order-bundle-checkout).

Black box through the gateway: sellers, listings and bundles are seeded with the real API,
buyers add bundles and check out, the seller changes a listing (unpublish, price) through
UpdateListing. Bundles stored before the CreateBundle rules (foreign owner, variant listing,
price at or above the sum) are inserted into team-domain's `bundles` table, as the
bundle-integrity feature does, because the API refuses them now.
"""

from __future__ import annotations

import os
import subprocess
import uuid
from typing import Any

import httpx
import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import prm_rma_support as rma
from tests.e2e.support.oic_order_support import (
    CART,
    LISTING,
    ORDER,
    Actor,
    OicWorld,
    code_of,
    ensure_address,
    get_order,
    ok,
    pay_order,
    post,
    register,
    wait_status,
)

PAID = "ORDER_STATUS_PAID"
DEFAULT = "buyer"


def _psql(sql: str) -> str:
    out = subprocess.run(
        [
            "docker", "exec", os.getenv("BCO_POSTGRES_CONTAINER") or os.getenv("POSTGRES_CONTAINER", "mainx-postgres-1"),
            "psql", "-U", "postgres", "-d", "listing_db", "-v", "ON_ERROR_STOP=1", "-At", "-c", sql,
        ],
        capture_output=True, text=True, timeout=60, check=True,
    )  # fmt: skip
    return out.stdout


@pytest.fixture
def bco():
    w = OicWorld()
    w.data.update(bundles={}, stored_ids=[], main=None)
    yield w
    for bundle_id in w.data["stored_ids"]:
        _psql(f"DELETE FROM bundles WHERE id = '{bundle_id}'")


# ── seeding helpers ──────────────────────────────────────────────────────
def _new_listing(
    w: OicWorld, owner: Actor, price: int, stock: int = 50, variants: bool = False
) -> str:
    listing: dict[str, Any] = {
        "title": f"[BCO] {uuid.uuid4().hex[:8]}",
        "categoryId": "cat-electronics",
        "price": price,
        "stock": stock,
        "status": "LISTING_STATUS_PUBLISHED",
        "currency": "VND",
        "description": "bundle checkout e2e",
    }
    if variants:
        listing["variants"] = [
            {"name": "Red", "sku": f"R-{uuid.uuid4().hex[:6]}", "price": price, "stock": 5},
            {"name": "Blue", "sku": f"B-{uuid.uuid4().hex[:6]}", "price": price, "stock": 5},
        ]
    resp = post(w, owner, LISTING, "CreateListing", {"listing": listing})
    listing_id = (ok(resp).get("listing") or {}).get("id", "")
    assert listing_id, resp.text
    return listing_id


def _seller(w: OicWorld) -> Actor:
    if "seller" not in w.actors:
        register(w, "seller", "seller")
    return w.actors["seller"]


def _register_bundle(w: OicWorld, label: str, ids: list[str], price: int, bundle_id: str) -> None:
    w.data["bundles"][label] = {"id": bundle_id, "listings": ids, "price": price}
    if w.data["main"] is None:
        w.data["main"] = label


def _create_bundle(w: OicWorld, label: str, ids: list[str], price: int) -> None:
    resp = post(
        w, _seller(w), LISTING, "CreateBundle",
        {"title": f"bundle {uuid.uuid4().hex[:6]}", "listingIds": ids, "bundlePrice": price},
    )  # fmt: skip
    bundle_id = (ok(resp).get("bundle") or {}).get("id", "")
    assert bundle_id, resp.text
    _register_bundle(w, label, ids, price, bundle_id)


def _store_bundle(w: OicWorld, label: str, ids: list[str], price: int) -> None:
    bundle_id = f"bco-{uuid.uuid4().hex[:12]}"
    array = ", ".join(f"'{i}'" for i in ids)
    _psql(
        "INSERT INTO bundles (id, seller_id, title, listing_ids, bundle_price) VALUES "
        f"('{bundle_id}', '{_seller(w).user_id}', 'stored bundle', ARRAY[{array}]::text[], {price})"
    )
    w.data["stored_ids"].append(bundle_id)
    _register_bundle(w, label, ids, price, bundle_id)


def _bundle(w: OicWorld, label: str | None = None) -> dict[str, Any]:
    return w.data["bundles"][label or w.data["main"]]


def _buyer(w: OicWorld, label: str = DEFAULT) -> Actor:
    return w.actors[f"buyer:{label}"]


def _make_buyer(w: OicWorld, label: str) -> Actor:
    actor = register(w, f"buyer:{label}", "buyer")
    ensure_address(w, actor)
    return actor


def _cart(w: OicWorld, actor: Actor) -> dict[str, Any]:
    return ok(post(w, actor, CART, "GetCart", {})).get("cart") or {}


def _lines(w: OicWorld, actor: Actor) -> list[dict[str, Any]]:
    return _cart(w, actor).get("items") or []


def _tagged(w: OicWorld, actor: Actor, label: str | None = None) -> list[dict[str, Any]]:
    bundle_id = _bundle(w, label)["id"]
    return [i for i in _lines(w, actor) if i.get("bundleId") == bundle_id]


def _line_of(w: OicWorld, actor: Actor, listing_id: str, bundled: bool) -> dict[str, Any] | None:
    for item in _lines(w, actor):
        if item.get("listingId") == listing_id and bool(item.get("bundleId")) == bundled:
            return item
    return None


def _bundle_lines_ordered(w: OicWorld, actor: Actor) -> list[dict[str, Any]]:
    bundle = _bundle(w)
    lines = _tagged(w, actor)
    by_listing = {line["listingId"]: line for line in lines}
    return [by_listing[i] for i in bundle["listings"] if i in by_listing]


def _add_bundle(
    w: OicWorld, actor: Actor | None, label: str | None, quantity: int
) -> httpx.Response:
    bundle_id = _bundle(w, label)["id"] if label != "unknown" else f"missing-{uuid.uuid4().hex}"
    resp = post(w, actor, CART, "AddBundleToCart", {"bundleId": bundle_id, "quantity": quantity})
    w.responses["bundle"] = resp
    return resp


def _update_listing(w: OicWorld, listing_id: str, **changes: Any) -> None:
    seller = _seller(w)
    cur = ok(post(w, None, LISTING, "GetListing", {"id": listing_id}))["listing"]
    cur.update(changes)
    ok(post(w, seller, LISTING, "UpdateListing", {"listing": cur}))


def _stock(w: OicWorld, listing_id: str) -> int:
    return int(
        ok(post(w, _seller(w), LISTING, "GetListing", {"id": listing_id}))["listing"].get(
            "stock", 0
        )
    )


def _message(resp: httpx.Response) -> str:
    try:
        return str(resp.json().get("message", ""))
    except ValueError:
        return resp.text


# ── Given ────────────────────────────────────────────────────────────────
@given(
    parsers.parse(
        "a seller with a published bundle of listings priced {a:d} and {b:d} at bundle price {p:d}"
    )
)
def seller_bundle(bco: OicWorld, a: int, b: int, p: int) -> None:
    seller = _seller(bco)
    bco.data["priced"] = [a, b]
    _create_bundle(bco, "main", [_new_listing(bco, seller, a), _new_listing(bco, seller, b)], p)


@given(
    parsers.parse(
        'a seller with a published bundle "{label}" of listings priced {a:d} and {b:d} at bundle price {p:d}'
    )
)
def seller_named_bundle(bco: OicWorld, label: str, a: int, b: int, p: int) -> None:
    seller = _seller(bco)
    _create_bundle(bco, label, [_new_listing(bco, seller, a), _new_listing(bco, seller, b)], p)


@given(
    parsers.parse(
        "a seller with a published bundle of {n:d} listings priced {price:d} each at bundle price {p:d}"
    )
)
def seller_bundle_n(bco: OicWorld, n: int, price: int, p: int) -> None:
    seller = _seller(bco)
    _create_bundle(bco, "main", [_new_listing(bco, seller, price) for _ in range(n)], p)


@given(
    parsers.parse(
        'a seller bundle "{label}" of listings priced {a:d} and {b:d} at bundle price {p:d} '
        "where the second listing has stock {stock:d}"
    )
)
def seller_scarce_bundle(bco: OicWorld, label: str, a: int, b: int, p: int, stock: int) -> None:
    seller = _seller(bco)
    ids = [_new_listing(bco, seller, a), _new_listing(bco, seller, b, stock=stock)]
    _create_bundle(bco, label, ids, p)


@given(
    parsers.parse(
        'a stored bundle "{label}" of one listing of the seller and one listing of another seller '
        "at bundle price {p:d}"
    )
)
def stored_foreign(bco: OicWorld, label: str, p: int) -> None:
    other = register(bco, "other", "seller")
    ids = [_new_listing(bco, _seller(bco), 100_000), _new_listing(bco, other, 50_000)]
    _store_bundle(bco, label, ids, p)


@given(
    parsers.parse(
        'a stored bundle "{label}" of one listing of the seller and one listing with variants '
        "at bundle price {p:d}"
    )
)
def stored_variant(bco: OicWorld, label: str, p: int) -> None:
    seller = _seller(bco)
    ids = [_new_listing(bco, seller, 100_000), _new_listing(bco, seller, 50_000, variants=True)]
    _store_bundle(bco, label, ids, p)


@given(
    parsers.parse(
        'a stored bundle "{label}" of two listings of the seller priced {a:d} and {b:d} '
        "at bundle price {p:d}"
    )
)
def stored_dear(bco: OicWorld, label: str, a: int, b: int, p: int) -> None:
    seller = _seller(bco)
    _store_bundle(bco, label, [_new_listing(bco, seller, a), _new_listing(bco, seller, b)], p)


@given("a buyer with a saved address")
def buyer_with_address(bco: OicWorld) -> None:
    _make_buyer(bco, DEFAULT)


@given(parsers.parse('a buyer "{label}" with a saved address'))
def named_buyer(bco: OicWorld, label: str) -> None:
    _make_buyer(bco, label)


@given("the seller has a saved address")
def seller_with_address(bco: OicWorld) -> None:
    ensure_address(bco, _seller(bco))


@given(parsers.parse("the buyer has {qty:d} of the first listing as a plain line"))
def buyer_plain_line(bco: OicWorld, qty: int) -> None:
    first = _bundle(bco)["listings"][0]
    ok(post(bco, _buyer(bco), CART, "AddToCart", {"listingId": first, "quantity": qty}))


@given(parsers.parse("the buyer has the bundle in the cart with quantity {qty:d}"))
def buyer_has_bundle(bco: OicWorld, qty: int) -> None:
    ok(_add_bundle(bco, _buyer(bco), None, qty))


@given(parsers.parse('buyer "{label}" has the bundle in the cart with quantity {qty:d}'))
def named_buyer_has_bundle(bco: OicWorld, label: str, qty: int) -> None:
    ok(_add_bundle(bco, _buyer(bco, label), None, qty))


@given(parsers.parse('the second listing of bundle "{label}" is unpublished'))
def unpublish_second_of(bco: OicWorld, label: str) -> None:
    _update_listing(bco, _bundle(bco, label)["listings"][1], status="LISTING_STATUS_DRAFT")


@given("the buyer has paid the bundle order")
def buyer_paid(bco: OicWorld) -> None:
    buyer = _buyer(bco)
    orders = ok(post(bco, buyer, ORDER, "CreateOrder", {"paymentMethod": "PAYMENT_METHOD_COD"}))
    order_id = orders["orders"][0]["id"]
    pay_order(bco, buyer, order_id)
    wait_status(bco, buyer, order_id, PAID)
    bco.data["order_id"] = order_id


# ── When ─────────────────────────────────────────────────────────────────
@when(parsers.parse("the buyer adds the bundle to the cart with quantity {qty:d}"))
def buyer_adds(bco: OicWorld, qty: int) -> None:
    _add_bundle(bco, _buyer(bco), None, qty)


@when(parsers.parse('the buyer adds the bundle "{label}" to the cart with quantity {qty:d}'))
def buyer_adds_named(bco: OicWorld, label: str, qty: int) -> None:
    _add_bundle(bco, _buyer(bco), label, qty)


@when(parsers.parse("the buyer adds an unknown bundle to the cart with quantity {qty:d}"))
def buyer_adds_unknown(bco: OicWorld, qty: int) -> None:
    bco.data["bundles"].setdefault("unknown", {"id": "", "listings": [], "price": 0})
    _add_bundle(bco, _buyer(bco), "unknown", qty)


@when(parsers.parse("a caller without a session adds the bundle to the cart with quantity {qty:d}"))
def anonymous_adds(bco: OicWorld, qty: int) -> None:
    _add_bundle(bco, None, None, qty)


@when(parsers.parse('buyer "{label}" adds the bundle to the cart with quantity {qty:d}'))
def named_adds(bco: OicWorld, label: str, qty: int) -> None:
    ok(_add_bundle(bco, _buyer(bco, label), None, qty))


@when(parsers.parse("the seller adds the bundle to their own cart and places the order"))
def seller_buys_own(bco: OicWorld) -> None:
    seller = _seller(bco)
    ok(_add_bundle(bco, seller, None, 1))
    bco.responses["order"] = post(
        bco, seller, ORDER, "CreateOrder", {"paymentMethod": "PAYMENT_METHOD_COD"}
    )


@when(parsers.parse("the buyer changes the quantity of the first bundle line to {qty:d}"))
def change_first(bco: OicWorld, qty: int) -> None:
    line = _bundle_lines_ordered(bco, _buyer(bco))[0]
    ok(post(bco, _buyer(bco), CART, "UpdateCartItem", {"itemId": line["id"], "quantity": qty}))


@when(parsers.parse("the buyer changes the quantity of the second bundle line to {qty:d}"))
def change_second(bco: OicWorld, qty: int) -> None:
    line = _bundle_lines_ordered(bco, _buyer(bco))[1]
    ok(post(bco, _buyer(bco), CART, "UpdateCartItem", {"itemId": line["id"], "quantity": qty}))


@when("the buyer removes the first bundle line")
def remove_first(bco: OicWorld) -> None:
    line = _bundle_lines_ordered(bco, _buyer(bco))[0]
    bco.data["removed"] = line
    ok(post(bco, _buyer(bco), CART, "RemoveFromCart", {"itemId": line["id"]}))


@when(
    parsers.parse(
        'buyer "{label}" changes the quantity of buyer "{owner}"\'s first bundle line to {qty:d}'
    )
)
def foreign_change(bco: OicWorld, label: str, owner: str, qty: int) -> None:
    line = _bundle_lines_ordered(bco, _buyer(bco, owner))[0]
    bco.responses["bundle"] = post(
        bco, _buyer(bco, label), CART, "UpdateCartItem", {"itemId": line["id"], "quantity": qty}
    )


@when(parsers.parse('buyer "{label}" removes buyer "{owner}"\'s first bundle line'))
def foreign_remove(bco: OicWorld, label: str, owner: str) -> None:
    line = _bundle_lines_ordered(bco, _buyer(bco, owner))[0]
    bco.responses["bundle"] = post(
        bco, _buyer(bco, label), CART, "RemoveFromCart", {"itemId": line["id"]}
    )


@when("the buyer previews the checkout")
def buyer_previews(bco: OicWorld) -> None:
    bco.responses["preview"] = post(bco, _buyer(bco), ORDER, "PreviewCheckout", {})
    bco.responses["checkout"] = bco.responses["preview"]


@when("the buyer places the order")
def buyer_places(bco: OicWorld) -> None:
    resp = post(bco, _buyer(bco), ORDER, "CreateOrder", {"paymentMethod": "PAYMENT_METHOD_COD"})
    bco.responses["order"] = resp
    bco.responses["checkout"] = resp


@when("the buyer places an order for only the first bundle line")
def buyer_places_partial(bco: OicWorld) -> None:
    line = _bundle_lines_ordered(bco, _buyer(bco))[0]
    resp = post(
        bco, _buyer(bco), ORDER, "CreateOrder",
        {"paymentMethod": "PAYMENT_METHOD_COD", "itemIds": [line["id"]]},
    )  # fmt: skip
    bco.responses["order"] = resp
    bco.responses["checkout"] = resp


@when("the seller reads the order")
def seller_reads(bco: OicWorld) -> None:
    order_id = ok(bco.responses["order"])["orders"][0]["id"]
    bco.data["seller_order"] = get_order(bco, _seller(bco), order_id)


@when(parsers.parse("the second listing of the bundle is unpublished"))
def unpublish_second(bco: OicWorld) -> None:
    _update_listing(bco, _bundle(bco)["listings"][1], status="LISTING_STATUS_DRAFT")


@when(parsers.parse("the seller changes the price of the first listing to {price:d}"))
def seller_changes_price(bco: OicWorld, price: int) -> None:
    _update_listing(bco, _bundle(bco)["listings"][0], price=price)


@when("the buyer requests a return without an amount")
def return_without_amount(bco: OicWorld) -> None:
    resp = rma.request_return(bco, _buyer(bco), bco.data["order_id"], None)
    bco.data["return"] = ok(resp).get("returnRequest", {})


@when("the buyer requests a return above the order total")
def return_above(bco: OicWorld) -> None:
    order = get_order(bco, _buyer(bco), bco.data["order_id"])
    bco.responses["bundle"] = rma.request_return(
        bco, _buyer(bco), bco.data["order_id"], int(order["totalAmount"]) + 1
    )


# ── Then ─────────────────────────────────────────────────────────────────
@then(parsers.parse("the bundle call fails with {code}"))
def call_fails(bco: OicWorld, code: str) -> None:
    resp = bco.responses["bundle"]
    assert resp.status_code != 200, f"expected {code}, call succeeded: {resp.text[:300]}"
    assert code_of(resp).lower() == code, f"expected {code}, got {code_of(resp)}: {resp.text[:300]}"


@then(parsers.parse("the buyer's cart holds {n:d} lines tagged with the bundle"))
def buyer_cart_tagged(bco: OicWorld, n: int) -> None:
    ok(bco.responses["bundle"])
    assert len(_tagged(bco, _buyer(bco))) == n, _lines(bco, _buyer(bco))


@then(parsers.parse('buyer "{label}" has {n:d} lines tagged with the bundle'))
def named_tagged(bco: OicWorld, label: str, n: int) -> None:
    assert len(_tagged(bco, _buyer(bco, label))) == n, _lines(bco, _buyer(bco, label))


@then(parsers.parse('buyer "{label}" has no line tagged with the bundle'))
def named_not_tagged(bco: OicWorld, label: str) -> None:
    assert not _tagged(bco, _buyer(bco, label)), _lines(bco, _buyer(bco, label))


@then(parsers.parse("the bundle line discounts are {a:d} and {b:d}"))
def discounts_are(bco: OicWorld, a: int, b: int) -> None:
    lines = _bundle_lines_ordered(bco, _buyer(bco))
    got = [int(line.get("bundleDiscount", 0)) for line in lines]
    assert got == [a, b], lines
    assert [int(line["unitPrice"]) for line in lines] == bco.data["priced"], lines


@then(parsers.parse("the bundle line discounts sum to {total:d}"))
def discounts_sum(bco: OicWorld, total: int) -> None:
    lines = _tagged(bco, _buyer(bco))
    assert sum(int(line.get("bundleDiscount", 0)) for line in lines) == total, lines


@then(parsers.parse("the cart bundle discount total is {total:d} and the subtotal is {subtotal:d}"))
def cart_totals(bco: OicWorld, total: int, subtotal: int) -> None:
    cart = _cart(bco, _buyer(bco))
    assert int(cart.get("bundleDiscountTotal", 0)) == total, cart
    assert int(cart.get("subtotal", 0)) == subtotal, cart


@then(parsers.parse("every bundle line has quantity {qty:d}"))
def every_line_qty(bco: OicWorld, qty: int) -> None:
    lines = _tagged(bco, _buyer(bco))
    assert len(lines) == 2, lines
    assert all(int(line["quantity"]) == qty for line in lines), lines


@then("the cart has a plain line of the first listing with quantity 1 and price 100000")
def plain_line_kept(bco: OicWorld) -> None:
    line = _line_of(bco, _buyer(bco), _bundle(bco)["listings"][0], bundled=False)
    assert line is not None, _lines(bco, _buyer(bco))
    assert int(line["quantity"]) == 1 and int(line["unitPrice"]) == 100_000, line


@then("the cart has a bundle line of the first listing")
def bundle_line_present(bco: OicWorld) -> None:
    line = _line_of(bco, _buyer(bco), _bundle(bco)["listings"][0], bundled=True)
    assert line is not None, _lines(bco, _buyer(bco))
    assert line["bundleId"] == _bundle(bco)["id"], line


@then("the buyer's cart is empty")
def cart_empty(bco: OicWorld) -> None:
    assert not _lines(bco, _buyer(bco)), _lines(bco, _buyer(bco))


@then("no line of the bundle keeps a bundle tag or discount")
def no_tags(bco: OicWorld) -> None:
    lines = _lines(bco, _buyer(bco))
    assert lines, "the cart lost its lines"
    assert not [x for x in lines if x.get("bundleId") or int(x.get("bundleDiscount", 0))], lines


@then("the cart holds only a plain line of the second listing at price 50000 with quantity 1")
def only_second_plain(bco: OicWorld) -> None:
    lines = _lines(bco, _buyer(bco))
    bundle = _bundle(bco)
    assert [x["listingId"] for x in lines] == [bundle["listings"][1]], lines
    line = lines[0]
    assert not line.get("bundleId") and int(line.get("bundleDiscount", 0)) == 0, line
    assert int(line["unitPrice"]) == 50_000 and int(line["quantity"]) == 1, line


@then("the cart holds one line of the first listing with quantity 3 and no bundle tag")
def merged_line(bco: OicWorld) -> None:
    first = _bundle(bco)["listings"][0]
    lines = [x for x in _lines(bco, _buyer(bco)) if x["listingId"] == first]
    assert len(lines) == 1, lines
    assert int(lines[0]["quantity"]) == 3 and not lines[0].get("bundleId"), lines


@then(parsers.parse("the preview items subtotal is {amount:d}"))
def preview_subtotal(bco: OicWorld, amount: int) -> None:
    preview = ok(bco.responses["preview"])
    assert int(preview.get("itemsSubtotal", 0)) == amount, preview


@then(parsers.parse("the order total is the bundle price {price:d} plus its shipping fee"))
def order_total(bco: OicWorld, price: int) -> None:
    orders = ok(bco.responses["order"]).get("orders", [])
    assert len(orders) == 1, orders
    order = get_order(bco, _buyer(bco), orders[0]["id"])
    assert int(order["totalAmount"]) == price + int(order.get("shippingFee", 0)), order


@then(
    parsers.parse(
        "every item of the order carries the bundle id and the discounts sum to {total:d}"
    )
)
def order_items_tagged(bco: OicWorld, total: int) -> None:
    items = bco.data["seller_order"].get("items", [])
    assert len(items) == 2, items
    assert all(i.get("bundleId") == _bundle(bco)["id"] for i in items), items
    assert sum(int(i.get("bundleDiscount", 0)) for i in items) == total, items


@then(parsers.parse('the checkout is refused with failed_precondition "{message}"'))
def refused_price_changed(bco: OicWorld, message: str) -> None:
    resp = bco.responses["checkout"]
    assert resp.status_code != 200, f"the checkout succeeded: {resp.text[:300]}"
    assert (code_of(resp), _message(resp)) == ("failed_precondition", message), resp.text[:300]


@then("the cart lines of the bundle are plain at regular prices")
def lines_plain(bco: OicWorld) -> None:
    bundle = _bundle(bco)
    lines = _lines(bco, _buyer(bco))
    assert len(lines) == 2, lines
    assert not [x for x in lines if x.get("bundleId") or int(x.get("bundleDiscount", 0))], lines
    prices = {x["listingId"]: int(x["unitPrice"]) for x in lines}
    assert prices == dict(zip(bundle["listings"], bco.data["priced"], strict=True)), lines


@then("the stock of both listings is unchanged")
def stock_unchanged(bco: OicWorld) -> None:
    assert [_stock(bco, i) for i in _bundle(bco)["listings"]] == [50, 50]


@then("the buyer has no orders")
def buyer_no_orders(bco: OicWorld) -> None:
    orders = ok(post(bco, _buyer(bco), ORDER, "ListBuyerOrders", {})).get("orders", [])
    assert not orders, orders


@then("the order is refused as a purchase of the seller's own listing")
def refused_own(bco: OicWorld) -> None:
    resp = bco.responses["order"]
    assert resp.status_code != 200, f"the order was created: {resp.text[:300]}"
    assert "own listing" in resp.text, resp.text[:300]


@then("the seller has no orders")
def seller_no_orders(bco: OicWorld) -> None:
    orders = ok(post(bco, _seller(bco), ORDER, "ListBuyerOrders", {})).get("orders", [])
    assert not orders, orders


@then("the return refund amount equals the order total")
def refund_equals_total(bco: OicWorld) -> None:
    order = get_order(bco, _buyer(bco), bco.data["order_id"])
    total = int(order["totalAmount"])
    assert total == 120_000 + int(order.get("shippingFee", 0)), order
    assert int(bco.data["return"].get("refundAmount", 0)) == total, bco.data["return"]
