"""Steps for payment-settlement-commission (initiative monetization).

Black box through the gateway: orders are placed with real shop and platform vouchers and paid
online, the seller's ledger is read with `ListLedgerEntries`, the breakdown with
`GetOrderSettlement`, the store constraints through psql into payment_db, replays through the
real `order.events` topic. Built on the multishop checkout steps (vouchers, cart, placing) and the
payment-ledger steps (`plp_steps`): one world serves both, `plp` is an alias of `msw`.
Amounts the spec states for a 30000 shipping fee follow the stack's quoted fee (35000) here: the
commission base excludes shipping, so commissions match the spec; payments and credits are
compared with the order's own total.
"""

from __future__ import annotations

import uuid

import httpx
import pytest
from pytest_bdd import given, parsers, then

from tests.e2e.step_definitions.order_multishop_checkout_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.order_multishop_checkout_steps import _placed
from tests.e2e.step_definitions.order_multishop_checkout_steps import place as _place_when
from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.plp_steps import (
    _insert,
    _last,
    _paid,
    _record_paid,
    _seller,
    any_step,
)
from tests.e2e.step_definitions.prm_pay_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_fee_charge_steps import *  # noqa: F401,F403
from tests.e2e.support import multishop_support as ms
from tests.e2e.support import oic_order_support as o
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import plp_support as p

COMMISSION = "COMMISSION"
REVERSAL = "COMMISSION_REVERSAL"
RATE_BPS = 500


@pytest.fixture
def plp(msw):
    msw.data.update(paid=[], credited=[], inserted=[])
    yield msw
    for row_id in msw.data.get("inserted", []):
        stack.psql(f"DELETE FROM wallet_ledger WHERE id = {stack.sql_lit(row_id)}", check=False)


def _commission(base: int) -> int:
    return (base * RATE_BPS + 5000) // 10000


def _facts(w: o.OicWorld) -> dict:
    return w.data["psc"]


def _principal(w: o.OicWorld, who: str) -> o.Actor | None:
    if who == "the seller":
        return _seller(w)
    if who == "another seller":
        return w.actors.get("other-seller") or o.register(w, "other-seller", "seller")
    if who == "an admin":
        return w.actors.get("admin") or o.login_admin(w)
    if who == "an anonymous caller":
        return None
    return w.actors[who.strip('"')]


def _get_settlement(w: o.OicWorld, who: str, order_id: str) -> httpx.Response:
    return o.post(w, _principal(w, who), o.PAYMENT, "GetOrderSettlement", {"orderId": order_id})


def _fields(resp: httpx.Response) -> dict[str, int]:
    body = o.ok(resp)
    return {
        k: int(body.get(k, 0) or 0)
        for k in (
            "credited",
            "commission",
            "commissionReversed",
            "refundDeducted",
            "net",
            "commissionRateBps",
        )
    }


@given("the call succeeds")
def call_succeeds_given(plp):
    resp = _last(plp)
    assert resp.status_code == 200, f"expected success, got {p.describe(resp)}"


@given(parsers.parse('"{label}" places the order with the same selection'))
def places_given(msw, label):
    _place_when(msw, label)


@any_step(parsers.parse('the seller of shop "{shop}" is the settling seller'))
def settling_seller(plp, shop):
    plp.actors["seller"] = plp.actors[f"shop-{shop}"]
    plp.data["balance0"] = p.balance(plp, _seller(plp))


@any_step(parsers.parse('"{label}" pays the first placed order online and the seller is credited'))
def pay_first_credited(plp, label):
    buyer = plp.actors[label]
    order_id = _placed(plp, label)[0]["id"]
    tx_id = p.pay(plp, buyer, order_id)
    o.wait_status(plp, buyer, order_id, o.PAID)
    order = o.get_order(plp, buyer, order_id)
    total = ms.n(order.get("totalAmount"))
    credit = total + ms.n(order.get("platformDiscountAmount"))
    plp.data["psc"] = {"order": order_id, "total": total, "credit": credit, "buyer": label}
    _record_paid(plp, "shop-A", total, order_id, tx_id)
    plp.data["last_rows"] = p.wait_rows(plp, _seller(plp), p.SETTLEMENT, credit, 1)
    plp.data["credited"].append(credit)


@any_step(parsers.parse('"{label}" places an order without paying it'))
def place_unpaid(plp, label):
    resp = ms.place(plp, plp.actors[label], {"shop_vouchers": [], "platform": ""})
    orders = o.ok(resp).get("orders", [])
    assert orders, resp.text[:300]
    plp.data.setdefault("ms", {"req": {}, "preview": {}, "placed": {}, "vouchers": {}})["placed"][
        label
    ] = orders
    plp.orders["order"] = orders[0]["id"]


@any_step(parsers.parse('"{label}" cancels the first placed order before paying'))
def cancel_unpaid(plp, label):
    o.ok(o.post(plp, plp.actors[label], o.ORDER, "CancelOrder", {"id": plp.orders["order"]}))


@then("the seller's ORDER_SETTLEMENT entry is the order's total amount plus its platform discount")
def settlement_entry_is_credit(plp):
    credit = _facts(plp)["credit"]
    got = p.rows(plp, _seller(plp), p.SETTLEMENT, credit)
    assert len(got) == 1, f"want one ORDER_SETTLEMENT of {credit}: {p.ledger(plp, _seller(plp))}"


@then(
    parsers.parse("the seller's ORDER_SETTLEMENT entry is {extra:d} above the order's total amount")
)
def settlement_entry_above_total(plp, extra):
    facts = _facts(plp)
    assert facts["credit"] == facts["total"] + extra, facts
    got = p.rows(plp, _seller(plp), p.SETTLEMENT, facts["total"] + extra)
    assert len(got) == 1, f"want one ORDER_SETTLEMENT of {facts['total'] + extra}: {got}"


@then(parsers.parse("the seller's balance is the order's credit minus {amount:d}"))
def balance_is_credit_minus(plp, amount):
    want = _facts(plp)["credit"] - amount
    got = p.balance(plp, _seller(plp)) - plp.data["balance0"]
    assert got == want, f"balance grew by {got}, want {want}: {p.ledger(plp, _seller(plp))}"


@then(parsers.parse("the seller's COMMISSION_REVERSAL entries are {first:d} and {second:d}"))
def reversals_are(plp, first, second):
    got = sorted(int(e["amount"]) for e in p.rows(plp, _seller(plp), REVERSAL))
    assert got == sorted([first, second]), f"reversals {got}: {p.ledger(plp, _seller(plp))}"


@then("the seller's ledger has no COMMISSION_REVERSAL entry")
def no_reversal(plp):
    assert not p.rows(plp, _seller(plp), REVERSAL), p.ledger(plp, _seller(plp))


@then("the seller's ledger has no COMMISSION entry")
def no_commission(plp):
    assert not p.rows(plp, _seller(plp), COMMISSION), p.ledger(plp, _seller(plp))


@then("the seller's ledger has no REFUND_DEDUCTION or COMMISSION_REVERSAL entry")
def no_deduction_or_reversal(plp):
    assert not p.rows(plp, _seller(plp), p.DEDUCTION), p.ledger(plp, _seller(plp))
    assert not p.rows(plp, _seller(plp), REVERSAL), p.ledger(plp, _seller(plp))


@then("the seller's balance equals the credits minus the commissions of the order and the sentinel")
def balance_after_replay(plp):
    facts = _facts(plp)
    sentinel = p.SENTINEL_PRICE + plp.data["sentinels"]
    want = facts["credit"] - 14000 + sentinel - _commission(sentinel)
    p.wait_balance(plp, _seller(plp), want)


@then(
    parsers.parse("exactly {n:d} COMMISSION entry of -{amount:d} references the credited payment")
)
def one_commission_references_payment(plp, n, amount):
    rows = p.wait_rows(plp, _seller(plp), COMMISSION, -amount, n)
    assert len(rows) == n, rows
    got = stack.psql(
        f"SELECT reference_id FROM wallet_ledger WHERE id = {stack.sql_lit(rows[-1]['id'])}",
        check=False,
    )
    assert got.stdout.strip() == _paid(plp)["tx"], got.stderr.strip()[:300] or got.stdout


# ── GetOrderSettlement ───────────────────────────────────────────────────
@any_step(
    parsers.re(
        r'(?P<who>the seller|another seller|an admin|an anonymous caller|"[^"]+") '
        r"calls GetOrderSettlement for the order"
    )
)
def calls_get_settlement(plp, who):
    resp = _get_settlement(plp, who, plp.orders["order"])
    plp.responses["last"] = resp
    plp.data.setdefault("settlements", {})[who] = resp


@any_step("the seller calls GetOrderSettlement with an empty order id")
def calls_with_empty_id(plp):
    plp.responses["last"] = _get_settlement(plp, "the seller", "")


@any_step(parsers.parse("the seller calls GetOrderSettlement for the order {n:d} times"))
def calls_n_times(plp, n):
    seller = _seller(plp)
    plp.data["before_reads"] = (p.ledger(plp, seller), p.balance(plp, seller))
    for _ in range(n):
        resp = _get_settlement(plp, "the seller", plp.orders["order"])
        assert resp.status_code == 200, f"GetOrderSettlement: {p.describe(resp)}"
        plp.responses["last"] = resp


@then("the ledger entries and the balance are unchanged by the reads")
def reads_wrote_nothing(plp):
    seller = _seller(plp)
    ids = lambda rows: sorted(e["id"] for e in rows)  # noqa: E731
    before, bal = plp.data["before_reads"]
    assert ids(p.ledger(plp, seller)) == ids(before)
    assert p.balance(plp, seller) == bal


@then(
    parsers.parse(
        "the settlement reports commission {commission:d}, commission reversed {reversed_:d}, "
        "refund deducted {deducted:d} and rate {rate:d} bps"
    )
)
def settlement_reports(plp, commission, reversed_, deducted, rate):
    got = _fields(_last(plp))
    want = {
        "commission": commission,
        "commissionReversed": reversed_,
        "refundDeducted": deducted,
        "commissionRateBps": rate,
    }
    assert {k: got[k] for k in want} == want, got


@then("the settlement credited amount is the order's total amount plus its platform discount")
def settlement_credited(plp):
    assert _fields(_last(plp))["credited"] == _facts(plp)["credit"]


@then("the settlement net equals the seller's balance growth")
def settlement_net_is_balance(plp):
    net = _fields(_last(plp))["net"]
    grew = p.balance(plp, _seller(plp)) - plp.data["balance0"]
    assert net == grew, f"net {net}, balance grew by {grew}: {p.ledger(plp, _seller(plp))}"


@then(parsers.parse("the settlement net is {amount:d}"))
def settlement_net_is(plp, amount):
    assert _fields(_last(plp))["net"] == amount


@then("both settlements are equal")
def settlements_equal(plp):
    seen = plp.data["settlements"]
    assert _fields(seen["the seller"]) == _fields(seen["an admin"])


@then(parsers.parse("the settlement read again by the seller after the replay has net {amount:d}"))
def settlement_after_replay(plp, amount):
    resp = _get_settlement(plp, "the seller", plp.orders["order"])
    assert _fields(resp)["net"] == amount, p.describe(resp)


# ── refunds ──────────────────────────────────────────────────────────────
@any_step(parsers.parse("another seller refunds {amount:d} of the payment"))
def other_seller_refunds(plp, amount):
    other = _principal(plp, "another seller")
    plp.responses["last"] = p.refund(plp, other, _paid(plp)["tx"], amount)


# ── held net of commission ───────────────────────────────────────────────
@then(parsers.parse("the payout is accepted for {amount:d}"))
def payout_accepted_for(plp, amount):
    resp = _last(plp)
    assert resp.status_code == 200, f"expected the payout, got {p.describe(resp)}"
    assert int((resp.json().get("entry") or {}).get("amount", 0)) == -amount, resp.text[:300]


# ── store constraints ────────────────────────────────────────────────────
@any_step(
    "a COMMISSION row of -1 with a fresh reference is accepted directly by team-payment's database"
)
def accepts_valid_commission(plp):
    _insert(plp, COMMISSION, -1, f"psc-fresh-{uuid.uuid4()}")
    proc = plp.data["insert"]
    assert proc.returncode == 0, f"a valid COMMISSION row is refused: {proc.stderr.strip()[:300]}"


@any_step(
    "a COMMISSION_REVERSAL row of 1 with a fresh reference is accepted directly by "
    "team-payment's database"
)
def accepts_valid_reversal(plp):
    _insert(plp, REVERSAL, 1, f"psc-fresh-{uuid.uuid4()}")
    proc = plp.data["insert"]
    assert proc.returncode == 0, f"a valid reversal is refused: {proc.stderr.strip()[:300]}"


@any_step(
    "a second COMMISSION row with the reference of the credited payment is inserted directly "
    "into team-payment's database"
)
def insert_second_commission(plp):
    _insert(plp, COMMISSION, -1, _paid(plp)["tx"])


@any_step(
    "a COMMISSION row of 1 with a fresh reference is inserted directly into team-payment's database"
)
def insert_positive_commission(plp):
    _insert(plp, COMMISSION, 1, f"psc-fresh-{uuid.uuid4()}")


@any_step(
    "a COMMISSION_REVERSAL row of -1 with a fresh reference is inserted directly into "
    "team-payment's database"
)
def insert_negative_reversal(plp):
    _insert(plp, REVERSAL, -1, f"psc-fresh-{uuid.uuid4()}")


@any_step(
    "a COMMISSION row of -1 without a reference is inserted directly into team-payment's database"
)
def insert_unreferenced_commission(plp):
    _insert(plp, COMMISSION, -1, None)
