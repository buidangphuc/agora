"""Helpers for engagement-consumer-key-redaction (area ekr).

Black box: a malformed record keyed by a user/seller-style id is produced to `listing.events`
(the topic team-engagement consumes); it is retried and parked on `listing.events.dlq`. The
observations are the container's JSON log lines and the DLQ record key. Every wait is a bounded
poll, never a sleep.
"""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import time
import uuid
from typing import Any

from tests.e2e.flows import srm_events_flow as ev

PARKED_MSG = "event parked to dlq after max attempts"
FAILED_MSG = "event handler failed"
PARK_S = 240.0  # 5 attempts with backoff, then the DLQ produce


def container() -> str:
    return os.getenv("PLP_TEAM_ENGAGEMENT_CONTAINER", "team-engagement-svc")


def fp(key: bytes) -> str:
    return hashlib.sha256(key).hexdigest()[:12]


def new_user_key(prefix: str = "seller") -> str:
    return f"{prefix}-{uuid.uuid4().hex}"


def poison(key: bytes | None) -> tuple[int, int]:
    """Produce an undecodable record with exactly `key` (None = nil key)."""
    from confluent_kafka import Producer

    producer = Producer({"bootstrap.servers": ev.brokers(), "partitioner": "murmur2_random"})
    done: dict[str, Any] = {}

    def _cb(err, msg):  # noqa: ANN001
        done["err"] = err
        done["pos"] = (msg.partition(), msg.offset()) if err is None else None

    producer.produce(ev.TOPIC, key=key, value=b"\xff\x00not-an-envelope\xfe", on_delivery=_cb)
    producer.flush(30)
    assert done.get("err") is None and done.get("pos"), f"poison publish failed: {done}"
    return done["pos"]


def log_lines(since_s: int) -> list[dict[str, Any]]:
    out = subprocess.run(
        ["docker", "logs", "--since", str(since_s), container()],
        capture_output=True,
        text=True,
        timeout=60,
        check=False,
    )
    lines = []
    for raw in (out.stdout + out.stderr).splitlines():
        try:
            lines.append(json.loads(raw))
        except ValueError:
            continue
    return lines


def raw_logs(since_s: int) -> str:
    out = subprocess.run(
        ["docker", "logs", "--since", str(since_s), container()],
        capture_output=True,
        text=True,
        timeout=60,
        check=False,
    )
    return out.stdout + out.stderr


def wait_parked(
    since_s: int, keys: list[bytes], timeout_s: float = PARK_S
) -> list[dict]:
    """Poll until a parking line exists for every key (by fingerprint OR by raw key, so a
    pre-change build is observed and then fails the assertion rather than timing out)."""
    deadline = time.monotonic() + timeout_s
    while True:
        fps = {fp(x) for x in keys}
        raws = {x.decode("utf-8", "replace") for x in keys}
        parked = [
            m
            for m in log_lines(since_s)
            if m.get("msg") == PARKED_MSG and (m.get("key_fp") in fps or m.get("key") in raws)
        ]
        if len(parked) >= len(keys):
            return parked
        if time.monotonic() >= deadline:
            raise AssertionError(
                f"no '{PARKED_MSG}' log in {container()} within {timeout_s:.0f}s "
                f"(seen {len(parked)}); the poison record never reached the consumer"
            )
        time.sleep(3.0)


def dlq_keys(since_ms: int, wanted: set[bytes], budget_s: float = 60.0) -> set[bytes]:
    """Keys of records on listing.events.dlq (since `since_ms`) that are in `wanted`."""
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    deadline = time.monotonic() + budget_s
    found: set[bytes] = set()
    while time.monotonic() < deadline and found != wanted:
        consumer = Consumer(
            {
                "bootstrap.servers": ev.brokers(),
                "group.id": f"e2e-ekr-{uuid.uuid4().hex}",
                "enable.auto.commit": False,
            }
        )
        try:
            meta = consumer.list_topics(ev.DLQ_TOPIC, timeout=10).topics.get(ev.DLQ_TOPIC)
            if meta is None or meta.error is not None:
                time.sleep(2.0)
                continue
            starts, ends = [], {}
            for p in meta.partitions:
                low, high = consumer.get_watermark_offsets(TopicPartition(ev.DLQ_TOPIC, p), 10)
                if high <= low:
                    continue
                start = consumer.offsets_for_times(
                    [TopicPartition(ev.DLQ_TOPIC, p, since_ms)], timeout=10
                )[0]
                if start.offset < 0:
                    continue
                starts.append(start)
                ends[p] = high
            if starts:
                consumer.assign(starts)
                end_by = time.monotonic() + 15
                while ends and time.monotonic() < end_by:
                    msg = consumer.poll(0.5)
                    if msg is None or msg.error():
                        continue
                    if msg.offset() + 1 >= ends.get(msg.partition(), 0):
                        ends.pop(msg.partition(), None)
                    if (msg.key() or b"") in wanted:
                        found.add(msg.key())
        finally:
            consumer.close()
        if found != wanted:
            time.sleep(2.0)
    return found
