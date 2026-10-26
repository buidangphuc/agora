"""Gateway flows for engagement-dispute-escalation (initiative q6-trust-enforcement).

Black box through the public edge with real, distinct users. The returns come from the prm_rma
helpers, the audit trail is read with QueryAuditLog as admin, and the DisputeResolved fact is read
off ``engagement.events`` with the prm_rma topic scan. Timers come from
compose/order-inventory.override.yaml (auto-complete 12s, return window 20s).
"""

from __future__ import annotations

import os
import time
from typing import Any

import httpx

from tests.e2e.flows.tracking_flow import _fields
from tests.e2e.support import audit_support as aud
from tests.e2e.support import oic_order_support as o
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_rma_support as r
from tests.e2e.support.oic_order_support import Actor, OicWorld, ok, post

ENGAGEMENT = "/platform.engagement.v1.EngagementService"
EVENTS_TOPIC = "engagement.events"
RESOLVED_TYPE = b"platform.engagement.v1.DisputeResolved"
OPEN = "DISPUTE_STATUS_OPEN"
INVESTIGATING = "DISPUTE_STATUS_INVESTIGATING"
RESOLVED = "DISPUTE_STATUS_RESOLVED"
REJECTED = "DISPUTE_STATUS_REJECTED"
BUYER_FAVOUR = "DISPUTE_RULING_BUYER_FAVOUR"
SELLER_FAVOUR = "DISPUTE_RULING_SELLER_FAVOUR"
RULING_UNSPECIFIED = "DISPUTE_RULING_UNSPECIFIED"
_STATUS_NUM = {1: OPEN, 2: INVESTIGATING, 3: RESOLVED, 4: REJECTED}
_RULING_NUM = {0: RULING_UNSPECIFIED, 1: BUYER_FAVOUR, 2: SELLER_FAVOUR}


def audit_container() -> str:
    return os.getenv("AUDIT_CONTAINER", "agora-team-audit-svc")


def parties(w: OicWorld) -> None:
    p.seller_with_listing(w, 500_000, "L")
    p.buyer(w, "buyer")


def order_seller_id(w: OicWorld, order_id: str) -> str:
    seller_id = o.get_order(w, w.actors["buyer"], order_id).get("sellerId", "")
    assert seller_id, f"order {order_id} carries no sellerId"
    return seller_id


def order_with_return(w: OicWorld, order: str, state: str, ret: str) -> tuple[str, str]:
    """A shipped COD order of the scenario's seller with one return in `state`.

    Everything runs back to back: a rejected return escalates only while the order is shipped or
    inside the 20 s return window that follows the 12 s auto-complete.
    """
    seller, buyer = w.actors["seller"], w.actors["buyer"]
    order_id = o.place_order(w, buyer, "L", 1)
    o.put_in_status(w, order_id, buyer, seller, o.SHIPPED)
    return_id = r.new_return(w, buyer, order_id, 100_000).get("id", "")
    assert return_id, "CreateReturnRequest returned no return id"
    target = {"rejected": r.REJECTED, "approved": r.APPROVED}[state]
    ok(r.set_return_status(w, seller, return_id, target))
    w.orders[order] = order_id
    w.data.setdefault("returns", {})[ret] = return_id
    return order_id, return_id


def plain_order(w: OicWorld, order: str) -> str:
    order_id = o.place_order(w, w.actors["buyer"], "L", 1)
    w.orders[order] = order_id
    return order_id


def create_dispute(
    w: OicWorld, actor: Actor | None, order_id: str, defendant_id: str, return_id: str = ""
) -> httpx.Response:
    body: dict[str, Any] = {
        "orderId": order_id,
        "defendantId": defendant_id,
        "reason": "e2e dispute escalation",
        "evidenceUrls": [],
    }
    if return_id:
        body["returnId"] = return_id
    return post(w, actor, ENGAGEMENT, "CreateDispute", body)


def admin(w: OicWorld) -> Actor:
    return w.actors.get("admin") or o.login_admin(w)


def resolve(
    w: OicWorld, actor: Actor | None, dispute_id: str, status: str, ruling: str = ""
) -> httpx.Response:
    body: dict[str, Any] = {
        "disputeId": dispute_id,
        "status": status,
        "resolution": f"e2e {status}",
    }
    if ruling:
        body["ruling"] = ruling
    return post(w, actor, ENGAGEMENT, "ResolveDispute", body)


def get_dispute(w: OicWorld, actor: Actor, dispute_id: str) -> dict:
    return ok(post(w, actor, ENGAGEMENT, "GetDispute", {"disputeId": dispute_id})).get(
        "dispute", {}
    )


def list_order_disputes(w: OicWorld, actor: Actor | None, order_id: str) -> httpx.Response:
    return post(w, actor, ENGAGEMENT, "ListOrderDisputes", {"orderId": order_id})


def list_all_open(w: OicWorld, actor: Actor, status: str, max_pages: int = 60) -> list[dict]:
    """Every dispute of `status`, following the cursor (oldest first)."""
    out: list[dict] = []
    cursor = ""
    for _ in range(max_pages):
        page: dict[str, Any] = {"pageSize": 100}
        if cursor:
            page["cursor"] = cursor
        body = ok(post(w, actor, ENGAGEMENT, "ListDisputes", {"status": status, "page": page}))
        out.extend(body.get("disputes", []))
        cursor = (body.get("page") or {}).get("nextCursor", "")
        if not cursor:
            return out
    raise AssertionError("ListDisputes paging did not terminate")


def audit_events(w: OicWorld, dispute_id: str) -> list[dict]:
    token = admin(w).token
    resp = aud.query_events(token, target_type="dispute", target_id=dispute_id, action="dispute.resolve")
    assert resp.status_code == 200, f"QueryAuditLog {resp.status_code}: {resp.text}"
    return [e for e in resp.json().get("events", []) if e.get("action") == "dispute.resolve"]


def wait_audit_up(w: OicWorld, timeout: float = 60.0) -> None:
    token = admin(w).token
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            if aud.query_events(token, page_size=1).status_code == 200:
                time.sleep(aud.RECONNECT_GRACE_S)
                return
        except aud.AuditUnavailable:
            pass
        time.sleep(1)
    raise TimeoutError("team-audit did not recover")


def resolved_facts(dispute_id: str, tail_s: float = 0.0) -> list[dict]:
    return [
        rec
        for rec in r.scan(EVENTS_TOPIC, dispute_id.encode(), tail_s)
        if RESOLVED_TYPE in rec["value"]
    ]


def wait_resolved_fact(dispute_id: str, timeout: float = 10.0) -> list[dict]:
    """Poll up to `timeout` for the fact, then read on a little for a duplicate."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if resolved_facts(dispute_id):
            return resolved_facts(dispute_id, tail_s=3.0)
        time.sleep(1)
    return []


def decode_resolved(record: dict) -> dict:
    """{type, key, dispute_id, order_id, return_id, status, ruling, resolver_id} of a record."""
    out: dict[str, Any] = {"key": record["key"].decode()}
    payload = b""
    for number, wire, value in _fields(record["value"]):
        if number == 2 and wire == 2:
            out["type"] = bytes(value).decode()
        elif number == 7 and wire == 2:
            payload = bytes(value)
    names = {1: "dispute_id", 2: "order_id", 3: "return_id", 6: "resolver_id"}
    for number, wire, value in _fields(payload):
        if number in names and wire == 2:
            out[names[number]] = bytes(value).decode()
        elif number == 4 and wire == 0:
            out["status"] = _STATUS_NUM.get(int(value), str(value))
        elif number == 5 and wire == 0:
            out["ruling"] = _RULING_NUM.get(int(value), str(value))
    out.setdefault("ruling", RULING_UNSPECIFIED)
    return out
