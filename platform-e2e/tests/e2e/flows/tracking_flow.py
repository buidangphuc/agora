"""Analytics tracking flows (emit-tracking-events).

Emit a browsing beacon at the gateway edge and assert the resulting
`EventEnvelope` (`type = platform.analytics.v1.TrackingEvent`) lands on the
`analytics.events` Kafka topic.

Kafka is a stack dependency, not a unit-test one: the consumer library is
imported lazily inside `consume_tracking_events` so importing this module (and
`pytest --collect-only`) never requires a Kafka client to be installed. When the
assertion actually runs it needs the local stack (broker on `KAFKA_BROKERS`) and
`kafka-python` available in the runner — both provided by the e2e Docker image /
CI, never on a bare host.
"""

from __future__ import annotations

import time
from typing import Any

ENVELOPE_TYPE = "platform.analytics.v1.TrackingEvent"


def consume_tracking_events(
    brokers: str,
    topic: str,
    *,
    contains: str | None = None,
    timeout_s: float = 15.0,
    max_events: int = 2000,
    from_beginning: bool = True,
    settle_s: float = 1.5,
) -> list[dict[str, Any] | bytes]:
    """Drain messages from `topic` and return those matching `contains`.

    `from_beginning=True` scans from the earliest offset so that filtering by a
    unique per-scenario marker (a fresh session id) yields a deterministic
    exactly-one result regardless of when the assertion runs.

    Each element is the decoded envelope (JSON dict when the envelope is
    protojson) or the raw bytes fallback. `contains` filters to envelopes whose
    raw bytes carry that marker (e.g. the envelope type or an EventType enum),
    which is how the exactly-one / correct-type assertions are made without
    vendoring the generated protobuf stubs into the e2e repo.
    """
    try:
        from confluent_kafka import OFFSET_BEGINNING, OFFSET_END, Consumer, TopicPartition
    except ImportError as exc:  # pragma: no cover - stack/CI-only dependency
        raise RuntimeError(
            "confluent-kafka is not installed — the analytics.events assertion is "
            "stack-gated (runs in the e2e Docker image / CI, not on a bare host)."
        ) from exc

    import json

    c = Consumer(
        {
            "bootstrap.servers": brokers,
            "group.id": f"e2e-analytics-{time.time()}",
            "auto.offset.reset": "earliest" if from_beginning else "latest",
            "enable.auto.commit": False,
        }
    )
    offset = OFFSET_BEGINNING if from_beginning else OFFSET_END
    c.assign([TopicPartition(topic, 0, offset)])

    matched: list[dict[str, Any] | bytes] = []
    deadline = time.time() + timeout_s
    last_match = 0.0
    try:
        while time.time() < deadline:
            # Once something matched, keep draining until the topic has been quiet for
            # `settle_s`: a batch publishes several envelopes, and "exactly one" must see extras.
            if matched and time.time() - last_match >= settle_s:
                break
            msg = c.poll(0.5)
            if msg is None:
                continue
            if msg.error():
                continue
            raw = msg.value() or b""
            if contains is not None and contains.encode() not in raw:
                continue
            try:
                matched.append(json.loads(raw.decode("utf-8")))
            except (UnicodeDecodeError, json.JSONDecodeError):
                matched.append(raw)
            last_match = time.time()
            if len(matched) >= max_events:
                break
    finally:
        c.close()
    return matched


# ── Wire decoding (the gateway publishes binary protobuf, not protojson) ─────
_EVENT_TYPES = {
    1: "EVENT_TYPE_VIEW",
    2: "EVENT_TYPE_CLICK",
    3: "EVENT_TYPE_ADD_TO_CART",
    4: "EVENT_TYPE_IMPRESSION",
    5: "EVENT_TYPE_REMOVE_FROM_CART",
    6: "EVENT_TYPE_BEGIN_CHECKOUT",
    7: "EVENT_TYPE_APPLY_PROMOTION",
    8: "EVENT_TYPE_SEARCH_FILTER",
    9: "EVENT_TYPE_FAVORITE",
    10: "EVENT_TYPE_SHARE",
    11: "EVENT_TYPE_VIEW_CART",
    12: "EVENT_TYPE_ADD_SHIPPING_INFO",
    13: "EVENT_TYPE_ADD_PAYMENT_INFO",
    14: "EVENT_TYPE_PURCHASE",
}
_ENVELOPE_PAYLOAD_FIELD = 7  # platform.events.v1.EventEnvelope.payload
_EVENT_TYPE_FIELD = 1  # platform.analytics.v1.TrackingEvent.event_type


def _varint(buf: bytes, pos: int) -> tuple[int, int]:
    shift = value = 0
    while True:
        byte = buf[pos]
        pos += 1
        value |= (byte & 0x7F) << shift
        if not byte & 0x80:
            return value, pos
        shift += 7


def _fields(buf: bytes) -> list[tuple[int, int, bytes | int]]:
    """Top-level (field number, wire type, value) triples of one protobuf message."""
    out: list[tuple[int, int, bytes | int]] = []
    pos = 0
    while pos < len(buf):
        key, pos = _varint(buf, pos)
        number, wire = key >> 3, key & 7
        if wire == 0:
            value, pos = _varint(buf, pos)
            out.append((number, wire, value))
        elif wire == 2:
            size, pos = _varint(buf, pos)
            out.append((number, wire, buf[pos : pos + size]))
            pos += size
        elif wire == 1:
            pos += 8
        elif wire == 5:
            pos += 4
        else:
            break
    return out


def tracking_event_type(envelope: Any) -> str | None:
    """EventType name carried by an `EventEnvelope` published on analytics.events.

    Decodes the binary protobuf without vendored stubs; a protojson dict/str envelope is
    searched for the enum name instead.
    """
    if not isinstance(envelope, (bytes, bytearray)):
        text = str(envelope)
        return next((n for n in _EVENT_TYPES.values() if n in text), None)
    try:
        for number, wire, value in _fields(bytes(envelope)):
            if number == _ENVELOPE_PAYLOAD_FIELD and wire == 2:
                for inner, inner_wire, enum in _fields(bytes(value)):  # type: ignore[arg-type]
                    if inner == _EVENT_TYPE_FIELD and inner_wire == 0:
                        return _EVENT_TYPES.get(int(enum))  # type: ignore[arg-type]
    except (IndexError, ValueError):
        return None
    return None
