import asyncio
from collections.abc import Sequence

import pytest
from llama_index.core import Document

from app.modules.messaging.kafka.consumer import (
    KafkaRecord,
    RetryingConsumer,
    build_aiokafka_consumer,
    build_aiokafka_dlq,
    supervise,
)
from app.modules.messaging.listing_indexer.decode import decode_listing_event
from app.modules.messaging.listing_indexer.handler import ListingEventIndexer
from tests.unit.modules.listing_indexer.test_decode import envelope


def record(value: bytes, offset: int = 0) -> KafkaRecord:
    return KafkaRecord("listing.events", 0, offset, b"k", value)


class FakeSource:
    def __init__(self, records: list[KafkaRecord], *, fail_start: bool = False) -> None:
        self._records = list(records)
        self.fail_start = fail_start
        self.committed: list[KafkaRecord] = []
        self.stopped = False

    async def start(self) -> None:
        if self.fail_start:
            raise ConnectionError("broker down")

    async def stop(self) -> None:
        self.stopped = True

    async def getone(self) -> KafkaRecord:
        if not self._records:
            raise asyncio.CancelledError  # end of the scripted stream
        return self._records.pop(0)

    async def commit(self, record: KafkaRecord) -> None:
        self.committed.append(record)


class FakeDlq:
    def __init__(self, *, fail_send: bool = False) -> None:
        self.sent: list[tuple[KafkaRecord, str]] = []
        self.fail_send = fail_send
        self.stopped = False

    async def start(self) -> None: ...

    async def stop(self) -> None:
        self.stopped = True

    async def send(self, record: KafkaRecord, *, error_type: str) -> None:
        if self.fail_send:
            raise ConnectionError("dlq down")
        self.sent.append((record, error_type))


class FakeRag:
    def __init__(self, failures: int = 0, error: Exception | None = None) -> None:
        self.indexed: list[Document] = []
        self.failures = failures
        self.error = error or ConnectionError("embed server down")

    async def index(self, documents: Sequence[Document]) -> dict[str, int]:
        if self.failures:
            self.failures -= 1
            raise self.error
        self.indexed.extend(documents)
        return {"chunk_count": len(documents)}

    async def delete(self, document_id: str) -> None: ...


async def no_sleep(_: float) -> None:
    return None


def build(
    records: list[KafkaRecord],
    rag: FakeRag,
    *,
    dlq: FakeDlq | None = None,
    source: FakeSource | None = None,
    attempts: int = 3,
    sleeps: list[float] | None = None,
) -> tuple[RetryingConsumer, FakeSource, FakeDlq]:
    source = source or FakeSource(records)
    sink = dlq or FakeDlq()
    indexer = ListingEventIndexer(rag)

    async def handle(rec: KafkaRecord) -> None:
        payload = decode_listing_event(rec.value)
        if payload is not None:
            await indexer.handle(payload)

    async def sleep(seconds: float) -> None:
        if sleeps is not None:
            sleeps.append(seconds)

    consumer = RetryingConsumer(
        source,
        sink,
        handle,
        max_attempts=attempts,
        base_backoff_seconds=1.0,
        sleep=sleep,
    )
    return consumer, source, sink


async def run_to_end(consumer: RetryingConsumer) -> None:
    with pytest.raises(asyncio.CancelledError):
        await consumer.run()


async def test_transient_failure_then_success_applies_and_commits_once() -> None:
    rag = FakeRag(failures=2)
    rec = record(envelope("l-1"))
    sleeps: list[float] = []
    consumer, source, dlq = build([rec], rag, sleeps=sleeps)
    await run_to_end(consumer)
    assert len(rag.indexed) == 1
    assert source.committed == [rec] and dlq.sent == []
    assert sleeps == [1.0, 2.0]


async def test_backoff_is_capped() -> None:
    rag = FakeRag(failures=99)
    sleeps: list[float] = []
    consumer, _, _ = build([record(envelope("l-1"))], rag, attempts=6, sleeps=sleeps)
    await run_to_end(consumer)
    assert sleeps == [1.0, 2.0, 4.0, 5.0, 5.0]


async def test_non_transient_failure_goes_to_dlq_at_once_and_is_committed() -> None:
    rag = FakeRag(failures=99, error=ValueError("bad document"))
    rec = record(envelope("l-1"))
    sleeps: list[float] = []
    consumer, source, dlq = build([rec], rag, sleeps=sleeps)
    await run_to_end(consumer)
    assert dlq.sent == [(rec, "ValueError")] and sleeps == []
    assert source.committed == [rec]


async def test_exhausted_retries_park_on_dlq_and_stream_continues() -> None:
    rag = FakeRag(failures=3)
    bad, good = (
        record(envelope("l-1", event_id="bad")),
        record(envelope("l-2", event_id="ok"), offset=1),
    )
    consumer, source, dlq = build([bad, good], rag)
    await run_to_end(consumer)
    assert dlq.sent == [(bad, "ConnectionError")]
    assert source.committed == [bad, good]
    assert [d.id_ for d in rag.indexed] == ["l-2"]


async def test_undecodable_record_goes_to_dlq_without_retries() -> None:
    rec = record(b"\xff\xff garbage")
    sleeps: list[float] = []
    consumer, source, dlq = build([rec], FakeRag(), sleeps=sleeps)
    await run_to_end(consumer)
    assert dlq.sent == [(rec, "UndecodableEventError")] and sleeps == []
    assert source.committed == [rec]


async def test_foreign_event_type_is_committed_without_indexing() -> None:
    rag = FakeRag()
    rec = record(envelope("l-1", type_="platform.order.v1.OrderPlaced"))
    consumer, source, dlq = build([rec], rag)
    await run_to_end(consumer)
    assert rag.indexed == [] and dlq.sent == [] and source.committed == [rec]


async def test_dlq_send_failure_does_not_commit_and_propagates() -> None:
    rag = FakeRag(failures=99)
    consumer, source, _ = build(
        [record(envelope("l-1"))], rag, dlq=FakeDlq(fail_send=True), attempts=2
    )
    with pytest.raises(ConnectionError):
        await consumer.run()
    assert source.committed == []
    assert source.stopped


async def test_stops_source_and_dlq_when_startup_fails() -> None:
    source = FakeSource([], fail_start=True)
    consumer, _, dlq = build([], FakeRag(), source=source)
    with pytest.raises(ConnectionError):
        await consumer.run()
    assert source.stopped and dlq.stopped


async def test_supervisor_rebuilds_a_crashed_consumer_until_cancelled() -> None:
    built = 0

    class Boom:
        async def run(self) -> None:
            nonlocal built
            built += 1
            if built < 3:
                raise ConnectionError("broker gone")
            await asyncio.Event().wait()

    task = asyncio.create_task(
        supervise(lambda: Boom(), restart_delay_seconds=0)  # type: ignore[arg-type,return-value]
    )
    for _ in range(50):
        if built >= 3:
            break
        await asyncio.sleep(0)
    task.cancel()
    await asyncio.gather(task, return_exceptions=True)
    assert built == 3 and task.cancelled()


async def test_aiokafka_adapters_are_configured_per_spec() -> None:
    pytest.importorskip("aiokafka")
    source = build_aiokafka_consumer(brokers="redpanda:9092", topic="t", group="g")
    consumer = source._consumer  # type: ignore[attr-defined]
    assert consumer._enable_auto_commit is False
    assert consumer._group_id == "g"
    assert build_aiokafka_dlq(brokers="redpanda:9092") is not None
