"""Steps for payment-refund-model / return-refund-settlement (area prm-rma).

Black box through the gateway with real, distinct users (one seller and its buyers per scenario).
The `ReturnRefunded` facts and dead letters are read off Kafka (`order.events`, the settlement
DLQ); the reference of a ledger row (not on the wire) is read with psql. Absence assertions are
tail scans or follow a sentinel order of the same seller, whose credit proves the single ordered
settlement consumer has applied (or parked) everything produced before it. Scenarios that stop
team-payment, replay or hand-produce Kafka records are tagged @destructive in the feature.
"""

from __future__ import annotations

import time
import uuid

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import oic_order_support as o
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_rma_support as r
from tests.e2e.support.oic_order_support import Actor, OicWorld, code_of, ok, post, race

_STATUS = {
    "PENDING": r.PENDING,
    "APPROVED": r.APPROVED,
    "REJECTED": r.REJECTED,
    "REFUNDED": r.REFUNDED,
}
_PAYMENT = {"PARTIALLY_REFUNDED": r.PAY_PARTIAL, "REFUNDED": r.PAY_REFUNDED, "PAID": p.PAID}
_SOURCE = {"RETURN": r.SRC_RETURN, "ORDER_CANCEL": r.SRC_CANCEL}
_W = r"(?:within the settle window )?"


def any_step(pattern):
    """Register one function as a Given, When and Then step (the feature may use And)."""

    def deco(fn):
        for kind in (given, when, then):
            kind(pattern, stacklevel=2)(fn)
        return fn

    return deco


@pytest.fixture
def prm():
    w = OicWorld()
    w.data.update(returns={}, reasons={}, sentinels=0)
    yield w
    if w.data.get("payment_stopped"):  # never leave team-payment stopped
        stack.start_payment()
        stack.wait_settled("team-payment")  # the next scenario must not hit a stale IP
        w.data.pop("payment_stopped", None)


def _seller(w: OicWorld) -> Actor:
    return w.actors["seller"]


def _rid(w: OicWorld, name: str) -> str:
    return w.data["returns"][name]


def _order(w: OicWorld) -> str:
    return w.orders["order"]


def _last(w: OicWorld):
    return w.responses["last"]


# ── catalogue and orders ─────────────────────────────────────────────────
@any_step(parsers.parse("a seller with a listing whose order pays {amount:d}"))
def seller_with_listing(prm, amount):
    p.seller_with_listing(prm, amount, "L")
    prm.data["amount"] = amount


@any_step(parsers.parse('a buyer "{name}"'))
def a_buyer(prm, name):
    p.buyer(prm, name)


@any_step(parsers.parse('"{b}" has paid an order of the listing'))
def has_paid(prm, b):
    order_id, tx_id = p.place_and_pay(prm, p.buyer(prm, b), "L")
    prm.orders["order"] = order_id
    prm.data["tx_id"] = tx_id


@any_step(parsers.parse('"{b}" has paid an order of the listing and the seller is credited'))
def has_paid_credited(prm, b):
    has_paid(prm, b)
    p.wait_rows(prm, _seller(prm), p.SETTLEMENT, prm.data["amount"], 1)


@any_step(parsers.parse('"{b}" pays a new order of the listing'))
def pays_new_order(prm, b):
    p.place_and_pay(prm, p.buyer(prm, b), "L")


@any_step(
    parsers.parse(
        '"{b}" has a cash-on-delivery order of the listing that the seller handed over '
        "without an online payment"
    )
)
def has_cod(prm, b):
    buyer = p.buyer(prm, b)
    order_id = o.place_order(prm, buyer, "L", 1)
    o.put_in_status(prm, order_id, buyer, _seller(prm), o.SHIPPED)
    prm.orders["order"] = order_id


@any_step(parsers.parse('"{b}" has an unpaid order of the listing'))
def has_unpaid(prm, b):
    buyer = p.buyer(prm, b)
    prm.orders["order"] = o.place_order(prm, buyer, "L", 1)
    prm.data["unpaid_buyer"] = buyer


@any_step(parsers.parse('"{b}" cancels the order'))
def cancels(prm, b):
    ok(post(prm, prm.actors[b], r.ORDER, "CancelOrder", {"id": _order(prm), "reason": "e2e"}))


# ── returns ──────────────────────────────────────────────────────────────
@any_step(parsers.parse('"{b}" requests the return "{name}" of {amount:d}'))
def requests_return(prm, b, name, amount):
    reason = f"e2e-{name}-{uuid.uuid4().hex[:6]}"
    ret = r.new_return(prm, prm.actors[b], _order(prm), amount, reason)
    assert ret.get("id"), "the return request was refused"
    assert ret.get("status") == r.PENDING, ret
    prm.data["returns"][name] = ret["id"]
    prm.data["reasons"][name] = reason


@any_step(parsers.parse('"{b}" requests a return of {amount:d}'))
def requests_return_raw(prm, b, amount):
    prm.responses["last"] = r.request_return(prm, prm.actors[b], _order(prm), amount)


@any_step(parsers.parse('the seller approves the return "{name}"'))
def seller_approves(prm, name):
    r.approve(prm, _seller(prm), _rid(prm, name))


@any_step(parsers.parse('the seller rejects the return "{name}"'))
def seller_rejects(prm, name):
    ok(r.set_return_status(prm, _seller(prm), _rid(prm, name), r.REJECTED))


@any_step(parsers.parse('the seller moves the return "{name}" to REFUNDED'))
def seller_refunds(prm, name):
    prm.responses["last"] = r.set_return_status(prm, _seller(prm), _rid(prm, name), r.REFUNDED)
    prm.data["refunded_at"] = time.time()


@any_step(parsers.parse('"{b}" moves the return "{name}" to REFUNDED'))
def buyer_moves(prm, b, name):
    prm.responses["last"] = r.set_return_status(prm, prm.actors[b], _rid(prm, name), r.REFUNDED)


@any_step(
    parsers.parse(
        'the seller sends {n:d} concurrent UpdateReturnStatus calls moving the return "{name}" '
        "to REFUNDED"
    )
)
def concurrent_refunds(prm, n, name):
    seller, rid = _seller(prm), _rid(prm, name)
    prm.data["race"] = race([lambda: r.set_return_status(prm, seller, rid, r.REFUNDED)] * n)


@any_step(parsers.parse('"{b}" sends {n:d} concurrent return requests of {amount:d}'))
def concurrent_requests(prm, b, n, amount):
    buyer, order_id = prm.actors[b], _order(prm)
    prm.data["race"] = race([lambda: r.request_return(prm, buyer, order_id, amount)] * n)
    prm.data["lister"] = buyer


@any_step(parsers.parse("the seller refunds {amount:d} of the payment directly"))
def seller_direct_refund(prm, amount):
    ok(r.seller_refund(prm, _seller(prm), prm.data["tx_id"], amount))


@then(parsers.re(r'the return "(?P<name>\w+)" reads (?P<status>[A-Z]+)$'))
def return_reads(prm, name, status):
    got = r.return_status(prm, _seller(prm), _rid(prm, name))
    assert got == _STATUS[status], f"return {name} reads {got}, want {_STATUS[status]}"


@then(parsers.re(r'the call fails with "(?P<code>[a-z_]+)"'))
def call_fails(prm, code):
    got = code_of(_last(prm))
    assert got == code, f"want {code}, got {p.describe(_last(prm))}"


@then(parsers.re(r'the call fails with "(?P<code>[a-z_]+)" and message "(?P<msg>.+)"'))
def call_fails_with_message(prm, code, msg):
    got = code_of(_last(prm))
    assert got == code, f"want {code}, got {p.describe(_last(prm))}"
    assert msg in p.message_of(_last(prm)), f"message {p.message_of(_last(prm))!r}, want {msg!r}"


@then(
    parsers.re(
        r"exactly (?P<n>\d+) of the calls succeeds? and the others fail with "
        r'"(?P<code>[a-z_]+)"'
    )
)
def race_outcome(prm, n, code):
    codes = [code_of(x) for x in prm.data["race"]]
    wins = codes.count("ok")
    assert wins == int(n), f"{wins} call(s) succeeded, want {n}: {codes}"
    others = [c for c in codes if c != "ok"]
    assert all(c == code for c in others), f"others failed with {others}, want {code}"


@then(parsers.parse("ListOrderReturns lists {n:d} returns totalling {total:d}"))
def list_totals(prm, n, total):
    got = ok(r.list_returns(prm, prm.data["lister"], _order(prm))).get("returns", [])
    assert len(got) == n, f"{len(got)} returns listed, want {n}: {got}"
    assert sum(int(x["refundAmount"]) for x in got) == total, got


@when("the seller lists the returns of the order through the gateway")
def seller_lists(prm):
    prm.responses["last"] = r.list_returns(prm, _seller(prm), _order(prm))


@when(parsers.parse('"{b}" lists the returns of the order through the gateway'))
def buyer_lists(prm, b):
    prm.responses["last"] = r.list_returns(prm, prm.actors[b], _order(prm))


@then("both returns are listed newest first, each with its reason, refund amount and status")
def listed_newest_first(prm):
    got = ok(_last(prm)).get("returns", [])
    assert [x["id"] for x in got] == [_rid(prm, "R2"), _rid(prm, "R1")], got
    for name, amount in (("R1", 200000), ("R2", 100000)):
        row = next(x for x in got if x["id"] == _rid(prm, name))
        assert row["reason"] == prm.data["reasons"][name], row
        assert int(row["refundAmount"]) == amount, row
        assert row["status"] == r.PENDING, row


# ── order.events facts and dead letters ──────────────────────────────────
@then(
    parsers.re(
        r"order\.events carries exactly (?P<n>\d+) ReturnRefunded records? for the return "
        r'"(?P<name>\w+)"(?P<rest>.*)'
    )
)
def fact_count(prm, n, name, rest):
    records = r.wait_fact(_rid(prm, name)) if int(n) else r.return_facts(_rid(prm, name), 5.0)
    assert len(records) == int(n), f"{len(records)} ReturnRefunded record(s), want {n}"
    if "keyed by the order id" in rest:
        fact = r.decode_fact(records[0])
        assert records[0]["key"] == _order(prm).encode(), records[0]["key"]
        want = int(rest.rsplit("refund_amount", 1)[1])
        assert fact.get("refund_amount") == want, fact
        assert fact.get("return_id") == _rid(prm, name) and fact.get("order_id") == _order(prm)


@then(parsers.parse('order.events carries no ReturnRefunded record for the return "{name}"'))
def no_fact(prm, name):
    found = r.return_facts(_rid(prm, name), tail_s=5.0)
    assert not found, f"{len(found)} ReturnRefunded record(s) on order.events for return {name}"


@then(parsers.parse('the settlement dead-letter topic has no record for the return "{name}"'))
def no_dlq(prm, name):
    found = r.dlq_records(_rid(prm, name), tail_s=3.0)
    assert not found, f"{len(found)} record(s) on {stack.DLQ_TOPIC} for return {name}"


@any_step(
    parsers.parse(
        'the ReturnRefunded record of the return "{name}" is produced to order.events again, '
        "byte for byte"
    )
)
def replay(prm, name):
    records = r.wait_fact(_rid(prm, name))
    assert records, f"no ReturnRefunded record of {name} on order.events"
    rec = records[0]
    stack.produce(stack.ORDER_EVENTS_TOPIC, rec["key"], rec["value"], rec["headers"] or None)


@any_step("a sentinel order of the seller is credited")
def sentinel(prm):
    p.settle_sentinel(prm, _seller(prm))


@any_step("a well-formed ReturnRefunded record for that unpaid order is produced to order.events")
def produce_fact_unpaid(prm):
    buyer, seller = prm.data["unpaid_buyer"], _seller(prm)
    prm.data["ghost_return"] = str(uuid.uuid4())
    key, value = r.build_return_refunded(
        prm.data["ghost_return"], _order(prm), buyer.user_id, seller.user_id, 200000
    )
    stack.produce(stack.ORDER_EVENTS_TOPIC, key, value)


@then("the ReturnRefunded record appears on the settlement dead-letter topic")
def fact_on_dlq(prm):
    deadline = time.monotonic() + 40
    while time.monotonic() < deadline:
        if r.dlq_records(prm.data["ghost_return"]):
            return
        time.sleep(1)
    raise AssertionError(
        f"ReturnRefunded {prm.data['ghost_return']} is not on {stack.DLQ_TOPIC} (not dead-lettered)"
    )


# ── what team-payment applied ────────────────────────────────────────────
@then(
    parsers.re(
        _W
        + r'the payment read by "(?P<b>\w+)" is (?P<status>[A-Z_]+) with (?P<amount>\d+) refunded'
    )
)
def payment_reads(prm, b, status, amount):
    r.wait_refunded(prm, prm.actors[b], _order(prm), int(amount), _PAYMENT[status])


@given(
    parsers.re(
        _W
        + r'the payment read by "(?P<b>\w+)" is (?P<status>[A-Z_]+) with (?P<amount>\d+) refunded'
    )
)
def payment_reads_given(prm, b, status, amount):
    payment_reads(prm, b, status, amount)


@then(
    parsers.re(
        r'the payment lists (?P<n>\d+) RETURN refunds? whose source id is the return "(?P<name>\w+)"'
    )
)
def lists_return_refund(prm, n, name):
    tx = p.payment_of(prm, prm.actors["b1"], _order(prm))
    got = r.refunds_of(tx, r.SRC_RETURN)
    assert len(got) == int(n), f"{len(got)} RETURN refund(s), want {n}: {tx.get('refunds')}"
    assert got[0].get("sourceId") == _rid(prm, name), got


@then(parsers.re(r"the payment lists (?P<n>\d+) RETURN refunds$"))
def lists_return_refunds(prm, n):
    tx = p.payment_of(prm, prm.actors["b1"], _order(prm))
    got = r.refunds_of(tx, r.SRC_RETURN)
    assert len(got) == int(n), f"{len(got)} RETURN refund(s), want {n}: {tx.get('refunds')}"


@then(
    parsers.re(
        r'the payment read by "(?P<b>\w+)" lists (?P<n>\d+) RETURN refunds? and has '
        r"(?P<amount>\d+) refunded"
    )
)
def lists_and_has(prm, b, n, amount):
    tx = p.payment_of(prm, prm.actors[b], _order(prm))
    assert len(r.refunds_of(tx, r.SRC_RETURN)) == int(n), tx.get("refunds")
    assert r.refunded_of(tx) == int(amount), tx


@then(
    parsers.re(
        r"the payment lists (?P<nr>\d+) RETURN refunds? and (?P<nc>\d+) ORDER_CANCEL refunds? "
        r"whose applied amounts sum to (?P<total>\d+)"
    )
)
def lists_return_and_cancel(prm, nr, nc, total):
    tx = p.payment_of(prm, prm.actors["b1"], _order(prm))
    returns, cancels = r.refunds_of(tx, r.SRC_RETURN), r.refunds_of(tx, r.SRC_CANCEL)
    assert (len(returns), len(cancels)) == (int(nr), int(nc)), tx.get("refunds")
    assert sum(r.applied(x) for x in returns + cancels) == int(total), tx.get("refunds")


@then(
    parsers.re(
        r'the RETURN refund of the return "(?P<name>\w+)" lists requested (?P<req>\d+) and '
        r"applied (?P<app>\d+)"
    )
)
def return_refund_pair(prm, name, req, app):
    refund = r.wait_return_refund(prm, prm.actors["b1"], _order(prm), _rid(prm, name))
    assert (r.requested(refund), r.applied(refund)) == (int(req), int(app)), refund


# ── the seller's ledger ──────────────────────────────────────────────────
@then(
    parsers.re(
        _W + r"the seller has exactly (?P<n>\d+) REFUND_DEDUCTION of (?P<amount>-\d+) referencing "
        r'the return "(?P<name>\w+)"'
    )
)
def one_deduction(prm, n, amount, name):
    ref = f"return:{_rid(prm, name)}"
    deadline = time.monotonic() + p.SETTLE_WAIT_S
    got: list[dict] = []
    while time.monotonic() < deadline:
        got = [d for d in r.deductions(prm, _seller(prm)) if d["reference"] == ref]
        if len(got) >= int(n):
            break
        time.sleep(0.5)
    assert len(got) == int(n), f"{len(got)} deduction(s) of {ref}, want {n}: {got}"
    assert all(d["amount"] == int(amount) for d in got), got


@then(
    parsers.re(
        r'the seller has a REFUND_DEDUCTION of (?P<amount>-\d+) referencing the return "(?P<name>\w+)"'
    )
)
def has_deduction(prm, amount, name):
    ref = f"return:{_rid(prm, name)}"
    got = [d for d in r.deductions(prm, _seller(prm)) if d["reference"] == ref]
    assert [d["amount"] for d in got] == [int(amount)], f"deductions of {ref}: {got}"


@then(
    parsers.re(
        r'the seller still has exactly (?P<n>\d+) REFUND_DEDUCTION referencing the return "(?P<name>\w+)"'
    )
)
def still_one_deduction(prm, n, name):
    ref = f"return:{_rid(prm, name)}"
    got = [d for d in r.deductions(prm, _seller(prm)) if d["reference"] == ref]
    assert len(got) == int(n), f"{len(got)} deduction(s) of {ref}, want {n}: {got}"


@then(
    parsers.re(
        r'the seller has exactly (?P<n>\d+) REFUND_DEDUCTION referencing the return "(?P<name>\w+)"'
    )
)
def exactly_deduction(prm, n, name):
    still_one_deduction(prm, n, name)


@then(parsers.re(_W + r"the seller has exactly (?P<n>\d+) REFUND_DEDUCTION entries"))
def deduction_entries(prm, n):
    got = r.wait_deductions(prm, _seller(prm), int(n))
    assert len(got) == int(n), got


@then(parsers.re(r'the seller has no REFUND_DEDUCTION referencing the return "(?P<name>\w+)"'))
def no_deduction_for_return(prm, name):
    ref = f"return:{_rid(prm, name)}"
    p.quiet(
        prm,
        lambda: _assert_none([d for d in r.deductions(prm, _seller(prm)) if d["reference"] == ref]),
    )


@then("the seller has no REFUND_DEDUCTION")
def no_deduction(prm):
    _assert_none(r.deductions(prm, _seller(prm)))


def _assert_none(got: list[dict]) -> None:
    assert not got, f"unexpected REFUND_DEDUCTION row(s): {got}"


@then(parsers.re(_W + r"the seller's REFUND_DEDUCTION entries sum to (?P<total>-\d+)"))
def deductions_sum(prm, total):
    deadline = time.monotonic() + p.SETTLE_WAIT_S
    got: list[dict] = []
    while time.monotonic() < deadline:
        got = r.deductions(prm, _seller(prm))
        if sum(d["amount"] for d in got) == int(total):
            return
        time.sleep(0.5)
    raise AssertionError(f"REFUND_DEDUCTION rows sum to {sum(d['amount'] for d in got)}: {got}")


@then(parsers.re(_W + r"the seller's REFUND_DEDUCTION amounts are (?P<a>-\d+) and (?P<b>-\d+)"))
def deduction_amounts(prm, a, b):
    want = sorted([int(a), int(b)])
    deadline = time.monotonic() + p.SETTLE_WAIT_S
    got: list[dict] = []
    while time.monotonic() < deadline:
        got = r.deductions(prm, _seller(prm))
        if sorted(d["amount"] for d in got) == want:
            return
        time.sleep(0.5)
    raise AssertionError(f"REFUND_DEDUCTION amounts {[d['amount'] for d in got]}, want {want}")


@then(parsers.re(_W + r"the seller is credited exactly (?P<n>\d+) times? of (?P<amount>\d+)"))
def credited_exactly(prm, n, amount):
    p.wait_rows(prm, _seller(prm), p.SETTLEMENT, int(amount), int(n))
    p.quiet(
        prm,
        lambda: _count(len(p.rows(prm, _seller(prm), p.SETTLEMENT, int(amount))), int(n)),
    )


def _count(got: int, want: int) -> None:
    assert got == want, f"{got} ORDER_SETTLEMENT row(s), want {want}"


# ── team-payment lifecycle (destructive lane) ────────────────────────────
@any_step("team-payment is stopped")
def stop_payment(prm):
    stack.stop_payment()
    prm.data["payment_stopped"] = True


@any_step("team-payment is started again")
def start_payment(prm):
    stack.start_payment()
    prm.data.pop("payment_stopped", None)
    p.wait_payment_up(prm, _seller(prm))
