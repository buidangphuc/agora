"""Steps for port-order-inventory-correctness, team-order scenarios.

Specs: order-checkout-correctness (minus the storefront requirement), order-lifecycle-guards,
order-read-access, and the order-upstream-principals "commits stock as the order service"
scenario. Everything is driven through the gateway with real, distinct users; the assertions
read back the state the spec promises (order status, listing stock, cart, buyer orders).
"""

from __future__ import annotations

import time
import uuid

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import oic_order_support as s

_STATUS = {
    "Pending": s.PENDING,
    "Paid": s.PAID,
    "Shipped": s.SHIPPED,
    "Completed": s.COMPLETED,
    "Cancelled": s.CANCELLED,
}


@pytest.fixture
def oic():
    w = s.OicWorld()
    yield w
    # destructive lane safety net: never leave team-domain stopped
    if w.data.get("domain_stopped"):
        s.start_domain(w)


def _actor(w: s.OicWorld, name: str) -> s.Actor:
    return w.actors[name]


def _last(w: s.OicWorld):
    return w.responses["last"]


def _key(w: s.OicWorld, name: str) -> str:
    """A unique value per scenario for the symbolic key name used in the feature."""
    return w.data.setdefault(f"key:{name}", f"oic-{name}-{uuid.uuid4().hex}")


# ── actors and catalogue ─────────────────────────────────────────────────
@given(parsers.parse('a seller "{name}" who owns a listing "{listing}" with stock {stock:d}'))
def given_seller_listing(oic, name, listing, stock):
    seller = oic.actors.get(name) or s.register(oic, name, "seller")
    s.create_listing(oic, listing, seller, stock)


@given(parsers.parse('a buyer "{name}"'))
def given_buyer(oic, name):
    buyer = s.register(oic, name, "buyer")
    s.ensure_address(oic, buyer)


@given(parsers.parse('"{buyer}" has {qty:d} of "{listing}" in the cart'))
def given_cart(oic, buyer, qty, listing):
    s.add_to_cart(oic, _actor(oic, buyer), listing, qty)


@given(
    parsers.parse('"{buyer}" has a {status} order for {qty:d} of "{listing}"'),
    target_fixture="order_id",
)
def given_order(oic, buyer, status, qty, listing):
    b = _actor(oic, buyer)
    seller = oic.listings[listing]["seller"]
    order_id = s.place_order(oic, b, listing, qty)
    s.put_in_status(oic, order_id, b, seller, _STATUS[status])
    oic.orders["order"] = order_id
    return order_id


@given(parsers.parse("a platform voucher with a quota of {quota:d}"))
def given_voucher(oic, quota):
    admin = s.login_admin(oic)
    code = f"OIC{uuid.uuid4().hex[:8].upper()}"
    resp = s.post(
        oic,
        admin,
        s.PROMO,
        "CreateVoucher",
        {
            "code": code,
            "scope": "VOUCHER_SCOPE_PLATFORM",
            "discountType": "DISCOUNT_TYPE_PERCENT",
            "discountValue": "10",
            "minSpend": "0",
            "maxDiscount": "50000",
            "quota": str(quota),
            "startsAt": "2026-01-01T00:00:00Z",
            "endsAt": "2027-12-31T23:59:59Z",
        },
    )
    assert (s.ok(resp).get("voucher") or {}).get("code") == code, resp.text
    oic.data["voucher"] = code


@given(
    parsers.parse(
        '"{buyer}" has a Pending order for {qty:d} of "{listing}" placed with the voucher'
    ),
    target_fixture="order_id",
)
def given_voucher_order(oic, buyer, qty, listing):
    order_id = s.place_order(oic, _actor(oic, buyer), listing, qty, voucher=oic.data["voucher"])
    oic.orders["order"] = order_id
    return order_id


@given("team-domain is stopped")
@when("team-domain is stopped")
def stop_domain(oic):
    s.stop_domain(oic)


@when("team-domain is started again")
def start_domain(oic):
    s.start_domain(oic)


# ── checkout ─────────────────────────────────────────────────────────────
@when(parsers.parse('"{buyer}" checks out'))
def when_checkout(oic, buyer):
    oic.responses["last"] = s.checkout(oic, _actor(oic, buyer))


@when(parsers.parse('"{buyer}" checks out again without a key'))
def when_checkout_again(oic, buyer):
    oic.responses["last"] = s.checkout(oic, _actor(oic, buyer))


@when(parsers.parse('"{buyer}" checks out with the idempotency key "{key}"'))
def when_checkout_key(oic, buyer, key):
    oic.responses["last"] = s.checkout(oic, _actor(oic, buyer), _key(oic, key))


@when(parsers.parse('"{buyer}" checks out twice with the idempotency key "{key}"'))
def when_checkout_twice(oic, buyer, key):
    b = _actor(oic, buyer)
    oic.responses["first"] = s.checkout(oic, b, _key(oic, key))
    oic.responses["last"] = s.checkout(oic, b, _key(oic, key))


@when(parsers.parse('"{buyer}" sends two simultaneous checkouts with the idempotency key "{key}"'))
def when_checkout_race(oic, buyer, key):
    b = _actor(oic, buyer)
    k = _key(oic, key)
    oic.data["race"] = s.race([lambda: s.checkout(oic, b, k), lambda: s.checkout(oic, b, k)])


@when(parsers.parse('"{buyer}" and "{other}" each check out with the idempotency key "{key}"'))
def when_two_buyers_same_key(oic, buyer, other, key):
    k = _key(oic, key)
    oic.responses["first"] = s.checkout(oic, _actor(oic, buyer), k)
    oic.responses["last"] = s.checkout(oic, _actor(oic, other), k)


@when(parsers.parse('"{seller}" restocks "{listing}" to {stock:d}'))
def when_restock(oic, seller, listing, stock):
    s.set_listing_stock(oic, listing, stock)


@then(parsers.parse('the call fails with "{code}"'))
def then_fails_with(oic, code):
    resp = _last(oic)
    assert (
        s.code_of(resp) == code
    ), f"expected {code}, got HTTP {resp.status_code}: {resp.text[:300]}"


@then("the call fails")
def then_fails(oic):
    resp = _last(oic)
    assert resp.status_code >= 400, f"call unexpectedly succeeded: {resp.text[:300]}"


@then("the call succeeds")
def then_succeeds(oic):
    resp = _last(oic)
    assert resp.status_code == 200, f"HTTP {resp.status_code}: {resp.text[:300]}"


@then(parsers.parse('"{buyer}" has {count:d} orders'))
def then_order_count(oic, buyer, count):
    got = s.buyer_orders(oic, _actor(oic, buyer))
    assert len(got) == count, f"{buyer} has {len(got)} orders, expected {count}: {got}"


@then(parsers.parse('"{buyer}" has exactly one new order'))
def then_one_order(oic, buyer):
    then_order_count(oic, buyer, 1)


@then(parsers.parse('the stock of "{listing}" is {stock:d}'))
def then_stock(oic, listing, stock):
    s.wait_stock(oic, listing, stock, timeout=15, stay=3)


@then(parsers.parse('the stock of "{listing}" is back to {stock:d}'))
def then_stock_back(oic, listing, stock):
    s.wait_stock(oic, listing, stock, timeout=30, stay=5)


@then(
    parsers.parse(
        'the stock of "{listing}" is back to {stock:d} within the reservation TTL '
        "and two sweep intervals"
    )
)
def then_stock_after_sweep(oic, listing, stock):
    # overlay: TTL 20s + 2 x 2s sweep; stays at the value afterwards (restored once)
    s.wait_stock(oic, listing, stock, timeout=s.TTL_WAIT_S, stay=6)


@then(parsers.parse('the stock of "{listing}" is unchanged at {stock:d}'))
def then_stock_unchanged(oic, listing, stock):
    s.wait_stock(oic, listing, stock, timeout=5, stay=4)


@then(parsers.parse('the cart of "{buyer}" still holds {n:d} items'))
def then_cart_holds(oic, buyer, n):
    items = s.cart_items(oic, _actor(oic, buyer))
    assert len(items) == n, f"cart holds {len(items)} items, expected {n}: {items}"


@then(parsers.parse('the cart of "{buyer}" is empty'))
def then_cart_empty(oic, buyer):
    items = s.cart_items(oic, _actor(oic, buyer))
    assert not items, f"cart still holds {items}"


@then(parsers.parse('{n:d} Pending orders are returned, one per seller "{a}" and "{b}"'))
def then_orders_per_seller(oic, n, a, b):
    orders = s.ok(_last(oic)).get("orders", [])
    assert len(orders) == n, orders
    assert {o.get("status") for o in orders} == {s.PENDING}, orders
    assert {o.get("sellerId") for o in orders} == {_actor(oic, a).user_id, _actor(oic, b).user_id}


@then("the second checkout returns one order per seller")
def then_second_two_orders(oic):
    orders = s.ok(_last(oic)).get("orders", [])
    assert len(orders) == 2, orders


@then("both calls return the same order ids")
def then_same_ids(oic):
    first = s.order_ids(oic.responses["first"])
    last = s.order_ids(oic.responses["last"])
    assert first and first == last, f"first {first} vs second {last}"


@then("each call returns the same order ids or fails with aborted and at least one returns orders")
def then_race_outcomes(oic):
    returned = []
    for resp in oic.data["race"]:
        if resp.status_code == 200:
            returned.append(s.order_ids(resp))
        else:
            assert s.code_of(resp) == "aborted", f"HTTP {resp.status_code}: {resp.text[:300]}"
    assert returned, "no call returned orders"
    assert all(ids == returned[0] and ids for ids in returned), returned


@then("each buyer receives their own new order")
def then_each_own_order(oic):
    a = s.order_ids(oic.responses["first"])
    b = s.order_ids(oic.responses["last"])
    assert len(a) == 1 and len(b) == 1 and a != b, f"{a} vs {b}"


@then(parsers.parse('"{buyer}" has the order of the first checkout'))
def then_has_order(oic, buyer):
    # the buyer's own order list contains the id the first checkout returned
    ids = s.order_ids(oic.responses["first"])
    mine = {o["id"] for o in s.buyer_orders(oic, _actor(oic, buyer))}
    assert set(ids) <= mine, f"{ids} not in {mine}"


# ── lifecycle ────────────────────────────────────────────────────────────
@when(parsers.parse('"{actor}" calls UpdateOrderStatus to {status} on the order'))
def when_update_status(oic, actor, status):
    oic.responses["last"] = s.post(
        oic,
        _actor(oic, actor),
        s.ORDER,
        "UpdateOrderStatus",
        {"id": oic.orders["order"], "status": _STATUS[status]},
    )


@when(parsers.parse('"{buyer}" cancels the order'))
def when_cancel(oic, buyer):
    oic.responses["last"] = s.post(
        oic, _actor(oic, buyer), s.ORDER, "CancelOrder", {"id": oic.orders["order"]}
    )


@when(parsers.parse('"{buyer}" sends two simultaneous cancels of the order'))
def when_cancel_race(oic, buyer):
    b = _actor(oic, buyer)
    oid = oic.orders["order"]
    oic.data["race"] = s.race(
        [lambda: s.post(oic, b, s.ORDER, "CancelOrder", {"id": oid}) for _ in range(2)]
    )


@when(parsers.parse('"{seller}" ships the order while "{buyer}" cancels it at the same time'))
def when_ship_cancel_race(oic, seller, buyer):
    sel, b = _actor(oic, seller), _actor(oic, buyer)
    oid = oic.orders["order"]
    oic.data["race"] = s.race(
        [
            lambda: s.post(
                oic, sel, s.ORDER, "UpdateOrderStatus", {"id": oid, "status": s.SHIPPED}
            ),
            lambda: s.post(oic, b, s.ORDER, "CancelOrder", {"id": oid}),
        ]
    )


@when(parsers.parse('"{seller}" calls CreateShipment for the order'))
def when_create_shipment(oic, seller):
    oic.responses["last"] = s.post(
        oic,
        _actor(oic, seller),
        s.ORDER,
        "CreateShipment",
        {
            "orderId": oic.orders["order"],
            "carrier": "SPX Express",
            "trackingCode": f"OIC{uuid.uuid4().hex[:10]}",
        },
    )
    oic.data["tracking"] = oic.responses["last"].request.content.decode()


@when(parsers.parse('"{buyer}" starts an online payment for the order'))
def when_open_payment(oic, buyer):
    oic.data["tx"] = s.open_payment(oic, _actor(oic, buyer), oic.orders["order"])


@when(parsers.parse('the payment of "{buyer}" succeeds'))
def when_payment_succeeds(oic, buyer):
    resp = s.settle_payment(oic, _actor(oic, buyer), oic.data["tx"])
    assert resp.status_code == 200, resp.text


@when(parsers.parse('the payment of "{buyer}" succeeds while "{buyer2}" cancels the order'))
def when_payment_cancel_race(oic, buyer, buyer2):
    b = _actor(oic, buyer)
    oid = oic.orders["order"]
    tx = oic.data["tx"]
    oic.data["race"] = s.race(
        [
            lambda: s.settle_payment(oic, b, tx),
            lambda: s.post(oic, b, s.ORDER, "CancelOrder", {"id": oid}),
        ]
    )


@then(parsers.parse('the order read by "{actor}" is {status}'))
def then_order_is(oic, actor, status):
    s.wait_status(oic, _actor(oic, actor), oic.orders["order"], _STATUS[status])


@then(parsers.parse('the order read by "{actor}" remains {status}'))
def then_order_still(oic, actor, status):
    s.stays_status(oic, _actor(oic, actor), oic.orders["order"], _STATUS[status], seconds=3)


@then(parsers.parse('the order read by "{actor}" stays {status} after the payment settles'))
def then_order_stays_after_payment(oic, actor, status):
    s.stays_status(oic, _actor(oic, actor), oic.orders["order"], _STATUS[status], seconds=8)


@then(parsers.parse('the order read by "{actor}" ends {status}'))
def then_order_ends(oic, actor, status):
    s.wait_status(oic, _actor(oic, actor), oic.orders["order"], _STATUS[status], timeout=30)
    s.stays_status(oic, _actor(oic, actor), oic.orders["order"], _STATUS[status], seconds=6)


@then("exactly one call succeeds and the other fails with failed_precondition")
def then_one_wins_cancel(oic):
    codes = sorted(s.code_of(r) for r in oic.data["race"])
    assert codes == ["failed_precondition", "ok"], [
        (r.status_code, r.text[:200]) for r in oic.data["race"]
    ]


@then(
    parsers.parse(
        "exactly one call succeeds and the order is Shipped with stock {shipped:d} "
        "or Cancelled with stock {cancelled:d}"
    )
)
def then_ship_or_cancel(oic, shipped, cancelled):
    results = oic.data["race"]
    wins = [r for r in results if r.status_code == 200]
    assert len(wins) == 1, [(r.status_code, r.text[:200]) for r in results]
    losers = [r for r in results if r.status_code != 200]
    assert s.code_of(losers[0]) == "failed_precondition", losers[0].text
    b = next(a for a in oic.actors.values() if a.role == "buyer")
    status = s.order_status(oic, b, oic.orders["order"])
    assert status in (s.SHIPPED, s.CANCELLED), status
    listing = next(iter(oic.listings))
    s.wait_stock(oic, listing, shipped if status == s.SHIPPED else cancelled, timeout=20, stay=4)


@then("no shipment exists for the order")
def then_no_shipment(oic):
    b = next(a for a in oic.actors.values() if a.role == "buyer")
    resp = s.post(oic, b, s.ORDER, "GetShipmentTracking", {"orderId": oic.orders["order"]})
    shipment = resp.json().get("shipment") if resp.status_code == 200 else None
    assert resp.status_code != 200 or not shipment, f"a shipment exists: {resp.text[:300]}"


@then("another buyer can still check out with that voucher")
def then_voucher_unused(oic):
    other = s.register(oic, "other", "buyer")
    s.ensure_address(oic, other)
    s.post(oic, other, s.CART, "ClearCart", {})
    s.add_to_cart(oic, other, "L", 1)
    resp = s.checkout(oic, other, voucher=oic.data["voucher"])
    assert resp.status_code == 200, f"voucher refused: HTTP {resp.status_code} {resp.text[:300]}"


# ── saga view / force-fail ───────────────────────────────────────────────
@when(parsers.parse('"{buyer}" reads the saga view of the order'), target_fixture="saga_view")
@then(parsers.parse('"{buyer}" reads the saga view of the order'), target_fixture="saga_view")
def read_saga(oic, buyer):
    return s.saga(oic, _actor(oic, buyer), oic.orders["order"])


@then(parsers.parse("saga step {n:d} is {status}"))
def then_saga_step(saga_view, n, status):
    got = s.step(saga_view, f"{n}.").get("status")
    assert got == status, f"step {n} is {got!r}, expected {status}: {saga_view}"


@then("saga step 3 carries a timestamp")
def then_saga_ts(saga_view):
    assert s.step(saga_view, "3.").get("timestamp"), saga_view


@then("the saga view reports no compensation")
def then_no_comp(saga_view):
    assert not saga_view.get("isCompensated"), saga_view
    assert all(st.get("status") != "COMPENSATED" for st in saga_view.get("steps", [])), saga_view


@then("the saga view is not compensated")
def then_not_comp(saga_view):
    assert not saga_view.get("isCompensated"), saga_view


@then("the saga view is compensated")
def then_comp(saga_view):
    assert saga_view.get("isCompensated") is True, saga_view


@when(
    parsers.parse('"{buyer}" calls ForceFailSaga with fail_step "{fail_step}" on the order'),
    target_fixture="force_fail",
)
def when_force_fail(oic, buyer, fail_step):
    resp = s.post(
        oic,
        _actor(oic, buyer),
        s.ORDER,
        "ForceFailSaga",
        {"orderId": oic.orders["order"], "failStep": fail_step},
    )
    oic.responses["last"] = resp
    return resp.json() if resp.status_code == 200 else {}


@then("ForceFailSaga reports success")
def then_ff_success(force_fail):
    assert force_fail.get("success") is True, force_fail


@then("ForceFailSaga reports failure with a message saying the stock release is pending retry")
def then_ff_failure(force_fail):
    assert force_fail, "ForceFailSaga failed outright"
    assert not force_fail.get("success"), force_fail
    msg = force_fail.get("message", "").lower()
    assert "retry" in msg, force_fail


@then("the returned saga view is compensated")
def then_ff_saga(force_fail):
    assert (force_fail.get("sagaState") or {}).get("isCompensated") is True, force_fail


# ── upstream principals ──────────────────────────────────────────────────
@when("the reservation TTL plus two sweep intervals pass")
def when_ttl_passes(oic):
    time.sleep(s.TTL_WAIT_S)


@then("the order exists and is Pending")
def then_order_exists(oic):
    b = next(a for a in oic.actors.values() if a.role == "buyer")
    ids = s.order_ids(oic.responses["last"])
    assert len(ids) == 1
    oic.orders["order"] = ids[0]
    assert s.order_status(oic, b, ids[0]) == s.PENDING
