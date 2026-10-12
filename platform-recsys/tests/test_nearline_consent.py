"""Nearline consent filter (spec recommendations: drop events without personalisation consent).

Messages are real protobuf bytes (EventEnvelope + TrackingEvent with ConsentState on field 26) driven through
``NearlineConsumer``; counters are observed as ``consent_denied N`` / ``decode_errors N`` in the log
(design.md Clarifications R1 Q3).
"""

from __future__ import annotations

import logging

import pytest

from recsys.nearline.consumer import NearlineConsumer
from recsys.nearline.signals import NearlineSignalAggregator, NearlineSignalStore
from tests.test_nearline_consumer import Clock, FakeSource, Msg

fakeredis = pytest.importorskip("fakeredis")

VIEW, CLICK, IMPRESSION = 1, 2, 4
T1, T2, T3 = 1_790_000_000, 1_790_000_100, 1_790_000_200


def _varint(n: int) -> bytes:
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        out.append(b | (0x80 if n else 0))
        if not n:
            return bytes(out)


def _ld(field: int, data: bytes) -> bytes:
    return _varint(field << 3 | 2) + _varint(len(data)) + data


def _vi(field: int, n: int) -> bytes:
    return _varint(field << 3) + _varint(n)


def consent(personalization: bool, version: int = 1) -> bytes:
    return _vi(1, 1) + _vi(2, 1) + (_vi(3, 1) if personalization else b"") + _vi(4, version)


def event(
    actor: str,
    listing: str,
    etype: int = VIEW,
    *,
    consent_bytes: bytes | None = None,
    at: int = T1,
    event_id: str = "evt",
    category: str = "books",
    position: int = 1,
) -> bytes:
    tracking = _vi(1, etype) + _ld(2, listing.encode()) + _ld(3, b"sess-1") + _vi(7, position)
    tracking += _ld(19, category.encode())
    if consent_bytes is not None:
        tracking += _ld(26, consent_bytes)
    principal = _ld(1, actor.encode()) + _vi(2, 2)
    return (
        _ld(1, event_id.encode())
        + _ld(2, b"platform.analytics.v1.TrackingEvent")
        + _ld(3, _vi(1, at))
        + _ld(4, principal)
        + _ld(7, tracking)
    )


def _run(messages):
    client = fakeredis.FakeRedis(decode_responses=True)
    store = NearlineSignalStore(client)
    agg = NearlineSignalAggregator(store, ttl_seconds=10**12)
    source = FakeSource(messages)
    consumer = NearlineConsumer(source, agg, idle_exit_seconds=3, clock=Clock())
    consumer.run()
    return client, store, source


def _dump(client) -> dict:
    out = {}
    for key in client.keys("*"):
        kind = client.type(key)
        if kind == "zset":
            out[key] = client.zrange(key, 0, -1, withscores=True)
        elif kind == "hash":
            out[key] = client.hgetall(key)
        elif kind == "list":
            out[key] = client.lrange(key, 0, -1)
        elif kind == "set":
            out[key] = sorted(client.smembers(key))
        else:
            out[key] = client.get(key)
    return out


def test_granted_event_is_applied():
    """Scenario: Granted event is applied."""
    client, store, source = _run([Msg(event("u-ok", "L1", consent_bytes=consent(True)))])
    assert client.zrevrange("recs:nearline:user:u-ok:items", 0, 0) == ["L1"]
    assert store.get_recent_items("u-ok") == ["L1"]
    assert source.commits == 1


def test_denied_event_changes_no_per_user_key(caplog):
    """Scenario: Denied event changes no per-user key."""
    with caplog.at_level(logging.INFO):
        client, _, source = _run([Msg(event("u-deny", "L1", consent_bytes=consent(False)))])
    assert _dump(client) == {}  # no user, session, co-view or seen key
    assert "consent_denied 1" in caplog.text
    assert source.commits == 1  # offset committed like an applied event


def test_version_zero_is_denied_even_with_flag_set(caplog):
    """Scenario: Denied event changes no per-user key (consent.version = 0)."""
    with caplog.at_level(logging.INFO):
        client, _, _ = _run([Msg(event("u-v0", "L1", consent_bytes=consent(True, version=0)))])
    assert _dump(client) == {}
    assert "consent_denied 1" in caplog.text


def test_event_without_consent_field_is_denied(caplog):
    """Scenario: Event without a consent field is denied."""
    with caplog.at_level(logging.INFO):
        client, _, _ = _run([Msg(event("u-none", "L1", consent_bytes=None))])
    assert _dump(client) == {}
    assert "consent_denied 1" in caplog.text


def test_denied_event_updates_no_ctr_counter():
    """Scenario: Denied event updates no item CTR counter."""
    denied = consent(False)
    client, _, _ = _run(
        [
            Msg(event("u-deny", "L1", IMPRESSION, consent_bytes=denied, event_id="e1")),
            Msg(event("u-deny", "L1", CLICK, consent_bytes=denied, event_id="e2")),
        ]
    )
    assert client.keys("recs:nearline:ctr:*") == []
    assert _dump(client) == {}


def test_denied_after_grant_is_dropped(caplog):
    """Scenario: Denied event arriving after a grant is dropped."""
    with caplog.at_level(logging.INFO):
        client, _, _ = _run(
            [
                Msg(event("u-late", "L-granted", at=T2, consent_bytes=consent(True), event_id="e1")),
                Msg(event("u-late", "L-denied", at=T3, consent_bytes=consent(False), event_id="e2")),
            ]
        )
    assert "consent_denied 1" in caplog.text
    for key in client.keys("recs:nearline:user:u-late:*"):
        assert "L-denied" not in str(_dump(client)[key])
    assert "L-denied" not in str(_dump(client))


def test_corrupt_message_skipped_and_committed(caplog):
    """Scenario: Corrupt message is skipped and committed."""
    with caplog.at_level(logging.INFO):
        client, store, source = _run(
            [Msg(b"\x0a\x05ab"), Msg(event("u-ok", "L1", consent_bytes=consent(True)))]
        )
    assert "decode_errors 1" in caplog.text
    assert store.get_recent_items("u-ok") == ["L1"]  # the next message was processed
    assert [k for k in client.keys("*") if "u-ok" not in k and "L1" not in k and "sess-1" not in k] == []
    assert source.commits >= 1


def test_grant_after_deny_uses_only_later_events():
    """Scenario: Grant after deny uses only later events."""
    client, store, _ = _run(
        [
            Msg(event("u-late", "L-denied", at=T1, consent_bytes=consent(False), event_id="e1")),
            Msg(event("u-late", "L-granted", at=T2, consent_bytes=consent(True), event_id="e2")),
        ]
    )
    assert client.zrevrange("recs:nearline:user:u-late:items", 0, -1) == ["L-granted"]
    assert "L-denied" not in str(_dump(client))
