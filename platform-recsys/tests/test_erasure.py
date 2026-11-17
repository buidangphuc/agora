"""recsys.erasure: the identity.events UserErased consumer over fake Redis, Qdrant, Kafka source and producer.

Scenarios AUE-19 to AUE-23 (ml-stores-erasure). Contract the implementation must offer:

    recsys.erasure.consumer.ErasureConsumer(source, redis, qdrant, producer, settings,
                                            idle_exit_seconds=0.0, clock=time.monotonic)
    .run(should_stop=lambda: False) -> stats; commits via source.commit(asynchronous=False) after the
    deletions (and after the DLQ publish); a Redis or Qdrant exception propagates with nothing committed.
"""

from __future__ import annotations

import logging
import re
from types import SimpleNamespace

import pytest

from recsys.config import Settings, load_settings
from recsys.erasure.consumer import ErasureConsumer

USER = "user-7f3a-secret-id"
OTHER = "user-9b21"
DLQ = "identity.events.recsys.dlq"


def _varint(n: int) -> bytes:
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        out.append(b | (0x80 if n else 0))
        if not n:
            return bytes(out)


def _ld(number: int, value: bytes) -> bytes:
    return _varint(number << 3 | 2) + _varint(len(value)) + value


def envelope(type_: str, payload: bytes, event_id: str = "evt-1") -> bytes:
    return _ld(1, event_id.encode()) + _ld(2, type_.encode()) + _ld(7, payload)


def user_erased(user_id: str, event_id: str = "evt-1") -> bytes:
    return envelope("platform.identity.v1.UserErased", _ld(1, user_id.encode()) if user_id else b"", event_id)


STATUS_CHANGED = envelope("platform.identity.v1.UserStatusChanged", _ld(1, USER.encode()))
GARBAGE = b"\x0a\x05ab"


class Msg:
    def __init__(self, value: bytes, error=None, key: bytes | None = None):
        self._value, self._error, self._key = value, error, key

    def value(self):
        return self._value

    def key(self):
        return self._key

    def error(self):
        return self._error


class Log:
    def __init__(self):
        self.events: list[tuple] = []


class Source:
    def __init__(self, messages, log: Log):
        self.messages = list(messages)
        self.log = log
        self.commits = 0
        self.closed = False

    def poll(self, timeout):
        return self.messages.pop(0) if self.messages else None

    def commit(self, asynchronous=False):
        self.commits += 1
        self.log.events.append(("commit",))

    def close(self):
        self.closed = True


class Producer:
    def __init__(self, log: Log):
        self.log = log
        self.sent: list[tuple[str, bytes | None, bytes | None]] = []
        self.headers: list[list[tuple[str, bytes]]] = []

    def produce(self, topic, value=None, key=None, *args, **kwargs):
        self.sent.append((topic, value, key))
        self.headers.append(kwargs.get("headers") or [])
        self.log.events.append(("produce", topic))

    def flush(self, timeout=None):
        return 0

    def poll(self, timeout=0):
        return 0


class Clock:
    def __init__(self):
        self.t = 0.0

    def __call__(self):
        self.t += 1.0
        return self.t


def _glob_to_re(pattern: str) -> re.Pattern:
    out, i = "", 0
    while i < len(pattern):
        c = pattern[i]
        if c == "\\" and i + 1 < len(pattern):
            out += re.escape(pattern[i + 1])
            i += 2
            continue
        out += ".*" if c == "*" else "." if c == "?" else re.escape(c)
        i += 1
    return re.compile(out + r"\Z", re.S)


class Redis:
    """Redis semantics that matter here: SCAN MATCH honours backslash escapes, DEL counts removed keys."""

    def __init__(self, keys=(), fail=False):
        self.store = {k: "v" for k in keys}
        self.fail = fail
        self.deleted = 0

    def _check(self):
        if self.fail:
            raise ConnectionError("redis down")

    def scan_iter(self, match="*", count=None):
        self._check()
        rx = _glob_to_re(match)
        yield from [k for k in list(self.store) if rx.match(k)]

    def delete(self, *keys):
        self._check()
        n = 0
        for k in keys:
            if self.store.pop(k, None) is not None:
                n += 1
        self.deleted += n
        return n

    def unlink(self, *keys):
        return self.delete(*keys)

    def ping(self):
        self._check()
        return True


class Qdrant:
    """Collections of points with payloads; delete accepts a FilterSelector on user_key."""

    def __init__(self, collections: dict[str, dict[int, dict]], fail=False, aliases=None):
        self.collections = collections
        self.aliases = aliases or {}
        self.fail = fail
        self.deleted_from: list[str] = []

    def get_collections(self):
        if self.fail:
            raise ConnectionError("qdrant down")
        return SimpleNamespace(collections=[SimpleNamespace(name=n) for n in self.collections])

    def get_aliases(self):
        return SimpleNamespace(
            aliases=[SimpleNamespace(alias_name=a, collection_name=c) for a, c in self.aliases.items()]
        )

    def delete(self, collection_name, points_selector, **kwargs):
        if self.fail:
            raise ConnectionError("qdrant down")
        assert collection_name in self.collections, "must delete from real collections, not aliases"
        cond = points_selector.filter.must[0]
        assert cond.key == "user_key"
        want = cond.match.value
        pts = self.collections[collection_name]
        for pid in [p for p, v in pts.items() if v["payload"].get("user_key") == want]:
            del pts[pid]
        self.deleted_from.append(collection_name)

    def count(self, collection_name, exact=True):
        return SimpleNamespace(count=len(self.collections[collection_name]))


def user_keys(uid: str) -> list[str]:
    return [
        f"recs:v1:gen:g1:user:{uid}",
        f"recs:v1:gen:g2:user:{uid}",
        f"recs:v1:user:{uid}",
        f"recs:nearline:user:{uid}:items",
        f"recs:nearline:user:{uid}:cats",
    ]


def seeded_qdrant(**kw):
    return Qdrant(
        {
            "user_als_vectors__g1": {
                1: {"payload": {"user_key": USER}},
                2: {"payload": {"user_key": OTHER}},
            },
            "user_als_vectors__g2": {
                3: {"payload": {"user_key": USER}},
                4: {"payload": {"user_key": OTHER}},
            },
            "item_als_vectors": {5: {"payload": {"item_key": USER}}},
        },
        aliases={"user_als_vectors": "user_als_vectors__g2"},
        **kw,
    )


def build(messages, redis=None, qdrant=None, idle=3):
    log = Log()
    source = Source(messages, log)
    producer = Producer(log)
    redis = redis if redis is not None else Redis(user_keys(USER) + user_keys(OTHER))
    qdrant = qdrant if qdrant is not None else seeded_qdrant()
    consumer = ErasureConsumer(
        source, redis, qdrant, producer, Settings(), idle_exit_seconds=idle, clock=Clock()
    )
    return consumer, source, producer, redis, qdrant, log


def test_aue19_user_keys_and_points_removed_and_other_user_untouched():
    consumer, source, producer, redis, qdrant, _ = build([Msg(user_erased(USER))])
    consumer.run()
    assert not [k for k in redis.store if USER in k]
    assert sorted(redis.store) == sorted(user_keys(OTHER))
    remaining = [p for c in qdrant.collections.values() for p in c.values()]
    assert not [p for p in remaining if p["payload"].get("user_key") == USER]
    assert [p for p in qdrant.collections["user_als_vectors__g1"]] == [2]
    assert [p for p in qdrant.collections["user_als_vectors__g2"]] == [4]
    assert 5 in qdrant.collections["item_als_vectors"]
    assert source.commits >= 1 and producer.sent == []


def test_aue19_id_with_glob_characters_does_not_match_other_users():
    victim = "u*"
    other = "ux-keep"
    redis = Redis(user_keys(victim) + user_keys(other) + user_keys("u[1]"))
    consumer, *_ = build([Msg(user_erased(victim))], redis=redis)
    consumer.run()
    assert sorted(redis.store) == sorted(user_keys(other) + user_keys("u[1]"))


def test_aue19_prefix_sharing_id_is_not_deleted():
    redis = Redis(user_keys(USER) + user_keys(USER + "-2"))
    consumer, *_ = build([Msg(user_erased(USER))], redis=redis)
    consumer.run()
    assert sorted(redis.store) == sorted(user_keys(USER + "-2"))


def test_aue19_other_envelope_types_are_skipped_without_dlq_or_deletes():
    consumer, source, producer, redis, qdrant, _ = build([Msg(STATUS_CHANGED)])
    before = dict(redis.store)
    consumer.run()
    assert redis.store == before and producer.sent == []
    assert source.commits >= 1


def test_aue20_redis_failure_exits_without_commit_and_restart_replays():
    redis = Redis(user_keys(USER), fail=True)
    qdrant = seeded_qdrant()
    consumer, source, producer, _, _, _ = build([Msg(user_erased(USER))], redis=redis, qdrant=qdrant)
    with pytest.raises(ConnectionError):
        consumer.run()
    assert source.commits == 0 and source.closed and producer.sent == []
    redis.fail = False
    replay, source2, *_ = build([Msg(user_erased(USER))], redis=redis, qdrant=qdrant)
    replay.run()
    assert redis.store == {} and source2.commits >= 1


def test_aue20_qdrant_failure_exits_without_commit():
    consumer, source, producer, *_ = build([Msg(user_erased(USER))], qdrant=seeded_qdrant(fail=True))
    with pytest.raises(ConnectionError):
        consumer.run()
    assert source.commits == 0 and source.closed and producer.sent == []


def test_aue21_redelivery_and_unknown_user_are_noops_committed_without_dlq():
    consumer, source, producer, redis, qdrant, _ = build(
        [Msg(user_erased(USER)), Msg(user_erased(USER)), Msg(user_erased("never-seen", "evt-3"))]
    )
    consumer.run()
    assert sorted(redis.store) == sorted(user_keys(OTHER))
    assert redis.deleted == 5
    assert producer.sent == []
    assert source.commits >= 1 and source.closed


def test_aue22_poison_records_go_to_dlq_then_commit_and_delete_nothing():
    consumer, source, producer, redis, qdrant, log = build([Msg(GARBAGE), Msg(user_erased(""))])
    before = dict(redis.store)
    consumer.run()
    assert [t for t, _, _ in producer.sent] == [DLQ, DLQ]
    assert producer.sent[0][1] == GARBAGE
    assert redis.store == before and redis.deleted == 0
    assert [len(c) for c in qdrant.collections.values()] == [2, 2, 1]
    first_commit = log.events.index(("commit",))
    assert log.events.index(("produce", DLQ)) < first_commit
    assert log.events.count(("produce", DLQ)) == 2
    assert source.commits >= 1


def test_aue22_poison_records_keep_their_key_and_carry_an_error_header():
    consumer, _, producer, *_ = build([Msg(GARBAGE, key=b"poison-key")])
    consumer.run()
    assert [(t, v, k) for t, v, k in producer.sent] == [(DLQ, GARBAGE, b"poison-key")]
    assert any(h == "dlq-error" and v for h, v in producer.headers[0])


def test_aue23_logs_carry_no_raw_id_and_report_counts(caplog):
    consumer, *_ = build([Msg(user_erased(USER))])
    with caplog.at_level(logging.DEBUG):
        consumer.run()
    text = caplog.text
    assert text
    assert USER not in text
    assert "deleted" in text.lower() and "5" in text


def test_aue23_poison_logs_carry_no_raw_id(caplog):
    consumer, *_ = build([Msg(user_erased(""))])
    with caplog.at_level(logging.DEBUG):
        consumer.run()
    assert USER not in caplog.text


def test_aue19_settings_defaults_and_env(monkeypatch):
    s = Settings()
    assert s.erasure_consumer_group == "platform-recsys.erasure"
    assert s.erasure_dlq_topic == DLQ
    monkeypatch.setenv("ERASURE_CONSUMER_GROUP", "g-x")
    monkeypatch.setenv("ERASURE_DLQ_TOPIC", "t-x")
    s = load_settings()
    assert (s.erasure_consumer_group, s.erasure_dlq_topic) == ("g-x", "t-x")


def test_aue19_readme_documents_the_erasure_consumer():
    from pathlib import Path

    text = (Path(__file__).resolve().parents[1] / "README.md").read_text()
    for needle in ("recsys.erasure", "ERASURE_CONSUMER_GROUP", "ERASURE_DLQ_TOPIC", DLQ):
        assert needle in text
