"""Flows for order-dispute-refund (initiative q6-trust-enforcement).

Black box through the gateway with real, distinct users: an order paid online, a return the
seller rejected, a dispute the buyer escalates and the admin rules on. The refund is read off
GetReturnRequest and GetPayment, the audit trail with QueryAuditLog. Reuses the plp, prm_rma and
dde helpers (import only).
"""

from __future__ import annotations

import time
import uuid

from tests.e2e.support import audit_support as aud
from tests.e2e.support import dde_support as d
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_rma_support as r
from tests.e2e.support.oic_order_support import OicWorld, ok

RETURN_AMOUNT = 200_000
ORDER_AMOUNT = 500_000
DLQ_TOPIC = "engagement.events.order.dlq"
AUDIT_ACTION = "return.dispute_refund"
RESOLVED_STATUS_NUM = 3
BUYER_FAVOUR_NUM = 1


def rejected_return_dispute(w: OicWorld) -> None:
    """Seller, buyer, a Paid-online order, its rejected return and an INVESTIGATING dispute."""
    p.seller_with_listing(w, ORDER_AMOUNT, "L")
    buyer = p.buyer(w, "buyer")
    seller = w.actors["seller"]
    order_id, _ = p.place_and_pay(w, buyer, "L")
    return_id = r.new_return(w, buyer, order_id, RETURN_AMOUNT).get("id", "")
    assert return_id, "CreateReturnRequest returned no return id"
    ok(r.set_return_status(w, seller, return_id, r.REJECTED))
    resp = d.create_dispute(w, buyer, order_id, d.order_seller_id(w, order_id), return_id)
    dispute_id = ok(resp)["dispute"]["id"]
    ok(d.resolve(w, d.admin(w), dispute_id, d.INVESTIGATING))
    w.orders["order"] = order_id
    w.data["return_id"] = return_id
    w.data["dispute_id"] = dispute_id


def rule(w: OicWorld, ruling: str) -> None:
    w.responses["call"] = d.resolve(
        w, d.admin(w), w.data["dispute_id"], d.RESOLVED, f"DISPUTE_RULING_{ruling}"
    )
    ok(w.responses["call"])


def return_state(w: OicWorld) -> str:
    return r.return_status(w, w.actors["buyer"], w.data["return_id"])


def wait_return_state(w: OicWorld, expected: str, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    got = ""
    while time.monotonic() < deadline:
        got = return_state(w)
        if got == expected:
            return
        time.sleep(0.5)
    raise AssertionError(
        f"return {w.data['return_id']} reads {got!r} after {timeout}s, want {expected!r}"
    )


def stays_return_state(w: OicWorld, expected: str, seconds: float) -> None:
    deadline = time.monotonic() + seconds
    while True:
        got = return_state(w)
        assert (
            got == expected
        ), f"return {w.data['return_id']} moved to {got!r}, want it to stay {expected!r}"
        if time.monotonic() >= deadline:
            return
        time.sleep(1)


def dispute_audit_events(w: OicWorld) -> list[dict]:
    found = aud.events_for(
        w, "return_request", w.data["return_id"], action=AUDIT_ACTION, token=d.admin(w).token
    )
    return [e for e in found if e.get("action") == AUDIT_ACTION]


def produce_resolved_fact(w: OicWorld) -> None:
    """A well-formed EventEnvelope{DisputeResolved} for the scenario's dispute, as team-engagement writes it.

    Needed because team-engagement refuses a ruling while team-audit is down (fail closed), so
    the admin ruling itself cannot be issued during the outage the scenario is about.
    """
    stamp = r._field_varint(1, int(time.time()))
    payload = (
        r._field_bytes(1, w.data["dispute_id"].encode())
        + r._field_bytes(2, w.orders["order"].encode())
        + r._field_bytes(3, w.data["return_id"].encode())
        + r._field_varint(4, RESOLVED_STATUS_NUM)
        + r._field_varint(5, BUYER_FAVOUR_NUM)
        + r._field_bytes(6, d.admin(w).user_id.encode())
        + r._field_bytes(7, stamp)
    )
    envelope = (
        r._field_bytes(1, str(uuid.uuid4()).encode())
        + r._field_bytes(2, d.RESOLVED_TYPE)
        + r._field_bytes(3, stamp)
        + r._field_bytes(7, payload)
    )
    stack.produce(d.EVENTS_TOPIC, w.data["dispute_id"].encode(), envelope)


def dlq_records(w: OicWorld) -> list[dict]:
    return r.scan(DLQ_TOPIC, w.data["dispute_id"].encode(), 2.0)


def assert_one_dispute_refund_event(w: OicWorld) -> None:
    found = dispute_audit_events(w)
    assert (
        len(found) == 1
    ), f"want exactly 1 {AUDIT_ACTION} event for {w.data['return_id']}, got {found}"
    event = found[0]
    assert event.get("targetType") in (None, "return_request"), event
    assert event.get("actorId") == d.admin(w).user_id, f"actor mismatch: {event}"
    want = {
        "order_id": w.orders["order"],
        "dispute_id": w.data["dispute_id"],
        "ruling": d.BUYER_FAVOUR,
        "from_status": r.REJECTED,
    }
    got = event.get("metadata") or {}
    for key, val in want.items():
        assert got.get(key) == val, f"metadata[{key}]={got.get(key)!r}, want {val!r}: {event}"
