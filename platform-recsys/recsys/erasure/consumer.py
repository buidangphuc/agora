"""Kafka consumer that erases a user's recsys data on ``platform.identity.v1.UserErased``.

Delivery is at least once: offsets are committed only after the Redis and Qdrant deletes (or the DLQ
publish for a poison record). A Redis or Qdrant failure propagates with the offset uncommitted, so the
restart replays the event; the deletes are idempotent. The user id is never logged.
"""

from __future__ import annotations

import logging
import time
from collections.abc import Callable
from dataclasses import dataclass

from recsys.config import Settings
from recsys.nearline.wire import DecodeError, _fields

log = logging.getLogger("recsys.erasure")

USER_ERASED_TYPE = "platform.identity.v1.UserErased"
SCAN_COUNT = 500


@dataclass
class Stats:
    erased: int = 0
    skipped: int = 0
    poison: int = 0
    redis_keys_deleted: int = 0
    qdrant_collections: int = 0


def _escape_glob(value: str) -> str:
    out = []
    for c in value:
        if c in "\\*?[]":
            out.append("\\")
        out.append(c)
    return "".join(out)


def _decode(value: bytes) -> tuple[str, str]:
    """Return (envelope type, user id) of an identity.events message; DecodeError if malformed."""
    type_ = ""
    payload = b""
    for number, field in _fields(value):
        if number == 2 and isinstance(field, bytes):
            type_ = field.decode("utf-8")
        elif number == 7 and isinstance(field, bytes):
            payload = field
    if type_ != USER_ERASED_TYPE:
        return type_, ""
    user_id = ""
    for number, field in _fields(payload):
        if number == 1 and isinstance(field, bytes):
            user_id = field.decode("utf-8")
    return type_, user_id


class ErasureConsumer:
    def __init__(
        self,
        source,
        redis,
        qdrant,
        producer,
        settings: Settings,
        idle_exit_seconds: float = 0.0,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self.source = source
        self.redis = redis
        self.qdrant = qdrant
        self.producer = producer
        self.settings = settings
        self.idle_exit_seconds = idle_exit_seconds
        self._clock = clock
        self.stats = Stats()

    def _patterns(self, uid: str) -> list[str]:
        s = self.settings
        esc = _escape_glob(uid)
        return [
            f"{s.gen_key_prefix}*:user:{esc}",
            s.user_cache_key(esc),
            f"{s.cache_prefix}:nearline:user:{esc}:*",
        ]

    def _delete_redis(self, uid: str) -> int:
        removed = 0
        for pattern in self._patterns(uid):
            keys = list(self.redis.scan_iter(match=pattern, count=SCAN_COUNT))
            if keys:
                removed += self.redis.delete(*keys)
        return removed

    def _delete_qdrant(self, uid: str) -> int:
        from qdrant_client import models  # noqa: PLC0415

        base = self.settings.qdrant_user_collection
        names = [c.name for c in self.qdrant.get_collections().collections]
        targets = [n for n in names if n == base or n.startswith(base + "__")]
        selector = models.FilterSelector(
            filter=models.Filter(
                must=[models.FieldCondition(key="user_key", match=models.MatchValue(value=uid))]
            )
        )
        for name in targets:
            self.qdrant.delete(collection_name=name, points_selector=selector)
        return len(targets)

    def _poison(self, value: bytes, reason: str, key: bytes | None = None) -> None:
        self.producer.produce(
            self.settings.erasure_dlq_topic,
            value=value,
            key=key,
            headers=[("dlq-error", reason.encode("utf-8"))],
        )
        self.producer.flush()
        self.stats.poison += 1
        log.warning("poison record parked on %s: %s", self.settings.erasure_dlq_topic, reason)

    def handle(self, value: bytes, key: bytes | None = None) -> None:
        try:
            type_, uid = _decode(value)
        except (DecodeError, UnicodeDecodeError) as exc:
            self._poison(value, f"undecodable: {exc}", key)
            return
        if type_ != USER_ERASED_TYPE:
            self.stats.skipped += 1
            return
        if not uid:
            self._poison(value, "UserErased without a user id", key)
            return
        keys = self._delete_redis(uid)
        collections = self._delete_qdrant(uid)
        self.stats.erased += 1
        self.stats.redis_keys_deleted += keys
        self.stats.qdrant_collections += collections
        log.info("user erased: deleted %d redis keys, cleaned %d qdrant collections", keys, collections)

    def run(self, should_stop: Callable[[], bool] = lambda: False, poll_timeout: float = 1.0) -> Stats:
        last_message = self._clock()
        try:
            while not should_stop():
                msg = self.source.poll(poll_timeout)
                now = self._clock()
                if msg is None:
                    if self.idle_exit_seconds and now - last_message >= self.idle_exit_seconds:
                        log.info("no message for %.0fs, exiting: %s", self.idle_exit_seconds, self.stats)
                        break
                    continue
                if msg.error():
                    log.warning("kafka error: %s", msg.error())
                    continue
                last_message = now
                self.handle(msg.value(), msg.key())
                self.source.commit(asynchronous=False)
        finally:
            self.source.close()
        return self.stats


def make_kafka_source(settings: Settings):
    from confluent_kafka import Consumer  # noqa: PLC0415

    consumer = Consumer(
        {
            "bootstrap.servers": settings.kafka_brokers,
            "group.id": settings.erasure_consumer_group,
            "enable.auto.commit": False,
            "auto.offset.reset": "earliest",
        }
    )
    consumer.subscribe([settings.identity_events_topic])
    return consumer


def make_kafka_producer(settings: Settings):
    from confluent_kafka import Producer  # noqa: PLC0415

    return Producer({"bootstrap.servers": settings.kafka_brokers})
