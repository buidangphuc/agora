"""Helpers for order-unpaid-expiry (initiative q10-order-lifecycle).

Online/COD placement through the gateway, GetOrder reads of the lifecycle fields, and a
decoder for the OrderCancelled / OrderPaymentReminder envelopes on ``order.events``.
Reuses ``oic_order_support`` for actors, listings, carts and the raw gateway client.
"""

from __future__ import annotations

import time
from typing import Any

from config.settings import get_settings
from tests.e2e.flows.tracking_flow import _fields
from tests.e2e.support import oic_order_support as s

CANCELLED_TYPE = "platform.order.v1.OrderCancelled"
REMINDER_TYPE = "platform.order.v1.OrderPaymentReminder"

# Overlay (platform-e2e/compose/order-inventory.override.yaml): PAYMENT_DEADLINE=45s,
# PAYMENT_REMINDER_BEFORE=30s, LIFECYCLE_SWEEP_INTERVAL=2s.
DEADLINE_S = 45.0
SWEEP_S = 2.0
EXPIRY_WAIT_S = DEADLINE_S + 3 * SWEEP_S + 20

REASON_ENUM = {
    "CANCEL_REASON_UNSPECIFIED": 0,
    "CANCEL_REASON_CHANGED_MIND": 1,
    "CANCEL_REASON_PAYMENT_EXPIRED": 6,
    "CANCEL_REASON_ADMIN_FORCE_FAIL": 7,
}
STATUS_ENUM = {s.PENDING: 1, s.PAID: 2, s.CANCELLED: 5}


def place(w: s.OicWorld, buyer: s.Actor, listing: str, qty: int, method: str) -> str:
    """One-listing checkout with an explicit payment method; returns the order id."""
    s.post(w, buyer, s.CART, "ClearCart", {})
    s.ensure_address(w, buyer)
    s.add_to_cart(w, buyer, listing, qty)
    resp = s.post(w, buyer, s.ORDER, "CreateOrder", {"paymentMethod": method})
    orders = s.ok(resp).get("orders", [])
    assert len(orders) == 1, orders
    return orders[0]["id"]


def cancel_reason(w: s.OicWorld, actor: s.Actor, order_id: str) -> str:
    return s.get_order(w, actor, order_id).get("cancelReason", "")


def wait_cancelled(w: s.OicWorld, actor: s.Actor, order_id: str, timeout: float) -> None:
    s.wait_status(w, actor, order_id, s.CANCELLED, timeout=timeout)


def _ts_seconds(buf: bytes) -> int:
    for n, wire, v in _fields(buf):
        if n == 1 and wire == 0:
            return int(v)  # type: ignore[arg-type]
    return 0


def _decode(raw: bytes) -> dict[str, Any]:
    out: dict[str, Any] = {}
    payload = b""
    for n, wire, v in _fields(raw):
        if n == 2 and wire == 2:
            out["type"] = bytes(v).decode()  # type: ignore[arg-type]
        elif n == 7 and wire == 2:
            payload = bytes(v)  # type: ignore[arg-type]
    for n, wire, v in _fields(payload):
        if n == 1 and wire == 2:
            out["order_id"] = bytes(v).decode()  # type: ignore[arg-type]
        elif out.get("type") == CANCELLED_TYPE and n == 4 and wire == 0:
            out["previous_status"] = int(v)  # type: ignore[arg-type]
        elif out.get("type") == CANCELLED_TYPE and n == 8 and wire == 0:
            out["reason_code"] = int(v)  # type: ignore[arg-type]
        elif out.get("type") == REMINDER_TYPE and n == 5 and wire == 2:
            out["payment_due_at"] = _ts_seconds(bytes(v))  # type: ignore[arg-type]
    return out


def _require_broker(c: Any) -> None:
    """Fail loudly when order.events is unreachable: an empty read must mean "no event".

    Without this an unreachable broker (e.g. KAFKA_BROKERS unset -> localhost:9092) makes
    every poll return None, so positive steps report "got []" and negative ones pass.
    """
    from confluent_kafka import KafkaException, TopicPartition

    brokers = get_settings().kafka_brokers
    try:
        c.get_watermark_offsets(TopicPartition("order.events", 0), timeout=10)
    except KafkaException as err:
        raise AssertionError(f"order.events unreachable at {brokers!r}: {err}") from err


def events_for(order_id: str, etype: str, *, wait_s: float, settle_s: float = 3.0) -> list[dict]:
    """Envelopes of ``etype`` for ``order_id`` on order.events.

    Polls up to ``wait_s`` for the first match, then drains ``settle_s`` more so a duplicate
    is seen by an "exactly one" assertion. With no match it returns [] after ``wait_s``.
    """
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    c = Consumer(
        {
            "bootstrap.servers": get_settings().kafka_brokers,
            "group.id": f"e2e-order-lifecycle-{time.time()}",
            "auto.offset.reset": "earliest",
            "enable.auto.commit": False,
        }
    )
    c.assign([TopicPartition("order.events", 0, OFFSET_BEGINNING)])
    _require_broker(c)
    matched: list[dict] = []
    deadline = time.time() + wait_s
    last = 0.0
    try:
        while time.time() < deadline:
            if matched and time.time() - last >= settle_s:
                break
            msg = c.poll(0.5)
            if msg is None or msg.error():
                continue
            raw = msg.value() or b""
            if order_id.encode() not in raw:
                continue
            d = _decode(raw)
            if d.get("type") == etype and d.get("order_id") == order_id:
                matched.append(d)
                last = time.time()
    finally:
        c.close()
    return matched


# ── order-receipt-completion (initiative q10-order-lifecycle) ────────────
# Overlay timers (compose/order-inventory.override.yaml): AUTO_COMPLETE_AFTER=12s,
# RETURN_WINDOW=20s, RETURN_AUTO_APPROVE_AFTER=30s, SHIP_BY_AFTER=60s, sweep every 2s.
AUTO_COMPLETE_S = 12.0
RETURN_WINDOW_S = 20.0
AUTO_APPROVE_S = 30.0
SHIP_BY_S = 60.0
COMPLETED_TYPE = "platform.order.v1.OrderCompleted"
RETURN_REQUESTED_TYPE = "platform.order.v1.ReturnRequested"
PAID_TYPE = "platform.order.v1.OrderPaidEvent"
PLACED_TYPE = "platform.order.v1.OrderPlaced"
RETURN_REFUNDED_TYPE = "platform.order.v1.ReturnRefunded"
COMPLETED_BY = {"BUYER": 1, "SELLER": 2, "SYSTEM": 3}
STATUS_WAIT_S = AUTO_COMPLETE_S + AUTO_APPROVE_S + 6 * SWEEP_S + 15


def ts(value: str | None) -> float:
    """Epoch seconds of a protojson Timestamp ('' / None -> 0)."""
    import re
    from datetime import datetime

    if not value:
        return 0.0
    v = re.sub(r"(\.\d{6})\d+", r"\1", value.replace("Z", "+00:00"))
    return datetime.fromisoformat(v).timestamp()


def payloads_for(needle: str, etype: str, *, wait_s: float, settle_s: float = 3.0) -> list[bytes]:
    """Payload bytes of every ``etype`` envelope on order.events whose record mentions ``needle``.

    Same polling contract as ``events_for``: wait up to ``wait_s`` for the first match, then
    drain ``settle_s`` so a duplicate is seen by an "exactly one" assertion.
    """
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    c = Consumer(
        {
            "bootstrap.servers": get_settings().kafka_brokers,
            "group.id": f"e2e-order-receipt-{time.time()}",
            "auto.offset.reset": "earliest",
            "enable.auto.commit": False,
        }
    )
    c.assign([TopicPartition("order.events", 0, OFFSET_BEGINNING)])
    _require_broker(c)
    out: list[bytes] = []
    deadline = time.time() + wait_s
    last = 0.0
    try:
        while time.time() < deadline:
            if out and time.time() - last >= settle_s:
                break
            msg = c.poll(0.5)
            if msg is None or msg.error():
                continue
            raw = msg.value() or b""
            if needle.encode() not in raw:
                continue
            kind, payload = b"", b""
            for n, wire, v in _fields(raw):
                if n == 2 and wire == 2:
                    kind = bytes(v)  # type: ignore[arg-type]
                elif n == 7 and wire == 2:
                    payload = bytes(v)  # type: ignore[arg-type]
            if kind.decode() == etype:
                out.append(payload)
                last = time.time()
    finally:
        c.close()
    return out


def pb(payload: bytes) -> dict[int, Any]:
    """Top-level fields of a payload: number -> int (varint) | str-able bytes; Timestamps via ``pb_ts``."""
    return {n: v for n, _w, v in _fields(payload)}


def pb_str(payload: bytes, number: int) -> str:
    v = pb(payload).get(number, b"")
    return bytes(v).decode() if isinstance(v, (bytes, bytearray)) else ""


def pb_ts(payload: bytes, number: int) -> int:
    """Seconds of a Timestamp field (0 when unset)."""
    v = pb(payload).get(number)
    return _ts_seconds(bytes(v)) if isinstance(v, (bytes, bytearray)) else 0


def pb_int(payload: bytes, number: int) -> int:
    v = pb(payload).get(number, 0)
    return int(v) if isinstance(v, int) else 0


def ship(w: s.OicWorld, order_id: str, seller: s.Actor) -> None:
    s.ok(s.post(w, seller, s.ORDER, "UpdateOrderStatus", {"id": order_id, "status": s.SHIPPED}))


def sleep_until(epoch_s: float) -> None:
    while time.time() < epoch_s:
        time.sleep(0.5)
