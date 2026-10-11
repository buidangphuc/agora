"""Kafka consumer that feeds ``NearlineSignalAggregator`` from ``analytics.events``.

Delivery is at least once: offsets are committed only after the batch they cover was applied to Redis.
A Redis failure stops the process (non-zero exit) with the offsets uncommitted, so the orchestrator's
restart replays the batch; the aggregator's replay guard (``recs:nearline:seen:<event_id>``) makes that
replay idempotent. A message that is not a tracking event, or cannot be decoded, is skipped and counted.
"""

from __future__ import annotations

import logging
import time
from collections.abc import Callable
from dataclasses import dataclass

from recsys.config import Settings
from recsys.nearline.signals import NearlineSignalAggregator
from recsys.nearline.wire import DecodeError, to_interaction

log = logging.getLogger("recsys.nearline")

COMMIT_EVERY = 100  # messages
LOG_EVERY_SECONDS = 60.0


@dataclass
class Stats:
    applied: int = 0
    ignored: int = 0  # not a nearline event, stale, or a redelivery
    undecodable: int = 0


class NearlineConsumer:
    """Pulls messages from ``source`` (a confluent-kafka style Consumer) and applies them.

    ``source`` needs ``poll(timeout) -> message | None``, ``commit(asynchronous=False)`` and ``close()``;
    a message needs ``value()`` and ``error()``.
    """

    def __init__(
        self,
        source,
        aggregator: NearlineSignalAggregator,
        idle_exit_seconds: float = 0.0,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self.source = source
        self.aggregator = aggregator
        self.idle_exit_seconds = idle_exit_seconds
        self._clock = clock
        self.stats = Stats()
        self._uncommitted = 0

    def handle(self, value: bytes) -> None:
        try:
            event = to_interaction(value)
        except DecodeError as exc:
            self.stats.undecodable += 1
            log.warning("undecodable message skipped: %s", exc)
            return
        if event is not None and self.aggregator.process_interaction(event):
            self.stats.applied += 1
        else:
            self.stats.ignored += 1

    def _commit(self) -> None:
        if self._uncommitted:
            self.source.commit(asynchronous=False)
            self._uncommitted = 0

    def run(self, should_stop: Callable[[], bool] = lambda: False, poll_timeout: float = 1.0) -> Stats:
        """Consume until ``should_stop`` or, with ``idle_exit_seconds``, until the topic is drained."""
        last_message = self._clock()
        last_log = last_message
        try:
            while not should_stop():
                msg = self.source.poll(poll_timeout)
                now = self._clock()
                if msg is None:
                    self._commit()
                    if self.idle_exit_seconds and now - last_message >= self.idle_exit_seconds:
                        log.info("no message for %.0fs, exiting: %s", self.idle_exit_seconds, self.stats)
                        break
                    continue
                if msg.error():
                    log.warning("kafka error: %s", msg.error())
                    continue
                last_message = now
                self.handle(msg.value())
                self._uncommitted += 1
                if self._uncommitted >= COMMIT_EVERY:
                    self._commit()
                if now - last_log >= LOG_EVERY_SECONDS:
                    log.info("nearline progress: %s", self.stats)
                    last_log = now
            self._commit()
        finally:
            self.source.close()
        return self.stats


def make_kafka_source(settings: Settings):
    """A subscribed confluent-kafka Consumer: manual commits, group start per NEARLINE_START_OFFSET."""
    from confluent_kafka import Consumer  # noqa: PLC0415

    consumer = Consumer(
        {
            "bootstrap.servers": settings.kafka_brokers,
            "group.id": settings.nearline_consumer_group,
            "enable.auto.commit": False,
            "auto.offset.reset": settings.nearline_start_offset,
        }
    )
    consumer.subscribe([settings.kafka_analytics_topic])
    return consumer
