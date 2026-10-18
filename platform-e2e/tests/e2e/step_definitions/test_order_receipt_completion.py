# ruff: noqa: F811
"""Binds order/order_receipt_completion.feature (order-receipt-completion, q10-order-lifecycle).

Black box through the gateway with real, distinct users; the completion and return facts are read
off ``order.events``. Reuses the oic actor/listing steps, the plp payment helpers and the prm
return helpers; the timers come from compose/order-inventory.override.yaml (auto-complete 12s,
return window 20s, return auto-approve 30s, ship-by 60s, sweep 2s).
"""

from __future__ import annotations

import time

from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.oic_order_steps import oic  # noqa: F401  (fixture)
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_rma_support as r

scenarios("order/order_receipt_completion.feature")

_SLOT = {"order": "order", "control order": "control", "paid order": "paid"}
_RET = {"return": "return", "control return": "control"}
_SLOTS = r"(?P<slot>order|control order|paid order)"
_METHOD = {"MOCK_MOMO": "PAYMENT_METHOD_MOCK_MOMO", "COD": "PAYMENT_METHOD_COD"}
_ST = {"Shipped": s.SHIPPED, "Paid": s.PAID, "Completed": s.COMPLETED}
_RST = {
    "PENDING": r.PENDING,
    "APPROVED": r.APPROVED,
    "REJECTED": r.REJECTED,
    "REFUNDED": r.REFUNDED,
}


def any_step(pattern):
    """Register one function as a Given, When and Then step (the feature may use And)."""

    def deco(fn):
        for kind in (given, when, then):
            kind(pattern, stacklevel=2)(fn)
        return fn

    return deco


# ── helpers ──────────────────────────────────────────────────────────────
def _oid(w, slot):
    return w.orders[_SLOT.get(slot, slot)]


def _buyer(w, slot):
    return w.data[f"{_SLOT.get(slot, slot)}:buyer"]


def _seller(w, slot):
    return w.data[f"{_SLOT.get(slot, slot)}:seller"]


def _order(w, slot="order") -> dict:
    return s.get_order(w, _buyer(w, slot), _oid(w, slot))


def _rid(w, which="return"):
    return w.data[f"ret:{_RET[which]}"]


def _register(w, slot, order_id, buyer, seller):
    key = _SLOT.get(slot, slot)
    w.orders[key] = order_id
    w.data[f"{key}:buyer"] = buyer
    w.data[f"{key}:seller"] = seller


def _place_cod(w, buyer, listing):
    return ol.place(w, buyer, listing, 1, "PAYMENT_METHOD_COD")


def _ship(w, slot, order_id, seller):
    ol.ship(w, order_id, seller)
    w.data[f"{_SLOT.get(slot, slot)}:shipped_t"] = time.time()


# ── catalogue and orders ─────────────────────────────────────────────────
@given(parsers.parse('a seller "{name}" with a listing "{listing}" priced {price:d}'))
def given_seller_priced(oic, name, listing, price):
    seller = oic.actors.get(name) or s.register(oic, name, "seller")
    s.create_listing(oic, listing, seller, 50, price=price)


@any_step(
    parsers.re(
        r'"(?P<b>\w+)" has (?P<which>a|a second|a third) '
        r'(?P<kind>shipped order paid online|shipped order|paid online order) for "(?P<l>\w+)"'
    )
)
def has_order(oic, b, which, kind, l):  # noqa: E741
    slot = {"a": "order", "a second": "control", "a third": "paid"}[which]
    buyer = oic.actors[b]
    seller = oic.listings[l]["seller"]
    if kind == "shipped order":
        order_id = _place_cod(oic, buyer, l)
        _register(oic, slot, order_id, buyer, seller)
        _ship(oic, slot, order_id, seller)
        return
    order_id, _tx = p.place_and_pay(oic, buyer, l)
    _register(oic, slot, order_id, buyer, seller)
    if kind == "shipped order paid online":
        _ship(oic, slot, order_id, seller)


@any_step(parsers.parse("a paid online order of {amount:d} shipped by its seller"))
def paid_shipped(oic, amount):
    order_id, _tx = r.paid_order(oic, amount, credited=True)
    seller = oic.actors["seller"]
    _register(oic, "order", order_id, oic.actors["b1"], seller)
    _ship(oic, "order", order_id, seller)


@any_step(parsers.parse('"{b}" places a {method} order for {qty:d} of "{listing}"'))
def places_order(oic, b, method, qty, listing):
    buyer = oic.actors[b]
    order_id = ol.place(oic, buyer, listing, qty, _METHOD[method])
    _register(oic, "order", order_id, buyer, oic.listings[listing]["seller"])


@any_step(parsers.parse('"{b}" pays the order'))
def pays_order(oic, b):
    buyer = oic.actors[b]
    tx_id = s.open_payment(oic, buyer, oic.orders["order"])
    s.ok(s.settle_payment(oic, buyer, tx_id))
    s.wait_status(oic, buyer, oic.orders["order"], s.PAID)


@any_step(parsers.re(r'"(?P<a>\w+)" calls UpdateOrderStatus to Completed on the paid order'))
def update_paid_to_completed(oic, a):
    oic.responses["last"] = s.post(
        oic,
        oic.actors[a],
        s.ORDER,
        "UpdateOrderStatus",
        {"id": oic.orders["paid"], "status": s.COMPLETED},
    )


@any_step(parsers.parse('"{seller}" ships the order'))
def seller_ships(oic, seller):
    ol.ship(oic, oic.orders["order"], oic.actors[seller])
    oic.data["order:shipped_t"] = time.time()


# ── confirm receipt ──────────────────────────────────────────────────────
@any_step(parsers.re(rf'"(?P<a>\w+)" confirms receipt of the {_SLOTS}'))
def confirms(oic, a, slot):
    oic.responses["last"] = s.post(
        oic, oic.actors[a], s.ORDER, "ConfirmReceipt", {"id": _oid(oic, slot)}
    )


@any_step(parsers.parse('"{b}" sends {n:d} concurrent confirms of the order'))
def concurrent_confirms(oic, b, n):
    buyer, oid = oic.actors[b], oic.orders["order"]
    oic.data["race"] = s.race(
        [lambda: s.post(oic, buyer, s.ORDER, "ConfirmReceipt", {"id": oid})] * n
    )


@then(
    parsers.parse(
        'exactly one confirm succeeds and the other {n:d} fail with "{code}"',
    )
)
def one_confirm_wins(oic, n, code):
    codes = [s.code_of(x) for x in oic.data["race"]]
    assert codes.count("ok") == 1, codes
    assert [c for c in codes if c != "ok"] == [code] * n, codes


@then(parsers.re(r'the call is refused with "(?P<code>\w+)" and the message "(?P<msg>[^"]+)"'))
def fails_with_message(oic, code, msg):
    resp = oic.responses["last"]
    assert s.code_of(resp) == code, f"HTTP {resp.status_code}: {resp.text[:300]}"
    got = resp.json().get("message", "")
    assert msg in got, f"message {got!r} lacks {msg!r}"


# ── order reads ──────────────────────────────────────────────────────────
@then(
    "the order reads Completed with completed_at set, no auto_complete_at and the configured "
    "return window"
)
def order_completed_window(oic):
    o = _order(oic)
    assert o.get("status") == s.COMPLETED, o
    assert o.get("completedAt"), o
    assert not o.get("autoCompleteAt"), o
    gap = ol.ts(o.get("returnWindowEndsAt")) - ol.ts(o["completedAt"])
    assert abs(gap - ol.RETURN_WINDOW_S) <= 1.5, f"window {gap}s, want {ol.RETURN_WINDOW_S}: {o}"


@then("the order reads Completed with completed_at set and no auto_complete_at")
def order_completed_plain(oic):
    s.wait_status(
        oic, _buyer(oic, "order"), oic.orders["order"], s.COMPLETED, timeout=ol.STATUS_WAIT_S
    )
    o = _order(oic)
    assert o.get("completedAt"), o
    assert not o.get("autoCompleteAt"), o


@then(parsers.re(r"the order still reads (?P<st>Shipped|Paid) with no completed_at"))
def order_still(oic, st):
    o = _order(oic)
    assert o.get("status") == _ST[st], o
    assert not o.get("completedAt"), o


@then(
    "the order reads Shipped with shipped_at set and auto_complete_at one auto-complete window "
    "later"
)
def order_shipped_stamp(oic):
    o = _order(oic)
    assert o.get("status") == s.SHIPPED, o
    assert o.get("shippedAt"), o
    gap = ol.ts(o.get("autoCompleteAt")) - ol.ts(o["shippedAt"])
    assert abs(gap - ol.AUTO_COMPLETE_S) <= 1.5, f"gap {gap}s, want {ol.AUTO_COMPLETE_S}: {o}"


@any_step(parsers.re(rf"the {_SLOTS} is recorded"))
def order_recorded(oic, slot):
    o = _order(oic, slot)
    oic.data[f"snap:{slot}"] = (o.get("status"), o.get("completedAt"))


@then(parsers.re(rf"the {_SLOTS} is unchanged"))
def order_unchanged(oic, slot):
    o = _order(oic, slot)
    assert (o.get("status"), o.get("completedAt")) == oic.data[f"snap:{slot}"], o


@when("the auto-complete window passes without a confirm")
def auto_complete_passes(oic):
    s.wait_status(
        oic, _buyer(oic, "order"), oic.orders["order"], s.COMPLETED, timeout=ol.STATUS_WAIT_S
    )


@then("the order stays Shipped past its auto-complete deadline")
def stays_shipped(oic):
    end = oic.data["order:shipped_t"] + ol.AUTO_COMPLETE_S + 4 * ol.SWEEP_S
    b, oid = _buyer(oic, "order"), oic.orders["order"]
    while True:
        got = s.order_status(oic, b, oid)
        assert got == s.SHIPPED, f"order {oid} is {got!r} while a return is open"
        if time.time() >= end:
            return
        time.sleep(1)


@when(parsers.re(rf"the return window of the {_SLOTS} has passed"))
def window_passed(oic, slot):
    o = _order(oic, slot)
    assert o.get("returnWindowEndsAt"), f"order has no return window: {o}"
    ol.sleep_until(ol.ts(o["returnWindowEndsAt"]) + 1.5)


# ── returns ──────────────────────────────────────────────────────────────
@any_step(parsers.re(rf'"(?P<b>\w+)" requests a return of (?P<amt>\d+) on the {_SLOTS}'))
def requests_return(oic, b, amt, slot):
    resp = r.request_return(oic, oic.actors[b], _oid(oic, slot), int(amt))
    oic.responses["last"] = resp
    if resp.status_code == 200:
        oic.data[f"ret:{'control' if slot == 'control order' else 'return'}"] = (
            resp.json().get("returnRequest", {}).get("id", "")
        )


@any_step(parsers.re(rf'"(?P<b>\w+)" has a return of (?P<amt>\d+) on the {_SLOTS}'))
def has_return(oic, b, amt, slot):
    requests_return(oic, b, amt, slot)
    ret = s.ok(oic.responses["last"]).get("returnRequest", {})
    assert ret.get("status") == r.PENDING, ret


@any_step(
    parsers.re(
        r'"(?P<a>\w+)" (?P<verb>approves|rejects|refunds) the (?P<which>return|control return)'
    )
)
def update_return(oic, a, verb, which):
    status = {"approves": r.APPROVED, "rejects": r.REJECTED, "refunds": r.REFUNDED}[verb]
    oic.responses["last"] = r.set_return_status(oic, oic.actors[a], _rid(oic, which), status)


def _return_json(w, actor, which="return") -> dict:
    resp = s.post(w, actor, s.ORDER, "GetReturnRequest", {"id": _rid(w, which)})
    return s.ok(resp).get("returnRequest", {})


@then(parsers.re(r'the return reads (?P<st>[A-Z]+) for "(?P<a>\w+)"'))
def return_reads(oic, st, a):
    got = _return_json(oic, oic.actors[a]).get("status")
    assert got == _RST[st], f"return reads {got!r}, want {_RST[st]!r}"


@then(parsers.re(r'the return reads can_escalate (?P<flag>true|false) for "(?P<a>\w+)"'))
def return_can_escalate(oic, flag, a):
    ret = _return_json(oic, oic.actors[a])
    assert bool(ret.get("canEscalate", False)) is (flag == "true"), ret


def _wait_return(w, actor, which, status, timeout):
    deadline = time.monotonic() + timeout
    got = ""
    while time.monotonic() < deadline:
        got = _return_json(w, actor, which).get("status", "")
        if got == status:
            return
        time.sleep(1)
    raise AssertionError(f"return {which} reads {got!r}, want {status!r}")


_APPROVE_WAIT_S = ol.AUTO_APPROVE_S + 4 * ol.SWEEP_S + 25


@when("the system approves the return after the auto-approve window")
def system_approves(oic):
    _wait_return(oic, _buyer(oic, "order"), "return", r.APPROVED, _APPROVE_WAIT_S)


@when("the control return is approved by the system")
def control_approved(oic):
    _wait_return(oic, _buyer(oic, "order"), "control return", r.APPROVED, _APPROVE_WAIT_S)


@then("the order has no returns")
def order_no_returns(oic):
    got = s.ok(r.list_returns(oic, _buyer(oic, "order"), oic.orders["order"])).get("returns", [])
    assert got == [], got


# ── order.events ─────────────────────────────────────────────────────────
@then(
    parsers.re(
        rf"exactly one OrderCompleted is published for the {_SLOTS} "
        r"with completed_by (?P<who>BUYER|SELLER|SYSTEM)"
    )
)
def one_completed(oic, slot, who):
    evs = ol.payloads_for(_oid(oic, slot), ol.COMPLETED_TYPE, wait_s=30)
    assert len(evs) == 1, f"expected one OrderCompleted, got {len(evs)}"
    assert ol.pb_int(evs[0], 6) == ol.COMPLETED_BY[who], ol.pb(evs[0])
    assert ol.pb_str(evs[0], 1) == _oid(oic, slot)
    oic.data["completed_evt"] = evs[0]


@then(
    "that OrderCompleted carries the order total, completed_at, return_window_ends_at and "
    "paid_online true"
)
def completed_fields(oic):
    evt, o = oic.data["completed_evt"], _order(oic)
    assert ol.pb_int(evt, 4) == int(o["totalAmount"]), (ol.pb(evt), o)
    assert ol.pb_ts(evt, 7) == int(ol.ts(o["completedAt"])), (ol.pb(evt), o)
    assert ol.pb_ts(evt, 8) == int(ol.ts(o["returnWindowEndsAt"])), (ol.pb(evt), o)
    assert ol.pb_int(evt, 9) == 1, ol.pb(evt)


@then(parsers.re(rf"no OrderCompleted is published for the {_SLOTS}"))
def no_completed(oic, slot):
    evs = ol.payloads_for(_oid(oic, slot), ol.COMPLETED_TYPE, wait_s=4, settle_s=0)
    assert evs == [], f"unexpected OrderCompleted: {len(evs)}"


@then(
    parsers.re(
        rf"exactly one ReturnRequested is published for the {_SLOTS} with the buyer, the seller "
        r"and refund_amount (?P<amt>\d+)"
    )
)
def one_return_requested(oic, slot, amt):
    evs = ol.payloads_for(_oid(oic, slot), ol.RETURN_REQUESTED_TYPE, wait_s=30)
    assert len(evs) == 1, f"expected one ReturnRequested, got {len(evs)}"
    e = evs[0]
    assert ol.pb_str(e, 2) == _oid(oic, slot)
    assert ol.pb_str(e, 3) == _buyer(oic, slot).user_id
    assert ol.pb_str(e, 4) == _seller(oic, slot).user_id
    assert ol.pb_int(e, 5) == int(amt), ol.pb(e)


@then(parsers.re(rf"no ReturnRequested is published for the {_SLOTS}"))
def no_return_requested(oic, slot):
    evs = ol.payloads_for(_oid(oic, slot), ol.RETURN_REQUESTED_TYPE, wait_s=4, settle_s=0)
    assert evs == [], f"unexpected ReturnRequested: {len(evs)}"


@then("no ReturnRefunded is published for the return")
def no_return_refunded(oic):
    evs = ol.payloads_for(_rid(oic), ol.RETURN_REFUNDED_TYPE, wait_s=4, settle_s=0)
    assert evs == [], f"unexpected ReturnRefunded: {len(evs)}"


@then("exactly one ReturnRefunded is published for the return")
def one_return_refunded(oic):
    evs = ol.payloads_for(_rid(oic), ol.RETURN_REFUNDED_TYPE, wait_s=40)
    assert len(evs) == 1, f"expected one ReturnRefunded, got {len(evs)}"


# ── ship_by ──────────────────────────────────────────────────────────────
@then("the order reads ship_by at paid_at plus the ship-by window")
def ship_by_paid(oic):
    o = _order(oic)
    assert o.get("paidAt"), o
    assert o.get("shipBy"), f"paid order has no shipBy: {o}"
    gap = ol.ts(o["shipBy"]) - ol.ts(o["paidAt"])
    assert abs(gap - ol.SHIP_BY_S) <= 1.5, f"ship_by gap {gap}s, want {ol.SHIP_BY_S}: {o}"


@then("the OrderPaidEvent of the order carries the same ship_by and the seller id")
def paid_event_ship_by(oic):
    o = _order(oic)
    evs = ol.payloads_for(oic.orders["order"], ol.PAID_TYPE, wait_s=30)
    assert len(evs) == 1, len(evs)
    assert ol.pb_ts(evs[0], 8) == int(ol.ts(o["shipBy"])), (ol.pb(evs[0]), o)
    assert ol.pb_str(evs[0], 7) == _seller(oic, "order").user_id


@then("the order reads ship_by at its creation plus the ship-by window")
def ship_by_cod(oic):
    o = _order(oic)
    assert o.get("shipBy"), f"COD order has no shipBy: {o}"
    gap = ol.ts(o["shipBy"]) - ol.ts(o["createdAt"])
    assert abs(gap - ol.SHIP_BY_S) <= 1.5, f"ship_by gap {gap}s, want {ol.SHIP_BY_S}: {o}"


@then("the OrderPlaced of the order carries the same ship_by")
def placed_event_ship_by(oic):
    o = _order(oic)
    evs = ol.payloads_for(oic.orders["order"], ol.PLACED_TYPE, wait_s=30)
    assert len(evs) == 1, len(evs)
    assert ol.pb_ts(evs[0], 9) == int(ol.ts(o["shipBy"])), (ol.pb(evs[0]), o)


@then("the order reads no ship_by and its OrderPlaced carries none")
def no_ship_by(oic):
    o = _order(oic)
    assert o.get("status") == s.PENDING, o
    assert not o.get("shipBy"), o
    evs = ol.payloads_for(oic.orders["order"], ol.PLACED_TYPE, wait_s=30)
    assert len(evs) == 1, f"no OrderPlaced for the online order: {len(evs)}"
    assert ol.pb_ts(evs[0], 9) == 0, ol.pb(evs[0])
