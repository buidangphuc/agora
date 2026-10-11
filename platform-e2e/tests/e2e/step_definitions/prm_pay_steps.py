# ruff: noqa: F811
"""Steps for payment-refund-model (area prm-pay): payment-cumulative-refunds and the modified
seller-refund-deduction / seller-payout-holdback scenarios.

Written from the spec ahead of the code. Refunds go through the gateway as the order's seller with
explicit refund ids (symbolic names like "R1" map to unique real ids). Concurrency scenarios release
their calls together through `oic_order_support.race`. The migration scenarios use a scratch
database (`prm_pay_stack`) and a throwaway team-payment: they are @destructive (serial lane).
The generic plp steps (seller/buyer/paid orders/cancel/sentinel/ledger counts) are reused from
`plp_steps`.
"""

from __future__ import annotations

import re
import uuid

import pytest
from pytest_bdd import parsers

from tests.e2e.step_definitions.plp_steps import (  # noqa: F401  (plp fixture is shared)
    _actor,
    _last,
    _paid,
    _seller,
    any_step,
    plp,
)
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_pay_stack as scratch_stack
from tests.e2e.support import prm_pay_support as r
from tests.e2e.support.oic_order_support import OicWorld, ok, race
from tests.e2e.support.oic_order_support import register as register_actor

_NUMBERS = {"no": 0, "one": 1, "two": 2, "three": 3, "four": 4}


def _n(text: str) -> int:
    return _NUMBERS[text] if text in _NUMBERS else int(text)


def _tx(w: OicWorld, buyer_name: str = "b1") -> dict:
    return p.payment_of(w, _actor(w, buyer_name), w.orders["order"])


# ── refunds through the gateway, with refund ids ─────────────────────────
@any_step(parsers.parse("the seller refunds {amount:d} of the payment with a fresh refund id"))
def refund_fresh(plp, amount):
    plp.responses["last"] = p.refund(plp, _seller(plp), _paid(plp)["tx"], amount)
    plp.data["last_call"] = (amount, "e2e", plp.data["last_refund_id"])


@any_step(
    parsers.re(
        r'the seller refunds (?P<amount>\d+) of the payment with refund id "(?P<name>[^"]+)"'
        r'(?: and reason "(?P<reason>[^"]+)")?$'
    )
)
def refund_named(plp, amount, name, reason):
    amount, reason = int(amount), reason or "e2e"
    plp.responses["last"] = r.refund_named(
        plp, _seller(plp), _paid(plp)["tx"], amount, name, reason
    )
    plp.data["last_call"] = (amount, reason, r.refund_id_named(plp, name))


@any_step("the seller sends the same refund call again")
def refund_again(plp):
    amount, reason, rid = plp.data["last_call"]
    plp.responses["last"] = p.refund(plp, _seller(plp), _paid(plp)["tx"], amount, reason, rid)


@any_step(parsers.parse("the seller refunds {amount:d} of the payment without a refund id"))
def refund_no_id(plp, amount):
    plp.responses["last"] = r.refund_without_id(plp, _seller(plp), _paid(plp)["tx"], amount)


@any_step(
    parsers.parse(
        "the seller sends {n:d} concurrent refunds of {amount:d} for the payment, "
        "each with its own refund id"
    )
)
def concurrent_own_ids(plp, n, amount):
    seller, tx_id = _seller(plp), _paid(plp)["tx"]
    plp.data["race"] = race([lambda: p.refund(plp, seller, tx_id, amount) for _ in range(n)])


@any_step(
    parsers.parse(
        "the seller sends {n:d} concurrent refunds of {amount:d} for the payment, "
        "all with the same refund id"
    )
)
def concurrent_same_id(plp, n, amount):
    seller, tx_id, rid = _seller(plp), _paid(plp)["tx"], p.new_refund_id("same")
    plp.data["race"] = race(
        [lambda: p.refund(plp, seller, tx_id, amount, "e2e", rid) for _ in range(n)]
    )


@any_step("every call succeeds")
def every_call_succeeds(plp):
    results = plp.data["race"]
    bad = [p.describe(x) for x in results if x.status_code != 200]
    assert not bad, f"{len(bad)} of {len(results)} calls failed: {bad}"


# ── what the payment reports ─────────────────────────────────────────────
@any_step(
    parsers.re(
        r'(?:within the settle window )?the payment read by "(?P<b>[^"]+)" reads (?P<st>[A-Z_]+) '
        r"with a refunded amount of (?P<amt>\d+)(?: and (?P<n>\d+|no) refunds?)?$"
    )
)
def payment_reads(plp, b, st, amt, n):
    tx = r.wait_payment_state(
        plp, _actor(plp, b), plp.orders["order"], r.status_name(st), int(amt), _n(n) if n else None
    )
    plp.data["tx_read"] = tx


@any_step(
    parsers.re(
        r'the payment read by "(?P<b>[^"]+)" lists the refunds '
        r"(?P<spec>\d+ [A-Z_]+(?:, \d+ [A-Z_]+)*), oldest first$"
    )
)
def payment_lists_refunds(plp, b, spec):
    want = [(int(a), r.source_name(src)) for a, src in (x.split() for x in spec.split(", "))]
    tx = _tx(plp, b)
    got = [(r.refund_amount(x), x.get("source")) for x in r.refunds_of(tx)]
    assert got == want, f"refunds {got}, want {want}: {r.describe_tx(tx)}"


@any_step(
    parsers.re(
        r'the payment read by "(?P<b>[^"]+)" lists (?P<n>\d+) refunds with source (?P<src>[A-Z_]+), '
        r"source ids (?P<ids>\"[^\"]+\"(?: and \"[^\"]+\")*) and requested and applied amounts of "
        r"(?P<amts>\d+(?: and \d+)*)$"
    )
)
def payment_lists_detail(plp, b, n, src, ids, amts):
    names = re.findall(r'"([^"]+)"', ids)
    amounts = [int(a) for a in amts.split(" and ")]
    assert len(names) == len(amounts) == int(n), (names, amounts, n)
    tx = _tx(plp, b)
    got = r.refunds_of(tx)
    assert len(got) == int(n), f"want {n} refunds: {r.describe_tx(tx)}"
    for ref, name, amount in zip(got, names, amounts, strict=True):
        assert ref.get("source") == r.source_name(src), f"source of {ref}"
        assert ref.get("sourceId") == r.refund_id_named(plp, name), f"source id of {ref}"
        assert int(ref.get("requestedAmount", 0)) == amount, f"requested amount of {ref}"
        assert r.refund_amount(ref) == amount, f"applied amount of {ref}"
        assert ref.get("id"), f"refund without an id: {ref}"
        assert ref.get("createdAt"), f"refund without a creation time: {ref}"


@any_step(parsers.parse('the first refund of the payment read by "{b}" has reason "{reason}"'))
def first_refund_reason(plp, b, reason):
    refunds = r.refunds_of(_tx(plp, b))
    assert refunds and refunds[0].get("reason") == reason, f"first refund: {refunds[:1]}"


# ── ledger references ────────────────────────────────────────────────────
@any_step(parsers.re(r"the seller's REFUND_DEDUCTION entries are (?P<amts>-?\d+(?: and -?\d+)*)$"))
def deductions_are(plp, amts):
    wanted = [int(x) for x in re.findall(r"-?\d+", amts)]
    plp.data["deductions"] = r.wait_deduction_amounts(plp, _seller(plp), wanted)
    p.quiet(plp, lambda: r.wait_deduction_amounts(plp, _seller(plp), wanted, timeout=0))


@any_step(
    parsers.re(
        r"the seller's REFUND_DEDUCTION entries are (?P<n>\d+) of (?P<amt>-?\d+), "
        r"each referencing one of the payment's refunds$"
    )
)
def n_deductions_each_referencing(plp, n, amt):
    r.wait_deduction_amounts(plp, _seller(plp), [int(amt)] * int(n))
    references_refunds(plp)


@any_step("each REFUND_DEDUCTION entry references the refund it deducts")
def references_refunds(plp):
    deds = r.deductions(plp, _seller(plp))
    r.check_references(deds, _tx(plp))


@any_step("the REFUND_DEDUCTION entry references the cancel refund of the order")
def deduction_references_cancel(plp):
    deds = r.deductions(plp, _seller(plp))
    want = f"cancel:{plp.orders['order']}"
    got = [r.reference_of(e) for e in deds]
    assert got == [want], f"deduction references {got}, want {[want]}"


@any_step("the seller's ledger shows the ORDER_SETTLEMENT referencing the payment id")
def settlement_references_payment(plp):
    credits = p.rows(plp, _seller(plp), p.SETTLEMENT)
    tx_id = _paid(plp)["tx"]
    got = [r.reference_of(e) for e in credits]
    assert tx_id in got, f"ORDER_SETTLEMENT references {got}, want the payment id {tx_id}"


@any_step("the seller's balance is back to its value before the payment")
def balance_back(plp):
    p.wait_balance(plp, _seller(plp), plp.data["balance0"])


# ── who can read the payment ─────────────────────────────────────────────
@any_step("the seller reads the payment of the order through the gateway")
def seller_reads_payment(plp):
    plp.responses["last"] = r.get_payment(plp, _seller(plp), plp.orders["order"])


@any_step("another seller reads the payment of the order through the gateway")
def other_seller_reads_payment(plp):
    other = register_actor(plp, "other_seller", "seller")
    plp.responses["last"] = r.get_payment(plp, other, plp.orders["order"])


@any_step(
    parsers.parse("the call returns the order's payment with {amount:d} refunded and {n:d} refund")
)
def call_returns_payment(plp, amount, n):
    tx = ok(_last(plp)).get("transaction", {})
    assert tx.get("orderId") == plp.orders["order"], f"not the order's payment: {tx}"
    assert tx.get("status") == r.status_name("PARTIALLY_REFUNDED"), r.describe_tx(tx)
    assert r.refunded_amount(tx) == amount and len(r.refunds_of(tx)) == n, r.describe_tx(tx)


# ── migration scenarios: scratch database and throwaway team-payment ─────
@pytest.fixture
def scratch():
    db = scratch_stack.ScratchDb()
    yield db
    db.drop()


def _tag(plp) -> str:
    return plp.data.setdefault("scratch_tag", uuid.uuid4().hex[:8])


@any_step("a scratch payment database at the previous schema")
def scratch_previous(plp, scratch):
    scratch.create()
    scratch.migrate("goto", "6")
    assert scratch.version() == "6", scratch.version()


@any_step(
    parsers.parse(
        "it holds a payment of {amount:d} refunded under the old model: status REFUNDED, refunded "
        "amount {refunded:d}, a credit of +{amount2:d} and a REFUND_DEDUCTION of -{ded:d} "
        "referencing the payment"
    )
)
def scratch_legacy_payment(plp, scratch, amount, refunded, amount2, ded):
    tag = _tag(plp)
    pid, seller = f"prm-legacy-{tag}", f"prm-seller-{tag}"
    plp.data["legacy"] = {"payment": pid, "seller": seller, "amount": amount}
    scratch.psql(
        "INSERT INTO payment_transactions (id, order_id, buyer_id, amount, status, refunded_amount) "
        f"VALUES ('{pid}', 'ord-{tag}', 'buyer-{tag}', {amount}, 4, {refunded});"
        "INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES "
        f"('cr-{tag}', '{seller}', 'ORDER_SETTLEMENT', {amount2}, 'COMPLETED', '{pid}'),"
        f"('rd-{tag}', '{seller}', 'REFUND_DEDUCTION', -{ded}, 'COMPLETED', '{pid}');"
    )


@any_step("it also holds a REFUNDED payment with a refunded amount of 0 and no deduction")
def scratch_zero_refunded(plp, scratch):
    tag = _tag(plp)
    pid = f"prm-zero-{tag}"
    plp.data["zero"] = pid
    scratch.psql(
        "INSERT INTO payment_transactions (id, order_id, buyer_id, amount, status, refunded_amount) "
        f"VALUES ('{pid}', 'ord-zero-{tag}', 'buyer-{tag}', 300000, 4, 0)"
    )


def _ledger_snapshot(scratch, with_reference: bool) -> list[list[str]]:
    cols = "id, seller_id, type, amount, status, created_at" + (
        ", reference_id" if with_reference else ""
    )
    return scratch.rows(f"SELECT {cols} FROM wallet_ledger ORDER BY id")


@any_step("the migration is applied with the team-payment migrate image")
def scratch_migrate_up(plp, scratch):
    plp.data["ledger_before"] = _ledger_snapshot(scratch, with_reference=False)
    scratch.migrate("goto", "7")
    assert scratch.version() == "7", scratch.version()


@any_step("the down migration is applied")
def scratch_migrate_down(plp, scratch):
    scratch.migrate("down", "1")
    assert scratch.version() == "6", scratch.version()


@any_step(
    parsers.parse(
        "the team-payment image runs against the scratch database and an admin refunds {amount:d} "
        "of the legacy payment with a fresh refund id"
    )
)
def scratch_admin_refund(plp, scratch, amount):
    scratch.start_service()
    plp.data["scratch_call"] = scratch.grpc_refund(
        plp.data["legacy"]["payment"], amount, p.new_refund_id("legacy")
    )


@any_step(parsers.parse('the scratch call fails with "{code}"'))
def scratch_call_fails(plp, code):
    got, body = plp.data["scratch_call"]
    assert got == code, f"expected {code}, got {got}: {body}"


@any_step(parsers.parse("the database holds {n:d} refund of the legacy payment"))
def scratch_refund_count(plp, scratch, n):
    got = scratch.scalar(
        f"SELECT count(*) FROM payment_refunds WHERE payment_id = '{plp.data['legacy']['payment']}'"
    )
    assert int(got) == n, f"payment has {got} refund(s), want {n}"


@any_step(
    parsers.parse("the database holds one LEGACY refund of {amount:d} for the legacy payment")
)
def scratch_legacy_refund(plp, scratch, amount):
    pid = plp.data["legacy"]["payment"]
    rows = scratch.rows(
        "SELECT id, source, source_id, requested_amount, amount FROM payment_refunds "
        f"WHERE payment_id = '{pid}'"
    )
    want = [f"legacy:{pid}", "LEGACY", pid, str(amount), str(amount)]
    assert rows == [want], f"payment_refunds rows {rows}, want {[want]}"


@any_step("the legacy deduction references the legacy refund")
def scratch_deduction_legacy_ref(plp, scratch):
    pid = plp.data["legacy"]["payment"]
    got = scratch.scalar("SELECT reference_id FROM wallet_ledger WHERE type = 'REFUND_DEDUCTION'")
    assert got == f"legacy:{pid}", f"deduction references {got!r}, want legacy:{pid}"


@any_step("the legacy deduction references the payment id again")
def scratch_deduction_payment_ref(plp, scratch):
    pid = plp.data["legacy"]["payment"]
    got = scratch.scalar("SELECT reference_id FROM wallet_ledger WHERE type = 'REFUND_DEDUCTION'")
    assert got == pid, f"deduction references {got!r}, want the payment id {pid}"


@any_step(
    parsers.parse(
        "the legacy payment is still REFUNDED with a refunded amount of {amount:d}, and the "
        "seller's ledger sum is still {total:d}"
    )
)
def scratch_legacy_unchanged(plp, scratch, amount, total):
    pid = plp.data["legacy"]["payment"]
    assert scratch.rows(
        f"SELECT status, refunded_amount FROM payment_transactions WHERE id = '{pid}'"
    ) == [["4", str(amount)]]
    _ledger_sum_is(plp, scratch, total)


@any_step(parsers.parse("the seller's ledger sum is still {total:d}"))
def scratch_ledger_sum(plp, scratch, total):
    _ledger_sum_is(plp, scratch, total)


def _ledger_sum_is(plp, scratch, total: int) -> None:
    got = scratch.scalar(
        f"SELECT COALESCE(SUM(amount), 0) FROM wallet_ledger WHERE seller_id = '{plp.data['legacy']['seller']}'"
    )
    assert int(got) == total, f"seller's ledger sum {got}, want {total}"


@any_step(
    "the zero-refunded payment has no refund, keeps status REFUNDED and a refunded amount of 0"
)
def scratch_zero_untouched(plp, scratch):
    pid = plp.data["zero"]
    assert scratch.scalar(f"SELECT count(*) FROM payment_refunds WHERE payment_id = '{pid}'") == "0"
    assert scratch.rows(
        f"SELECT status, refunded_amount FROM payment_transactions WHERE id = '{pid}'"
    ) == [["4", "0"]]


@any_step("no ledger row changed apart from the legacy deduction's reference")
def scratch_ledger_rows_unchanged(plp, scratch):
    after = _ledger_snapshot(scratch, with_reference=False)
    assert (
        after == plp.data["ledger_before"]
    ), f"ledger rows changed: {after} != {plp.data['ledger_before']}"
    refs = scratch.rows("SELECT type, reference_id FROM wallet_ledger ORDER BY type")
    pid = plp.data["legacy"]["payment"]
    assert refs == [["ORDER_SETTLEMENT", pid], ["REFUND_DEDUCTION", f"legacy:{pid}"]], refs
