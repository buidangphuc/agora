"""The Kafka consumer loop (recsys.nearline.consumer) over a fake confluent-kafka source."""

from __future__ import annotations

import pytest

from recsys.nearline.consumer import COMMIT_EVERY, NearlineConsumer
from recsys.nearline.signals import NearlineSignalAggregator, NearlineSignalStore
from tests.test_nearline_wire import GOLDEN_ANON_CLICK, GOLDEN_OTHER_TYPE


class Msg:
    def __init__(self, value: bytes, error=None):
        self._value, self._error = value, error

    def value(self):
        return self._value

    def error(self):
        return self._error


class FakeSource:
    def __init__(self, messages):
        self.messages = list(messages)
        self.commits = 0
        self.closed = False

    def poll(self, timeout):
        return self.messages.pop(0) if self.messages else None

    def commit(self, asynchronous=False):
        self.commits += 1

    def close(self):
        self.closed = True


class Clock:
    """Each call advances one second, so an idle consumer reaches its idle limit."""

    def __init__(self):
        self.t = 0.0

    def __call__(self):
        self.t += 1.0
        return self.t


def _consumer(messages, idle=3):
    store = NearlineSignalStore()
    # the golden events carry old timestamps: a wide window keeps them applicable
    agg = NearlineSignalAggregator(store, ttl_seconds=10**12)
    source = FakeSource(messages)
    return NearlineConsumer(source, agg, idle_exit_seconds=idle, clock=Clock()), store, source


def test_consumer_applies_tracking_events_and_drains():
    consumer, store, source = _consumer([Msg(GOLDEN_ANON_CLICK)])
    stats = consumer.run()
    assert stats.applied == 1
    assert store.get_recent_items("anon:anon-9") == ["listing-B"]
    assert source.commits == 1 and source.closed  # committed after applying, closed on exit


def test_other_envelopes_and_garbage_are_skipped_not_fatal():
    consumer, store, source = _consumer([Msg(GOLDEN_OTHER_TYPE), Msg(b"\x0a\x05ab"), Msg(GOLDEN_ANON_CLICK)])
    stats = consumer.run()
    assert (stats.applied, stats.ignored, stats.undecodable) == (1, 1, 1)


def test_broker_errors_do_not_stop_the_loop():
    consumer, store, _ = _consumer([Msg(b"", error="boom"), Msg(GOLDEN_ANON_CLICK)])
    assert consumer.run().applied == 1


def test_offsets_are_committed_in_batches():
    n = COMMIT_EVERY + 5
    consumer, _, source = _consumer([Msg(GOLDEN_OTHER_TYPE)] * n)
    consumer.run()
    assert source.commits == 2  # one full batch, then the remainder when drained


def test_a_redis_failure_stops_the_consumer_without_committing():
    class Boom(NearlineSignalAggregator):
        def process_interaction(self, event):
            raise ConnectionError("redis down")

    source = FakeSource([Msg(GOLDEN_ANON_CLICK)])
    consumer = NearlineConsumer(source, Boom(NearlineSignalStore()), clock=Clock())
    with pytest.raises(ConnectionError):
        consumer.run()
    assert source.commits == 0 and source.closed
