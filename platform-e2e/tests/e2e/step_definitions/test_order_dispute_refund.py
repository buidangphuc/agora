"""Binds order/dispute_refund.feature (change order-dispute-refund, q6-trust-enforcement).

Black box through the gateway with real users. The audit-down scenario is @destructive: it stops
team-audit and a fixture teardown always starts it again.
"""

from __future__ import annotations

import pytest
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.flows.stack_flow import _docker
from tests.e2e.support import dde_support as d
from tests.e2e.support import odr_support as x
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_rma_support as r
from tests.e2e.support.oic_order_support import OicWorld

scenarios("order/dispute_refund.feature")


@pytest.fixture
def odr():
    w = OicWorld()
    yield w
    if w.data.get("audit_stopped"):
        _docker("start", d.audit_container())
        d.wait_audit_up(w)


@given("a seller and a buyer with a Paid-online order whose return was rejected and escalated")
def setup(odr: OicWorld) -> None:
    x.rejected_return_dispute(odr)


@when(parsers.parse("the admin rules the dispute {ruling}"))
def admin_rules(odr: OicWorld, ruling: str) -> None:
    x.rule(odr, ruling)


@when("team-audit is stopped")
def audit_stopped(odr: OicWorld) -> None:
    _docker("stop", d.audit_container())
    odr.data["audit_stopped"] = True


@when("team-audit is started again")
def audit_started(odr: OicWorld) -> None:
    _docker("start", d.audit_container())
    odr.data["audit_stopped"] = False
    d.wait_audit_up(odr)


@when("a buyer-favour DisputeResolved fact for the dispute is produced to engagement.events")
def produce_fact(odr: OicWorld) -> None:
    x.produce_resolved_fact(odr)


@then(parsers.parse("within {secs:d} seconds the buyer reads the return as REFUNDED"))
def refunded(odr: OicWorld, secs: int) -> None:
    x.wait_return_state(odr, r.REFUNDED, secs)


@then(parsers.parse("for {secs:d} seconds the return stays REJECTED"))
def stays_rejected(odr: OicWorld, secs: int) -> None:
    x.stays_return_state(odr, r.REJECTED, secs)


@then("the payment of the order shows one return refund of the return amount")
def one_return_refund(odr: OicWorld) -> None:
    tx = r.wait_refunded(
        odr, odr.actors["buyer"], odr.orders["order"], x.RETURN_AMOUNT, r.PAY_PARTIAL
    )
    mine = [f for f in r.refunds_of(tx, r.SRC_RETURN) if f.get("sourceId") == odr.data["return_id"]]
    assert len(mine) == 1, f"want one RETURN refund of the return, got {tx.get('refunds')}"
    assert r.requested(mine[0]) == x.RETURN_AMOUNT or r.applied(mine[0]) == x.RETURN_AMOUNT, mine


@then("the payment of the order shows no refund")
def no_refund(odr: OicWorld) -> None:
    tx = p.payment_of(odr, odr.actors["buyer"], odr.orders["order"])
    assert r.refunded_of(tx) == 0 and not r.refunds_of(tx), f"unexpected refund: {tx}"


@then("exactly one return.dispute_refund audit event names the admin for the return")
def one_audit(odr: OicWorld) -> None:
    x.assert_one_dispute_refund_event(odr)


@then("the audit log holds no return.dispute_refund event for the return")
def no_audit(odr: OicWorld) -> None:
    assert not x.dispute_audit_events(odr), "a return.dispute_refund event exists"


@then(
    parsers.parse(
        "the dispute reads RESOLVED with ruling {ruling} and its DisputeResolved fact is on engagement.events"
    )
)
def resolved_control(odr: OicWorld, ruling: str) -> None:
    dispute = d.get_dispute(odr, odr.actors["buyer"], odr.data["dispute_id"])
    assert dispute.get("status") == d.RESOLVED, dispute
    assert dispute.get("ruling") == f"DISPUTE_RULING_{ruling}", dispute
    facts = d.wait_resolved_fact(odr.data["dispute_id"])
    assert len(facts) == 1, f"want one DisputeResolved fact, got {len(facts)}"
    assert d.decode_resolved(facts[0]).get("ruling") == f"DISPUTE_RULING_{ruling}"


@then("the engagement.events.order.dlq topic holds no record for the dispute")
def dlq_empty(odr: OicWorld) -> None:
    got = x.dlq_records(odr)
    assert not got, f"dispute {odr.data['dispute_id']} was dead-lettered: {got}"
