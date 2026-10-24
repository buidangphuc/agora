"""Binds engagement/dispute_escalation.feature (change engagement-dispute-escalation, q6-trust-enforcement).

Black box through the gateway with real, distinct users. The DisputeResolved fact is read off
engagement.events, the audit trail with QueryAuditLog as admin. The audit-down scenario is
@destructive: it stops team-audit and a fixture teardown always starts it again.
"""

from __future__ import annotations

import pytest
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.flows.stack_flow import _docker
from tests.e2e.support import dde_support as d
from tests.e2e.support import oic_order_support as o
from tests.e2e.support import prm_rma_support as r
from tests.e2e.support.oic_order_support import OicWorld, code_of, ok

scenarios("engagement/dispute_escalation.feature")


@pytest.fixture
def dde():
    w = OicWorld()
    w.data.update(returns={}, disputes={})
    yield w
    if w.data.get("audit_stopped"):
        _docker("start", d.audit_container())
        d.wait_audit_up(w)


def _did(w: OicWorld, name: str) -> str:
    return w.data["disputes"][name]


@given("a seller and a buyer exist")
def seller_and_buyer(dde: OicWorld) -> None:
    d.parties(dde)


@given("a second buyer who did not place the order exists")
def second_buyer(dde: OicWorld) -> None:
    o.register(dde, "stranger", "buyer")


@given(parsers.parse('the buyer has order "{order}" with an "{state}" return "{ret}"'))
@given(parsers.parse('the buyer has order "{order}" with a "{state}" return "{ret}"'))
def order_with_return(dde: OicWorld, order: str, state: str, ret: str) -> None:
    d.order_with_return(dde, order, state, ret)


@given(parsers.parse('the buyer has a plain order "{order}"'))
def plain_order(dde: OicWorld, order: str) -> None:
    d.plain_order(dde, order)


@when(parsers.parse('the buyer escalates return "{ret}" of order "{order}" as dispute "{name}"'))
def escalate_named(dde: OicWorld, ret: str, order: str, name: str) -> None:
    order_id = dde.orders[order]
    resp = d.create_dispute(
        dde,
        dde.actors["buyer"],
        order_id,
        d.order_seller_id(dde, order_id),
        dde.data["returns"][ret],
    )
    dde.responses["call"] = resp
    if resp.status_code == 200:
        dde.data["disputes"][name] = resp.json()["dispute"]["id"]


@when(parsers.parse('the buyer escalates return "{ret}" of order "{order}"'))
def escalate(dde: OicWorld, ret: str, order: str) -> None:
    order_id = dde.orders[order]
    dde.responses["call"] = d.create_dispute(
        dde,
        dde.actors["buyer"],
        order_id,
        d.order_seller_id(dde, order_id),
        dde.data["returns"][ret],
    )


@when(parsers.parse('the buyer opens a plain dispute "{name}" on order "{order}"'))
def plain_dispute(dde: OicWorld, name: str, order: str) -> None:
    order_id = dde.orders[order]
    resp = d.create_dispute(dde, dde.actors["buyer"], order_id, d.order_seller_id(dde, order_id))
    dde.data["disputes"][name] = ok(resp)["dispute"]["id"]
    dde.responses["call"] = resp


@when(parsers.parse('the admin moves dispute "{name}" to "{status}" with ruling "{ruling}"'))
def admin_moves_with_ruling(dde: OicWorld, name: str, status: str, ruling: str) -> None:
    dde.responses["call"] = d.resolve(
        dde, d.admin(dde), _did(dde, name), f"DISPUTE_STATUS_{status}", f"DISPUTE_RULING_{ruling}"
    )


@when(parsers.parse('the admin moves dispute "{name}" to "{status}"'))
def admin_moves(dde: OicWorld, name: str, status: str) -> None:
    dde.responses["call"] = d.resolve(
        dde, d.admin(dde), _did(dde, name), f"DISPUTE_STATUS_{status}"
    )


@when(parsers.parse('the buyer moves dispute "{name}" to "{status}" with ruling "{ruling}"'))
def buyer_moves(dde: OicWorld, name: str, status: str, ruling: str) -> None:
    dde.responses["call"] = d.resolve(
        dde,
        dde.actors["buyer"],
        _did(dde, name),
        f"DISPUTE_STATUS_{status}",
        f"DISPUTE_RULING_{ruling}",
    )


@when("team-audit is stopped")
def audit_stopped(dde: OicWorld) -> None:
    _docker("stop", d.audit_container())
    dde.data["audit_stopped"] = True


@when("team-audit is started again")
def audit_started(dde: OicWorld) -> None:
    _docker("start", d.audit_container())
    dde.data["audit_stopped"] = False
    d.wait_audit_up(dde)


@when(parsers.parse('the second buyer lists the disputes of order "{order}"'))
def stranger_lists(dde: OicWorld, order: str) -> None:
    dde.responses["call"] = d.list_order_disputes(dde, dde.actors["stranger"], dde.orders[order])


@then("the dispute call succeeds")
def call_succeeds(dde: OicWorld) -> None:
    resp = dde.responses["call"]
    assert resp.status_code == 200, f"expected 200, got {resp.status_code}: {resp.text[:400]}"


@then(parsers.parse('the dispute call is refused with code "{code}"'))
def call_refused(dde: OicWorld, code: str) -> None:
    resp = dde.responses["call"]
    assert resp.status_code != 200, f"expected {code}, got 200: {resp.text[:400]}"
    assert code_of(resp) == code, f"expected {code}, got {resp.status_code}: {resp.text[:400]}"


@then(parsers.parse('the dispute call message is "{message}"'))
def call_message(dde: OicWorld, message: str) -> None:
    resp = dde.responses["call"]
    assert resp.json().get("message") == message, resp.text[:400]


@then("the refused response holds no dispute")
def refused_holds_none(dde: OicWorld) -> None:
    resp = dde.responses["call"]
    assert resp.status_code != 200, resp.text[:400]
    assert not resp.json().get("disputes"), resp.text[:400]


@then(
    parsers.parse(
        'the dispute call succeeds with dispute "{name}" OPEN and linked to return "{ret}"'
    )
)
def created_linked(dde: OicWorld, name: str, ret: str) -> None:
    resp = dde.responses["call"]
    assert resp.status_code == 200, f"expected 200, got {resp.status_code}: {resp.text[:400]}"
    dispute = resp.json()["dispute"]
    assert dispute.get("status") == d.OPEN, dispute
    assert dispute.get("returnId") == dde.data["returns"][ret], dispute
    assert dispute.get("id") == _did(dde, name), dispute


@then(parsers.parse('the admin sees {count:d} disputes on order "{order}"'))
def admin_sees(dde: OicWorld, count: int, order: str) -> None:
    body = ok(d.list_order_disputes(dde, d.admin(dde), dde.orders[order]))
    got = body.get("disputes", [])
    assert len(got) == count, f"order {order} has {len(got)} dispute(s), want {count}: {got}"


@then(parsers.parse('dispute "{name}" reads status "{status}" with ruling "{ruling}"'))
def reads(dde: OicWorld, name: str, status: str, ruling: str) -> None:
    dispute = d.get_dispute(dde, dde.actors["buyer"], _did(dde, name))
    assert dispute.get("status", "DISPUTE_STATUS_UNSPECIFIED") == f"DISPUTE_STATUS_{status}", dispute
    assert dispute.get("ruling", d.RULING_UNSPECIFIED) == f"DISPUTE_RULING_{ruling}", dispute


@then(
    parsers.parse(
        'the buyer reads dispute "{name}" as RESOLVED with ruling "{ruling}" and return "{ret}"'
    )
)
def reads_resolved(dde: OicWorld, name: str, ruling: str, ret: str) -> None:
    dispute = d.get_dispute(dde, dde.actors["buyer"], _did(dde, name))
    assert dispute.get("status") == d.RESOLVED, dispute
    assert dispute.get("ruling") == f"DISPUTE_RULING_{ruling}", dispute
    assert dispute.get("returnId") == dde.data["returns"][ret], dispute


@then(parsers.parse('dispute "{name}" has {count:d} dispute.resolve audit events'))
def audit_count(dde: OicWorld, name: str, count: int) -> None:
    got = d.audit_events(dde, _did(dde, name))
    assert len(got) == count, f"want {count} dispute.resolve event(s), got {got}"


@then(
    parsers.parse(
        'dispute "{name}" has {count:d} dispute.resolve audit events whose last carries '
        'ruling "{ruling}" and return "{ret}"'
    )
)
def audit_last(dde: OicWorld, name: str, count: int, ruling: str, ret: str) -> None:
    got = d.audit_events(dde, _did(dde, name))
    assert len(got) == count, f"want {count} dispute.resolve event(s), got {got}"
    last = max(got, key=lambda e: e.get("createdAt", ""))
    meta = last.get("metadata") or {}
    assert meta.get("ruling") == ruling, last
    assert meta.get("return_id") == dde.data["returns"][ret], last


@then(parsers.parse('dispute "{name}" has {count:d} DisputeResolved facts on engagement.events'))
def fact_count(dde: OicWorld, name: str, count: int) -> None:
    got = d.resolved_facts(_did(dde, name), tail_s=3.0)
    assert len(got) == count, f"want {count} DisputeResolved record(s), got {len(got)}"


@then(
    parsers.parse(
        "within 10 seconds engagement.events holds exactly 1 DisputeResolved envelope keyed by "
        'dispute "{name}" for return "{ret}" with ruling "{ruling}"'
    )
)
def one_fact(dde: OicWorld, name: str, ret: str, ruling: str) -> None:
    got = d.wait_resolved_fact(_did(dde, name), 10.0)
    assert len(got) == 1, f"want exactly 1 DisputeResolved record, got {len(got)}"
    fact = d.decode_resolved(got[0])
    assert fact["type"] == d.RESOLVED_TYPE.decode(), fact
    assert fact["key"] == _did(dde, name), fact
    assert fact["dispute_id"] == _did(dde, name), fact
    assert fact["return_id"] == dde.data["returns"][ret], fact
    assert fact["status"] == d.RESOLVED, fact
    assert fact["ruling"] == f"DISPUTE_RULING_{ruling}", fact
    assert fact["resolver_id"] == d.admin(dde).user_id, fact


def _listed(dde: OicWorld, actor_name: str, order: str) -> list[str]:
    body = ok(d.list_order_disputes(dde, dde.actors[actor_name], dde.orders[order]))
    return [x["id"] for x in body.get("disputes", [])]


@then(parsers.parse('the buyer lists the disputes of order "{order}" and gets "{a}" then "{b}"'))
def buyer_lists_two(dde: OicWorld, order: str, a: str, b: str) -> None:
    assert _listed(dde, "buyer", order) == [_did(dde, a), _did(dde, b)]


@then(parsers.parse('the seller lists the disputes of order "{order}" and gets "{a}" then "{b}"'))
def seller_lists_two(dde: OicWorld, order: str, a: str, b: str) -> None:
    assert _listed(dde, "seller", order) == [_did(dde, a), _did(dde, b)]


@then(parsers.parse('the buyer lists the disputes of order "{order}" and gets "{a}"'))
def buyer_lists_one(dde: OicWorld, order: str, a: str) -> None:
    assert _listed(dde, "buyer", order) == [_did(dde, a)]


@then(
    parsers.parse(
        'the admin lists OPEN disputes oldest first and finds "{a}", "{b}", "{c}" in that order '
        'and not "{gone}"'
    )
)
def admin_lists_open(dde: OicWorld, a: str, b: str, c: str, gone: str) -> None:
    got = d.list_all_open(dde, d.admin(dde), d.OPEN)
    ids = [x["id"] for x in got]
    assert all(x.get("status") == d.OPEN for x in got), "ListDisputes returned a non-OPEN dispute"
    want = [_did(dde, n) for n in (a, b, c)]
    assert _did(dde, gone) not in ids, f"closed dispute {gone} is listed as OPEN"
    missing = [i for i in want if i not in ids]
    assert not missing, f"missing from the OPEN list: {missing}"
    assert [i for i in ids if i in want] == want, "OPEN disputes are not oldest first"
    stamps = [x.get("createdAt", "") for x in got]
    assert stamps == sorted(stamps), "createdAt is not ascending"


@then(parsers.parse('return "{ret}" still reads REJECTED'))
def return_still_rejected(dde: OicWorld, ret: str) -> None:
    got = r.return_status(dde, dde.actors["buyer"], dde.data["returns"][ret])
    assert got == r.REJECTED, got
