"""Order domain-fact flow (ADR-0013): assert team-order's `order.events` output.

team-order writes an outbox row in the same transaction as the PAID transition and
its relayer produces a binary-protobuf `platform.events.v1.EventEnvelope` carrying a
`platform.order.v1.OrderPaidEvent`, keyed by order_id. This module drains the topic
and decodes the envelope without vendoring generated stubs (same approach as
tracking_flow). Like tracking_flow, the Kafka client is imported lazily so
`pytest --collect-only` never needs it; the assertion is stack-gated.
"""

from __future__ import annotations

import time
from typing import Any

from .tracking_flow import _fields

ORDER_PAID_TYPE = "platform.order.v1.OrderPaidEvent"

# platform.events.v1.EventEnvelope field numbers.
_ENV_EVENT_ID, _ENV_TYPE, _ENV_PAYLOAD = 1, 2, 7


def _text(value: bytes | int) -> str:
    return bytes(value).decode("utf-8") if isinstance(value, (bytes, bytearray)) else str(value)


def decode_order_paid_envelope(raw: bytes) -> dict[str, Any]:
    """Decode an `order.events` record into a plain dict.

    Shape: {event_id, type, order_id, buyer_id, total_amount, currency,
    items: [{listing_id, variant_id, seller_id, quantity, unit_price, currency}]}.
    """
    out: dict[str, Any] = {"items": []}
    payload = b""
    for number, wire, value in _fields(raw):
        if number == _ENV_EVENT_ID and wire == 2:
            out["event_id"] = _text(value)
        elif number == _ENV_TYPE and wire == 2:
            out["type"] = _text(value)
        elif number == _ENV_PAYLOAD and wire == 2:
            payload = bytes(value)  # type: ignore[arg-type]
    # platform.order.v1.OrderPaidEvent
    for number, wire, value in _fields(payload):
        if number == 1 and wire == 2:
            out["order_id"] = _text(value)
        elif number == 2 and wire == 2:
            out["buyer_id"] = _text(value)
        elif number == 4 and wire == 0:
            out["total_amount"] = int(value)  # type: ignore[arg-type]
        elif number == 5 and wire == 2:
            out["currency"] = _text(value)
        elif number == 3 and wire == 2:  # OrderLineItemFact
            item: dict[str, Any] = {}
            names = {1: "listing_id", 2: "variant_id", 3: "seller_id", 6: "currency"}
            for n, w, v in _fields(bytes(value)):  # type: ignore[arg-type]
                if n in names and w == 2:
                    item[names[n]] = _text(v)
                elif n == 4 and w == 0:
                    item["quantity"] = int(v)  # type: ignore[arg-type]
                elif n == 5 and w == 0:
                    item["unit_price"] = int(v)  # type: ignore[arg-type]
            out["items"].append(item)
    return out


def consume_order_paid_events(
    brokers: str,
    topic: str,
    order_id: str,
    *,
    timeout_s: float = 30.0,
    settle_s: float = 2.0,
) -> list[dict[str, Any]]:
    """Return decoded OrderPaidEvent envelopes for `order_id` published on `topic`.

    Scans from the earliest offset (the relayer publishes asynchronously, so poll
    until a match appears, then keep draining for `settle_s` so a duplicate would
    be seen by an "exactly one" assertion).
    """
    try:
        from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition
    except ImportError as exc:  # pragma: no cover - stack/CI-only dependency
        raise RuntimeError(
            "confluent-kafka is not installed — the order.events assertion is "
            "stack-gated (runs in the e2e Docker image / CI, not on a bare host)."
        ) from exc

    c = Consumer(
        {
            "bootstrap.servers": brokers,
            "group.id": f"e2e-order-events-{time.time()}",
            "auto.offset.reset": "earliest",
            "enable.auto.commit": False,
        }
    )
    c.assign([TopicPartition(topic, 0, OFFSET_BEGINNING)])
    matched: list[dict[str, Any]] = []
    deadline = time.time() + timeout_s
    last_match = 0.0
    try:
        while time.time() < deadline:
            if matched and time.time() - last_match >= settle_s:
                break
            msg = c.poll(0.5)
            if msg is None or msg.error():
                continue
            raw = msg.value() or b""
            if order_id.encode() not in raw:
                continue
            decoded = decode_order_paid_envelope(raw)
            if decoded.get("type") == ORDER_PAID_TYPE and decoded.get("order_id") == order_id:
                decoded["_key"] = (msg.key() or b"").decode("utf-8", errors="replace")
                matched.append(decoded)
                last_match = time.time()
    finally:
        c.close()
    return matched
