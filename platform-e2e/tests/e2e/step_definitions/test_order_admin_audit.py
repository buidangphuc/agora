"""Binds order/admin_audit.feature (change order-admin-audit, initiative q3-audit-log).

Black box through the gateway. Order setup, the order/saga calls and the generic call assertions are
reused from oic_order_steps; audit assertions go through support/audit_support (read only).
"""

from __future__ import annotations

import re

from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.support import audit_support as aud
from tests.e2e.support import oic_order_support as s
from tests.e2e.support.world import World

scenarios("order/admin_audit.feature")


@given("the seeded order admin")
def given_order_admin(oic):
    s.login_admin(oic)


@given(parsers.parse('"{buyer}" has a Requested return on the order'))
def given_requested_return(oic, buyer):
    """A return can only be requested on a shipped order: the seller ships (unaudited), the buyer asks."""
    b = oic.actors[buyer]
    seller = next(iter(oic.listings.values()))["seller"]
    order_id = oic.orders["order"]
    s.ok(s.post(oic, seller, s.ORDER, "UpdateOrderStatus", {"id": order_id, "status": s.SHIPPED}))
    resp = s.post(
        oic,
        b,
        s.ORDER,
        "CreateReturnRequest",
        {"orderId": order_id, "reason": "changed_mind", "refundAmount": "1000"},
    )
    ret = s.ok(resp).get("returnRequest", {})
    assert ret.get("id") and ret.get("status") == "RETURN_STATUS_PENDING", resp.text
    oic.data["return_id"] = ret["id"]


@when("the admin approves the return")
def when_admin_approves_return(oic):
    oic.responses["last"] = s.post(
        oic,
        oic.actors["admin"],
        s.ORDER,
        "UpdateReturnStatus",
        {"id": oic.data["return_id"], "status": "RETURN_STATUS_APPROVED"},
    )


@when(parsers.parse('the admin creates a shipment with carrier "{carrier}" for the order'))
def when_admin_creates_shipment(oic, carrier):
    oic.responses["last"] = s.post(
        oic,
        oic.actors["admin"],
        s.ORDER,
        "CreateShipment",
        {"orderId": oic.orders["order"], "carrier": carrier, "trackingCode": f"AUD{oic.orders['order'][:8]}"},
    )


@given("team-audit is stopped")
def given_audit_stopped(oic, world: World):
    aud.stop_audit(world)


@when("team-audit is running again")
def when_audit_running(oic, world: World):
    aud.start_audit(world)


def _target(oic, kind: str) -> tuple[str, str]:
    return ("return_request", oic.data["return_id"]) if kind == "return" else ("order", oic.orders["order"])


def _meta(oic, raw: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for key, quoted, ref in re.findall(r'(\w+)=(?:"([^"]*)"|<([^>]*)>)', raw):
        if ref:
            m = re.fullmatch(r'seller "(\w+)" id', ref)
            if m:
                out[key] = oic.actors[m.group(1)].user_id
            elif ref == "order id":
                out[key] = oic.orders["order"]
            else:
                raise AssertionError(f"unknown placeholder <{ref}>")
        else:
            out[key] = quoted
    return out


@then(
    parsers.re(
        r'exactly one "(?P<action>[^"]+)" audit event exists for the (?P<kind>order|return)'
        r"(?: with metadata (?P<meta>.+))?"
    )
)
def then_one_event(oic, world: World, action, kind, meta):
    ttype, tid = _target(oic, kind)
    oic.data["audit_event"] = aud.assert_one_event(
        world,
        target_type=ttype,
        target_id=tid,
        action=action,
        actor_id=oic.actors["admin"].user_id,
        metadata=_meta(oic, meta or ""),
    )


@then(parsers.parse('that audit event carries only the metadata key "{key}"'))
def then_only_key(oic, key):
    got = set((oic.data["audit_event"].get("metadata") or {}).keys())
    assert got == {key}, f"metadata keys {got}, want only {key}: {oic.data['audit_event']}"


@then("no audit event exists for the order")
def then_no_event(oic, world: World):
    aud.assert_no_event(world, target_type="order", target_id=oic.orders["order"])
