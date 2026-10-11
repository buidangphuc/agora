"""Gateway flows for payment-refund-model, return-refund-settlement (area prm-rma).

Everything goes through the public edge (Connect JSON) with real, distinct users: one seller per
scenario, distinct buyers for A-vs-B. The facts team-order emits are read off `order.events`
with a Kafka consumer that scans to the end of the topic (bytes kept verbatim); what
team-payment applied is read from `GetPayment` and the seller's ledger (the reference of a row
is not on the wire, so it is read with psql). Absence assertions follow a tail scan, never a
bare sleep. It reuses the plp_* helpers (one-seller listings, payment waits, ledger reads).
"""

from __future__ import annotations

import time
import uuid
from typing import Any

import httpx

from tests.e2e.flows.tracking_flow import _fields
from tests.e2e.support import oic_order_support as o
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import plp_support as p
from tests.e2e.support.oic_order_support import Actor, OicWorld, ok, post

ORDER = o.ORDER
PAYMENT = o.PAYMENT

PENDING = "RETURN_STATUS_PENDING"
APPROVED = "RETURN_STATUS_APPROVED"
REJECTED = "RETURN_STATUS_REJECTED"
REFUNDED = "RETURN_STATUS_REFUNDED"
SRC_RETURN = "PAYMENT_REFUND_SOURCE_RETURN"
SRC_CANCEL = "PAYMENT_REFUND_SOURCE_ORDER_CANCEL"
SRC_SELLER = "PAYMENT_REFUND_SOURCE_SELLER_OR_ADMIN"
PAY_PARTIAL = "PAYMENT_STATUS_PARTIALLY_REFUNDED"
PAY_REFUNDED = "PAYMENT_STATUS_REFUNDED"

RETURN_REFUNDED_TYPE = b"platform.order.v1.ReturnRefunded"
COD_MESSAGE = "order was not paid online; cash-on-delivery refunds are handled outside the system"
SETTLE_WAIT_S = p.SETTLE_WAIT_S


# ── orders ───────────────────────────────────────────────────────────────
def paid_order(w: OicWorld, amount: int = 500_000, credited: bool = True, buyer_name: str = "b1"):
    """The scenario's seller with a listing, and a buyer's order of `amount` paid online.

    Returns (order id, transaction id). When `credited` the seller's credit is observed first.
    """
    seller = p.seller_with_listing(w, amount, "L")
    b = p.buyer(w, buyer_name)
    order_id, tx_id = p.place_and_pay(w, b, "L")
    w.orders["order"] = order_id
    w.data["tx_id"] = tx_id
    w.data["amount"] = amount
    if credited:
        p.wait_rows(w, seller, p.SETTLEMENT, amount, 1)
    return order_id, tx_id


def cod_order_handed_over(w: OicWorld, amount: int = 500_000, buyer_name: str = "b1") -> str:
    """A cash-on-delivery order the seller handed over (Shipped) without any online payment."""
    seller = p.seller_with_listing(w, amount, "L")
    b = p.buyer(w, buyer_name)
    order_id = o.place_order(w, b, "L", 1)
    o.put_in_status(w, order_id, b, seller, o.SHIPPED)
    w.orders["order"] = order_id
    w.data["amount"] = amount
    return order_id


# ── returns ──────────────────────────────────────────────────────────────
def request_return(
    w: OicWorld, buyer: Actor, order_id: str, amount: int | None, reason: str | None = None
) -> httpx.Response:
    body: dict[str, Any] = {"orderId": order_id, "reason": reason or f"e2e-{uuid.uuid4().hex[:8]}"}
    if amount is not None:
        body["refundAmount"] = amount
    return post(w, buyer, ORDER, "CreateReturnRequest", body)


def new_return(
    w: OicWorld, buyer: Actor, order_id: str, amount: int, reason: str | None = None
) -> dict:
    return ok(request_return(w, buyer, order_id, amount, reason)).get("returnRequest", {})


def set_return_status(w: OicWorld, actor: Actor, return_id: str, status: str) -> httpx.Response:
    return post(w, actor, ORDER, "UpdateReturnStatus", {"id": return_id, "status": status})


def return_status(w: OicWorld, actor: Actor, return_id: str) -> str:
    resp = post(w, actor, ORDER, "GetReturnRequest", {"id": return_id})
    return ok(resp).get("returnRequest", {}).get("status", "")


def list_returns(w: OicWorld, actor: Actor, order_id: str) -> httpx.Response:
    return post(w, actor, ORDER, "ListOrderReturns", {"orderId": order_id})


def approve(w: OicWorld, seller: Actor, return_id: str) -> None:
    ok(set_return_status(w, seller, return_id, APPROVED))


def approve_and_refund(w: OicWorld, seller: Actor, return_id: str) -> None:
    approve(w, seller, return_id)
    ok(set_return_status(w, seller, return_id, REFUNDED))


# ── payments ─────────────────────────────────────────────────────────────
def refunded_of(tx: dict) -> int:
    return int(tx.get("refundedAmount", 0) or 0)


def refunds_of(tx: dict, source: str | None = None) -> list[dict]:
    got = tx.get("refunds", []) or []
    return [r for r in got if source is None or r.get("source") == source]


def applied(refund: dict) -> int:
    return int(refund.get("amount", 0) or 0)


def requested(refund: dict) -> int:
    return int(refund.get("requestedAmount", 0) or 0)


def seller_refund(
    w: OicWorld, seller: Actor, tx_id: str, amount: int, refund_id: str | None = None
) -> httpx.Response:
    """The order's seller refunds `amount` of the payment directly (explicit refund id)."""
    return post(
        w,
        seller,
        PAYMENT,
        "RefundPayment",
        {
            "paymentId": tx_id,
            "amount": amount,
            "reason": "e2e direct refund",
            "refundId": refund_id or f"e2e-{uuid.uuid4().hex[:12]}",
        },
    )


def wait_refunded(
    w: OicWorld,
    actor: Actor,
    order_id: str,
    amount: int,
    status: str,
    timeout: float = SETTLE_WAIT_S,
) -> dict:
    """Poll `GetPayment` until it reads `status` with `amount` refunded; returns the payment."""
    deadline = time.monotonic() + timeout
    tx: dict = {}
    while time.monotonic() < deadline:
        try:
            tx = p.payment_of(w, actor, order_id)
        except (AssertionError, httpx.HTTPError):  # team-payment restarting
            tx = {}
        if tx.get("status") == status and refunded_of(tx) == amount:
            return tx
        time.sleep(0.5)
    raise AssertionError(
        f"payment of order {order_id} is {tx.get('status')!r} with {refunded_of(tx)} refunded, "
        f"want {status!r} with {amount}: {tx.get('refunds')}"
    )


def wait_return_refund(
    w: OicWorld, actor: Actor, order_id: str, return_id: str, timeout: float = SETTLE_WAIT_S
) -> dict:
    """Poll until the payment lists the RETURN refund of `return_id`; returns that refund."""
    deadline = time.monotonic() + timeout
    tx: dict = {}
    while time.monotonic() < deadline:
        try:
            tx = p.payment_of(w, actor, order_id)
        except (AssertionError, httpx.HTTPError):
            tx = {}
        for r in refunds_of(tx, SRC_RETURN):
            if r.get("sourceId") == return_id:
                return r
        time.sleep(0.5)
    raise AssertionError(f"payment of {order_id} lists no RETURN refund of {return_id}: {tx}")


def deductions(w: OicWorld, seller: Actor) -> list[dict]:
    """The seller's REFUND_DEDUCTION rows with their reference (psql: it is not on the wire)."""
    rows = stack.psql_rows(
        "SELECT id, amount, COALESCE(reference_id, '') FROM wallet_ledger "
        f"WHERE seller_id = {stack.sql_lit(seller.user_id)} AND type = {stack.sql_lit(p.DEDUCTION)} "
        "ORDER BY created_at, id"
    )
    return [{"id": r[0], "amount": int(r[1]), "reference": r[2]} for r in rows]


def wait_deductions(
    w: OicWorld, seller: Actor, count: int, timeout: float = SETTLE_WAIT_S
) -> list[dict]:
    deadline = time.monotonic() + timeout
    got: list[dict] = []
    while time.monotonic() < deadline:
        got = deductions(w, seller)
        if len(got) >= count:
            return got
        time.sleep(0.5)
    raise AssertionError(f"seller has {len(got)} REFUND_DEDUCTION row(s), want {count}: {got}")


# ── order.events facts ───────────────────────────────────────────────────
def scan(topic: str, needle: bytes, tail_s: float = 0.0) -> list[dict]:
    """Every record on `topic` containing `needle`, read from the start to the end of the topic.

    After reaching the end that was current when the scan began, it keeps reading for `tail_s`
    so a record the relayer is about to publish is seen too. A missing topic yields [].
    """
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    c = Consumer(
        {
            "bootstrap.servers": stack._brokers(),
            "group.id": f"prm-e2e-{uuid.uuid4().hex}",
            "enable.auto.commit": False,
            "auto.offset.reset": "earliest",
        }
    )
    found: list[dict] = []
    try:
        meta = c.list_topics(topic, timeout=10).topics.get(topic)
        if meta is None or meta.error is not None:
            return found
        parts = [TopicPartition(topic, n, OFFSET_BEGINNING) for n in meta.partitions]
        c.assign(parts)
        ends = {}
        for tp in parts:
            _, high = c.get_watermark_offsets(tp, timeout=10)
            ends[tp.partition] = high
        pos = {tp.partition: -1 for tp in parts}
        at_end_since: float | None = None
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            msg = c.poll(0.5)
            if msg is not None and not msg.error():
                pos[msg.partition()] = msg.offset()
                hdrs = msg.headers() or []
                hay = [msg.key() or b"", msg.value() or b""] + [v or b"" for _, v in hdrs]
                if any(needle in h for h in hay):
                    found.append(
                        {
                            "key": msg.key() or b"",
                            "value": msg.value() or b"",
                            "headers": hdrs,
                            "offset": msg.offset(),
                        }
                    )
            if all(pos[n] + 1 >= ends[n] for n in ends):
                at_end_since = at_end_since or time.monotonic()
                if time.monotonic() - at_end_since >= tail_s:
                    break
    finally:
        c.close()
    return found


def return_facts(return_id: str, tail_s: float = 0.0) -> list[dict]:
    """The ReturnRefunded records of `return_id` on order.events."""
    return [
        r
        for r in scan(stack.ORDER_EVENTS_TOPIC, return_id.encode(), tail_s)
        if RETURN_REFUNDED_TYPE in r["value"]
    ]


def wait_fact(return_id: str, timeout: float = 40.0) -> list[dict]:
    """Poll until the fact is on order.events, then read on for a duplicate."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if return_facts(return_id):
            return return_facts(return_id, tail_s=3.0)
        time.sleep(1)
    return []


def decode_fact(record: dict) -> dict:
    """{type, return_id, order_id, buyer_id, seller_id, refund_amount, currency} of a record."""
    out: dict[str, Any] = {}
    payload = b""
    for number, wire, value in _fields(record["value"]):
        if number == 1 and wire == 2:
            out["event_id"] = bytes(value).decode()
        elif number == 2 and wire == 2:
            out["type"] = bytes(value).decode()
        elif number == 7 and wire == 2:
            payload = bytes(value)
    names = {1: "return_id", 2: "order_id", 3: "buyer_id", 4: "seller_id", 6: "currency"}
    for number, wire, value in _fields(payload):
        if number in names and wire == 2:
            out[names[number]] = bytes(value).decode()
        elif number == 5 and wire == 0:
            out["refund_amount"] = int(value)
    return out


def dlq_records(needle: str, tail_s: float = 0.0) -> list[dict]:
    return scan(stack.DLQ_TOPIC, needle.encode(), tail_s)


# ── hand-built ReturnRefunded (DLQ scenario) ─────────────────────────────
def _varint(n: int) -> bytes:
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        if n:
            out.append(b | 0x80)
        else:
            out.append(b)
            return bytes(out)


def _field_bytes(number: int, data: bytes) -> bytes:
    return _varint(number << 3 | 2) + _varint(len(data)) + data


def _field_varint(number: int, value: int) -> bytes:
    return _varint(number << 3) + _varint(value)


def build_return_refunded(
    return_id: str, order_id: str, buyer_id: str, seller_id: str, amount: int
) -> tuple[bytes, bytes]:
    """(key, value) of a well-formed `EventEnvelope{ReturnRefunded}` as team-order would write it."""
    now = int(time.time())
    stamp = _field_varint(1, now)
    payload = (
        _field_bytes(1, return_id.encode())
        + _field_bytes(2, order_id.encode())
        + _field_bytes(3, buyer_id.encode())
        + _field_bytes(4, seller_id.encode())
        + _field_varint(5, amount)
        + _field_bytes(6, b"VND")
        + _field_bytes(7, stamp)
    )
    envelope = (
        _field_bytes(1, str(uuid.uuid4()).encode())
        + _field_bytes(2, RETURN_REFUNDED_TYPE)
        + _field_bytes(3, stamp)
        + _field_bytes(7, payload)
    )
    return order_id.encode(), envelope
