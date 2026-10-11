"""Black-box `listing.events` access for port-search-read-model-correctness (area srm).

What the search scenarios need from Kafka and OpenSearch, with no vendored stubs:

* an `EventEnvelope` encoder (`ListingChanged`, `ListingStockChanged`, `ListingStatusChanged`,
  `ListingBaseInfoChanged`, `ListingPricingChanged`) with a chosen `occurred_at`, published keyed by
  listing id, and a raw re-publish of a record already on the topic (a redelivery);
* a reader of the records on `listing.events` for one listing (type, `occurred_at`, change type,
  partition/offset) so a scenario can derive times from the REAL domain events;
* `wait_consumed`: the committed offset of the `team-search-indexer` group passing a record;
* a reader of `listing.events.dlq`;
* a reader of one OpenSearch `_doc` (the tombstone / "no document" observations).

Names come from the environment with the local compose defaults: `KAFKA_BROKERS`
(localhost:19092), `SRM_INDEXER_GROUP` (team-search-indexer), `SRM_OPENSEARCH_URL`
(http://localhost:9200), `SRM_OPENSEARCH_INDEX` (listings). confluent-kafka is imported lazily.
"""

from __future__ import annotations

import os
import time
import uuid
from dataclasses import dataclass
from typing import Any

import httpx

from .tracking_flow import _fields

TOPIC = "listing.events"
DLQ_TOPIC = "listing.events.dlq"

LISTING_CHANGED = "platform.listing.v1.ListingChanged"
BASE_INFO_CHANGED = "platform.listing.v1.ListingBaseInfoChanged"
PRICING_CHANGED = "platform.listing.v1.ListingPricingChanged"
STOCK_CHANGED = "platform.listing.v1.ListingStockChanged"
STATUS_CHANGED = "platform.listing.v1.ListingStatusChanged"

# platform.listing.v1 enums
CREATED, UPDATED, DELETED = 1, 2, 3
DRAFT, PUBLISHED, REJECTED = 1, 2, 3


def brokers() -> str:
    return os.getenv("KAFKA_BROKERS", "localhost:19092")


def indexer_group() -> str:
    return os.getenv("SRM_INDEXER_GROUP", "team-search-indexer")


def opensearch_url() -> str:
    return os.getenv("SRM_OPENSEARCH_URL", "http://localhost:9200").rstrip("/")


def opensearch_index() -> str:
    return os.getenv("SRM_OPENSEARCH_INDEX", "listings")


# ── protobuf encoding ────────────────────────────────────────────────────
def _varint(value: int) -> bytes:
    if value < 0:  # int32/int64 negatives are the 64-bit two's complement
        value += 1 << 64
    out = bytearray()
    while True:
        low = value & 0x7F
        value >>= 7
        if value:
            out.append(low | 0x80)
        else:
            out.append(low)
            return bytes(out)


def _int(field: int, value: int) -> bytes:
    return _varint(field << 3) + _varint(value)


def _bytes(field: int, value: bytes) -> bytes:
    return _varint((field << 3) | 2) + _varint(len(value)) + value


def _str(field: int, value: str) -> bytes:
    return _bytes(field, value.encode("utf-8"))


def timestamp(ns: int) -> bytes:
    """google.protobuf.Timestamp of `ns` epoch nanoseconds."""
    seconds, nanos = divmod(ns, 1_000_000_000)
    return _int(1, seconds) + (_int(2, nanos) if nanos else b"")


def listing_msg(
    listing_id: str,
    *,
    title: str = "",
    description: str = "",
    price: int = 100_000,
    currency: str = "VND",
    status: int = PUBLISHED,
    seller_id: str = "",
    category_id: str = "cat-laptop",
    stock: int = 0,
) -> bytes:
    """platform.listing.v1.Listing (a zero/empty field is omitted, as proto3 does)."""
    out = _str(1, listing_id)
    for number, text in ((2, title), (3, description)):
        if text:
            out += _str(number, text)
    if price:
        out += _int(4, price)
    if currency:
        out += _str(5, currency)
    if status:
        out += _int(6, status)
    if seller_id:
        out += _str(7, seller_id)
    if category_id:
        out += _str(9, category_id)
    if stock:
        out += _int(10, stock)
    return out


def envelope(type_name: str, payload: bytes, occurred_at_ns: int | None) -> bytes:
    """platform.events.v1.EventEnvelope; `occurred_at_ns=None` leaves occurred_at absent."""
    out = _str(1, str(uuid.uuid4())) + _str(2, type_name)
    if occurred_at_ns is not None:
        out += _bytes(3, timestamp(occurred_at_ns))
    return out + _bytes(7, payload)


def listing_changed(change_type: int, listing: bytes, occurred_at_ns: int | None) -> bytes:
    return envelope(LISTING_CHANGED, _bytes(1, listing) + _int(2, change_type), occurred_at_ns)


def stock_changed(listing_id: str, stock: int, occurred_at_ns: int | None) -> bytes:
    """ListingStockChanged; `listing_id=""` / `stock=0` are omitted like any proto3 default."""
    payload = (_str(1, listing_id) if listing_id else b"") + (_int(2, stock) if stock else b"")
    return envelope(STOCK_CHANGED, payload, occurred_at_ns)


def status_changed(listing_id: str, status: int, occurred_at_ns: int | None) -> bytes:
    return envelope(STATUS_CHANGED, _str(1, listing_id) + _int(2, status), occurred_at_ns)


def base_info_changed(
    listing_id: str,
    *,
    title: str,
    change_type: int,
    status: int = PUBLISHED,
    seller_id: str = "",
    category_id: str = "cat-laptop",
    occurred_at_ns: int | None,
) -> bytes:
    payload = _str(1, listing_id) + _str(2, title) + _str(4, category_id)
    if seller_id:
        payload += _str(6, seller_id)
    payload += _int(7, status) + _int(8, change_type)
    return envelope(BASE_INFO_CHANGED, payload, occurred_at_ns)


def pricing_changed(listing_id: str, price: int, occurred_at_ns: int | None) -> bytes:
    payload = _str(1, listing_id) + _int(2, price) + _str(4, "VND")
    return envelope(PRICING_CHANGED, payload, occurred_at_ns)


# ── decoding (a record on the topic) ─────────────────────────────────────
def _text(value: bytes | int) -> str:
    return bytes(value).decode("utf-8", "replace") if isinstance(value, bytes) else str(value)


@dataclass
class Record:
    partition: int
    offset: int
    key: str
    value: bytes
    type: str = ""
    occurred_at_ns: int | None = None
    change_type: int = 0
    status: int = 0
    stock: int | None = None
    listing_id: str = ""


def decode(raw: bytes) -> dict[str, Any]:
    """{type, occurred_at_ns, listing_id, change_type, status, stock} of one envelope."""
    out: dict[str, Any] = {
        "type": "",
        "occurred_at_ns": None,
        "listing_id": "",
        "change_type": 0,
        "status": 0,
        "stock": None,
    }
    payload = b""
    for number, wire, value in _fields(raw):
        if number == 2 and wire == 2:
            out["type"] = _text(value)
        elif number == 3 and wire == 2:
            seconds = nanos = 0
            for n, w, v in _fields(bytes(value)):  # type: ignore[arg-type]
                if n == 1 and w == 0:
                    seconds = int(v)  # type: ignore[arg-type]
                elif n == 2 and w == 0:
                    nanos = int(v)  # type: ignore[arg-type]
            out["occurred_at_ns"] = seconds * 1_000_000_000 + nanos
        elif number == 7 and wire == 2:
            payload = bytes(value)  # type: ignore[arg-type]
    for number, wire, value in _fields(payload):
        if out["type"] == LISTING_CHANGED:
            if number == 1 and wire == 2:
                for n, w, v in _fields(bytes(value)):  # type: ignore[arg-type]
                    if n == 1 and w == 2:
                        out["listing_id"] = _text(v)
                    elif n == 6 and w == 0:
                        out["status"] = int(v)  # type: ignore[arg-type]
                    elif n == 10 and w == 0:
                        out["stock"] = int(v)  # type: ignore[arg-type]
            elif number == 2 and wire == 0:
                out["change_type"] = int(value)  # type: ignore[arg-type]
        elif out["type"] == STOCK_CHANGED:
            if number == 1 and wire == 2:
                out["listing_id"] = _text(value)
            elif number == 2 and wire == 0:
                out["stock"] = int(value)  # type: ignore[arg-type]
        elif out["type"] == STATUS_CHANGED:
            if number == 1 and wire == 2:
                out["listing_id"] = _text(value)
            elif number == 2 and wire == 0:
                out["status"] = int(value)  # type: ignore[arg-type]
        elif out["type"] in (BASE_INFO_CHANGED, PRICING_CHANGED):
            if number == 1 and wire == 2:
                out["listing_id"] = _text(value)
            elif number == 8 and wire == 0 and out["type"] == BASE_INFO_CHANGED:
                out["change_type"] = int(value)  # type: ignore[arg-type]
    return out


# ── publish ──────────────────────────────────────────────────────────────
def publish(listing_id: str, value: bytes, *, topic: str = TOPIC) -> tuple[int, int]:
    """Produce one record keyed by `listing_id`; returns its (partition, offset) once acked."""
    from confluent_kafka import Producer

    producer = Producer({"bootstrap.servers": brokers(), "partitioner": "murmur2_random"})
    result: dict[str, Any] = {}

    def _done(err, msg):  # noqa: ANN001
        result["err"] = err
        result["pos"] = (msg.partition(), msg.offset()) if err is None else None

    producer.produce(topic, key=listing_id.encode(), value=value, on_delivery=_done)
    producer.flush(30)
    assert result.get("err") is None and result.get("pos"), f"publish to {topic} failed: {result}"
    return result["pos"]


def republish(record: Record) -> tuple[int, int]:
    """Re-publish the raw bytes of a record already on the topic, unchanged, same key."""
    return publish(record.key, record.value)


# ── read ─────────────────────────────────────────────────────────────────
def _scan(topic: str, needle: str, since_ms: int | None, budget_s: float = 30.0) -> list[Record]:
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    consumer = Consumer(
        {
            "bootstrap.servers": brokers(),
            "group.id": f"e2e-srm-{uuid.uuid4().hex}",
            "enable.auto.commit": False,
        }
    )
    found: list[Record] = []
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
                if resolved.offset < 0:
                    continue
                start = resolved
            assignments.append(start)
            ends[partition] = high
        if not assignments:
            return found
        consumer.assign(assignments)
        deadline = time.monotonic() + budget_s
        while ends and time.monotonic() < deadline:
            msg = consumer.poll(0.5)
            if msg is None or msg.error():
                continue
            if msg.offset() + 1 >= ends.get(msg.partition(), 0):
                ends.pop(msg.partition(), None)
            raw = msg.value() or b""
            if needle.encode() not in raw:
                continue
            info = decode(raw)
            if info["listing_id"] != needle:
                continue
            found.append(
                Record(
                    partition=msg.partition(),
                    offset=msg.offset(),
                    key=(msg.key() or b"").decode("utf-8", "replace"),
                    value=raw,
                    type=info["type"],
                    occurred_at_ns=info["occurred_at_ns"],
                    change_type=info["change_type"],
                    status=info["status"],
                    stock=info["stock"],
                    listing_id=info["listing_id"],
                )
            )
    finally:
        consumer.close()
    return found


def records_for(listing_id: str, since_ms: int | None = None) -> list[Record]:
    """Every record about `listing_id` now on `listing.events`, in offset order."""
    return sorted(_scan(TOPIC, listing_id, since_ms), key=lambda r: (r.partition, r.offset))


def dlq_records_for(listing_id: str, since_ms: int | None = None) -> list[Record]:
    """Every record about `listing_id` parked on `listing.events.dlq`."""
    return sorted(_scan(DLQ_TOPIC, listing_id, since_ms), key=lambda r: (r.partition, r.offset))


def wait_for_record(
    listing_id: str,
    *,
    type_name: str,
    change_type: int | None = None,
    since_ms: int | None = None,
    timeout_s: float = 30.0,
) -> Record:
    """Poll until the domain has written a record of this type (and change type) for the listing."""
    deadline = time.monotonic() + timeout_s
    while True:
        for rec in records_for(listing_id, since_ms):
            if rec.type == type_name and (change_type is None or rec.change_type == change_type):
                return rec
        if time.monotonic() >= deadline:
            raise AssertionError(
                f"no {type_name} (change_type={change_type}) for {listing_id} on {TOPIC} "
                f"within {timeout_s:.0f}s"
            )
        time.sleep(1.0)


def committed_offset(partition: int, group: str | None = None) -> int:
    """Next offset the indexer group will read on `listing.events` (-1001 when none)."""
    from confluent_kafka import Consumer, TopicPartition

    consumer = Consumer(
        {
            "bootstrap.servers": brokers(),
            "group.id": group or indexer_group(),
            "enable.auto.commit": False,
        }
    )
    try:
        return consumer.committed([TopicPartition(TOPIC, partition)], timeout=10)[0].offset
    finally:
        consumer.close()


def wait_consumed(partition: int, offset: int, timeout_s: float = 60.0) -> None:
    """Block (bounded poll) until the indexer group's committed offset has passed `offset`."""
    deadline = time.monotonic() + timeout_s
    last = -1
    while time.monotonic() < deadline:
        last = committed_offset(partition)
        if last > offset:
            return
        time.sleep(0.5)
    raise AssertionError(
        f"the {indexer_group()} group did not consume {TOPIC}[{partition}]@{offset} within "
        f"{timeout_s:.0f}s (committed offset {last})"
    )


def publish_and_wait(listing_id: str, value: bytes, timeout_s: float = 60.0) -> tuple[int, int]:
    """Publish a crafted record and wait until the indexer has consumed past it."""
    partition, offset = publish(listing_id, value)
    wait_consumed(partition, offset, timeout_s)
    return partition, offset


def now_ns() -> int:
    return time.time_ns()


# ── OpenSearch _doc ──────────────────────────────────────────────────────
def os_doc(listing_id: str) -> dict[str, Any] | None:
    """The `_source` of the read-model document `listing_id`, or None when there is none."""
    resp = httpx.get(f"{opensearch_url()}/{opensearch_index()}/_doc/{listing_id}", timeout=15.0)
    if resp.status_code == 404:
        return None
    resp.raise_for_status()
    body = resp.json()
    return body.get("_source") if body.get("found") else None
