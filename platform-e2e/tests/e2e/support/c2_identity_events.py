"""Helpers for the session-revocation denylist-expiry scenario (area c2).

Publishes a `platform.identity.v1.SessionRevoked` EventEnvelope to the real
`identity.events` topic (hand-encoded protobuf, like tracking_flow's decoder) and reads the
gateway's `gateway_revocation_denylist_size` gauge through the stack's Prometheus.
"""

from __future__ import annotations

import os
import time
import uuid

import httpx

SESSION_REVOKED_TYPE = "platform.identity.v1.SessionRevoked"
TOPIC = "identity.events"
DENYLIST_METRIC = "marketplace_gateway_revocation_denylist_size"


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


def _ld(field: int, data: bytes) -> bytes:
    return _varint(field << 3 | 2) + _varint(len(data)) + data


def _timestamp(epoch_s: float) -> bytes:
    secs = int(epoch_s)
    return _varint(1 << 3 | 0) + _varint(secs)


def encode_session_revoked(session_id: str, user_id: str, expires_at: float) -> bytes:
    payload = (
        _ld(1, session_id.encode()) + _ld(2, user_id.encode()) + _ld(3, _timestamp(expires_at))
    )
    return (
        _ld(1, str(uuid.uuid4()).encode())
        + _ld(2, SESSION_REVOKED_TYPE.encode())
        + _ld(3, _timestamp(time.time()))
        + _ld(7, payload)
    )


def publish_session_revoked(brokers: str, session_id: str, user_id: str, ttl_s: int) -> None:
    from confluent_kafka import Producer

    producer = Producer({"bootstrap.servers": brokers})
    value = encode_session_revoked(session_id, user_id, time.time() + ttl_s)
    producer.produce(TOPIC, key=user_id.encode(), value=value)
    if producer.flush(15) != 0:
        raise RuntimeError("SessionRevoked event was not delivered to identity.events")


def prometheus_url() -> str:
    return os.getenv("PROMETHEUS_URL", "http://localhost:9090").rstrip("/")


def denylist_size() -> int | None:
    """The gateway's current denylist size, or None while Prometheus has no sample."""
    r = httpx.get(f"{prometheus_url()}/api/v1/query", params={"query": DENYLIST_METRIC}, timeout=10)
    r.raise_for_status()
    result = r.json()["data"]["result"]
    return int(float(result[0]["value"][1])) if result else None
