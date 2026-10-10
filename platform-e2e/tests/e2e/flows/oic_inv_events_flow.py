"""Read `ListingStockChanged` envelopes from the `listing.events` topic.

team-domain's outbox relayer publishes a binary-protobuf `platform.events.v1.EventEnvelope`
(`type = platform.listing.v1.ListingStockChanged`) keyed by listing id. Decoded here without
vendored stubs (same approach as tracking_flow). confluent-kafka is imported lazily.
"""

from __future__ import annotations

import time
from typing import Any

from .tracking_flow import _fields

STOCK_CHANGED_TYPE = "platform.listing.v1.ListingStockChanged"


def _text(value: bytes | int) -> str:
    return (
        bytes(value).decode("utf-8", errors="replace") if isinstance(value, bytes) else str(value)
    )


def decode_stock_changed(raw: bytes) -> dict[str, Any]:
    """{type, listing_id, stock} of one `listing.events` record (stock 0 when omitted)."""
    out: dict[str, Any] = {"type": "", "listing_id": "", "stock": 0}
    payload = b""
    for number, wire, value in _fields(raw):
        if number == 2 and wire == 2:
            out["type"] = _text(value)
        elif number == 7 and wire == 2:
            payload = bytes(value)  # type: ignore[arg-type]
    if out["type"] == STOCK_CHANGED_TYPE:
        for number, wire, value in _fields(payload):
            if number == 1 and wire == 2:
                out["listing_id"] = _text(value)
            elif number == 2 and wire == 0:
                out["stock"] = int(value)  # type: ignore[arg-type]
    return out


def read_stock_events(
    brokers: str, topic: str, listing_id: str, since_ms: int | None = None
) -> list[dict[str, Any]]:
    """Every ListingStockChanged for `listing_id` now on `topic`, with partition/offset/key.

    `since_ms` (epoch ms) starts each partition at the first record at/after that time, so a
    scenario does not rescan the whole topic.
    """
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    consumer = Consumer(
        {
            "bootstrap.servers": brokers,
            "group.id": f"e2e-oic-inv-{time.time()}",
            "enable.auto.commit": False,
        }
    )
    found: list[dict[str, Any]] = []
    try:
        meta = consumer.list_topics(topic, timeout=10).topics.get(topic)
        if meta is None or meta.error is not None:
            return found
        assignments, ends = [], {}
        for partition in meta.partitions:
            low, high = consumer.get_watermark_offsets(TopicPartition(topic, partition), 10)
            if high <= low:
                continue
            start = TopicPartition(topic, partition, OFFSET_BEGINNING)
            if since_ms is not None:
                resolved = consumer.offsets_for_times(
                    [TopicPartition(topic, partition, since_ms)], timeout=10
                )[0]
                if resolved.offset < 0:  # nothing at/after `since_ms`
                    continue
                start = resolved
            assignments.append(start)
            ends[partition] = high
        if not assignments:
            return found
        consumer.assign(assignments)
        deadline = time.time() + 30
        while ends and time.time() < deadline:
            msg = consumer.poll(0.5)
            if msg is None or msg.error():
                continue
            if msg.offset() + 1 >= ends.get(msg.partition(), 0):
                ends.pop(msg.partition(), None)
            raw = msg.value() or b""
            if listing_id.encode() not in raw and listing_id.encode() != (msg.key() or b""):
                continue
            decoded = decode_stock_changed(raw)
            if decoded["type"] == STOCK_CHANGED_TYPE and decoded["listing_id"] == listing_id:
                decoded.update(
                    partition=msg.partition(),
                    offset=msg.offset(),
                    key=(msg.key() or b"").decode("utf-8", errors="replace"),
                )
                found.append(decoded)
    finally:
        consumer.close()
    return found
