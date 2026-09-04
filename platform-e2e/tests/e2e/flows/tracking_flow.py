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
        from confluent_kafka import Consumer, TopicPartition, OFFSET_BEGINNING, OFFSET_END
    except ImportError as exc:  # pragma: no cover - stack/CI-only dependency
        raise RuntimeError(
            "confluent-kafka is not installed — the analytics.events assertion is "
            "stack-gated (runs in the e2e Docker image / CI, not on a bare host)."
        ) from exc

    import json

    c = Consumer({
        "bootstrap.servers": brokers,
        "group.id": f"e2e-analytics-{time.time()}",
        "auto.offset.reset": "earliest" if from_beginning else "latest",
        "enable.auto.commit": False,
    })
    offset = OFFSET_BEGINNING if from_beginning else OFFSET_END
    c.assign([TopicPartition(topic, 0, offset)])

    matched: list[dict[str, Any] | bytes] = []
    deadline = time.time() + timeout_s
    try:
        while time.time() < deadline:
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
            if len(matched) >= max_events:
                break
            if matched:
                break
    finally:
        c.close()
    return matched
