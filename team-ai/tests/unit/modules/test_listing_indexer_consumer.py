"""The listing.events consumer: envelope decode, idempotency, retry/DLQ, lifecycle.

Everything runs against in-memory fakes of the Kafka source/DLQ and a real
``KnowledgeRetrievalService`` (mock embeddings) where retrievability matters.
"""

from __future__ import annotations

import asyncio
from typing import Any

import pytest
from fastapi import FastAPI
from google.protobuf.timestamp_pb2 import Timestamp
from llama_index.core.embeddings import MockEmbedding

from app.bootstrap.resources import (
    ApplicationResources,
    validate_core_resource_requirements,
)
from app.core.redaction import RedactionPolicy
from app.modules.ai.rag.service import KnowledgeRetrievalService, build_rag_node_parser
from app.modules.messaging.indexer.consumer import ListingEventConsumer, RetryPolicy
from app.modules.messaging.indexer.decode import (
    UndecodableEventError,
    decode_listing_event,
)
from app.modules.messaging.indexer.factory import ListingIndexerAddon, supervise
from app.modules.messaging.indexer.handler import ListingEventIndexer
from app.transport.grpc._pb.platform.events.v1 import events_pb2
from app.transport.grpc._pb.platform.listing.v1 import listing_pb2
from tests.factories import build_test_settings


def envelope(
    listing_id: str,
    change: int,
    *,
    event_id: str = "ev-1",
    title: str = "Wireless Mouse",
    description: str = "rechargeable bluetooth mouse",
    status: int = listing_pb2.LISTING_STATUS_PUBLISHED,
    seconds: int = 1_000,
    type_: str = "platform.listing.v1.ListingChanged",
) -> bytes:
    changed = listing_pb2.ListingChanged(
        listing=listing_pb2.Listing(
            id=listing_id,
            title=title,
            description=description,
            price=250000,
            currency="VND",
            status=status,
            seller_id="seller-1",
            category_id="cat-electronics",
        ),
        change_type=change,
    )
    return events_pb2.EventEnvelope(
        event_id=event_id,
        type=type_,
        occurred_at=Timestamp(seconds=seconds),
        payload=changed.SerializeToString(),
    ).SerializeToString()


class Record:
    def __init__(self, value: bytes | None, key: bytes = b"k") -> None:
        self.key, self.value = key, value
        self.topic, self.partition, self.offset = "listing.events", 0, 0


class FakeSource:
    def __init__(self, records: list[Record]) -> None:
        self._records = list(records)
        self.committed: list[Record] = []
        self.stopped = False

    async def start(self) -> None: ...

    async def stop(self) -> None:
        self.stopped = True

    async def getone(self) -> Record:
        if not self._records:
            raise asyncio.CancelledError  # end of the scripted stream
        return self._records.pop(0)

    async def commit(self, record: Record) -> None:
        self.committed.append(record)


class FakeDLQ:
    def __init__(self, *, fail: bool = False) -> None:
        self.sent: list[tuple[bytes | None, bytes | None]] = []
        self.fail = fail

    async def start(self) -> None: ...
    async def stop(self) -> None: ...

    async def send(self, key: bytes | None, value: bytes | None) -> None:
        if self.fail:
            raise ConnectionError("dlq down")
        self.sent.append((key, value))


def real_rag() -> KnowledgeRetrievalService:
    return KnowledgeRetrievalService(
        embed_model=MockEmbedding(embed_dim=16),
        node_parser=build_rag_node_parser(chunk_size=128, chunk_overlap=8),
        redaction_policy=RedactionPolicy(mode="redacted"),
    )


async def no_sleep(_: float) -> None:
    return None


def consumer_for(
    records: list[Record], rag: Any, *, dlq: FakeDLQ | None = None, attempts: int = 3
) -> tuple[ListingEventConsumer, FakeSource, FakeDLQ]:
    source, sink = FakeSource(records), dlq or FakeDLQ()
    return (
        ListingEventConsumer(
            source=source,
            dead_letter=sink,
            indexer=ListingEventIndexer(rag_service=rag),
            retry=RetryPolicy(max_attempts=attempts, base_backoff_seconds=0),
            sleep=no_sleep,
        ),
        source,
        sink,
    )


async def run_to_end(consumer: ListingEventConsumer) -> None:
    with pytest.raises(asyncio.CancelledError):
        await consumer.run()


async def listing_ids(rag: KnowledgeRetrievalService, query: str) -> set[Any]:
    nodes = await rag.search(query, top_k=20)
    return {n.node.metadata.get("listing_id") for n in nodes}


# --- decode -------------------------------------------------------------------


def test_decode_maps_envelope_and_listing_fields() -> None:
    event = decode_listing_event(envelope("l-1", listing_pb2.CHANGE_TYPE_CREATED))
    assert event is not None
    assert (event.listing_id, event.action, event.status) == (
        "l-1",
        "CREATED",
        "PUBLISHED",
    )
    assert event.event_id == "ev-1"
    assert event.occurred_ns == 1_000 * 1_000_000_000
    assert (event.title, event.category, event.seller_id) == (
        "Wireless Mouse",
        "cat-electronics",
        "seller-1",
    )


def test_decode_ignores_other_event_types_and_rejects_garbage() -> None:
    other = envelope("l-1", 1, type_="platform.listing.v1.ListingStockChanged")
    assert decode_listing_event(other) is None
    with pytest.raises(UndecodableEventError):
        decode_listing_event(b"\xff\xff not a protobuf")


# --- scenarios: created is indexed / deleted is removed ------------------------


async def test_created_listing_becomes_retrievable_then_deleted_is_gone() -> None:
    rag = real_rag()
    consumer, source, dlq = consumer_for(
        [
            Record(envelope("l-1", listing_pb2.CHANGE_TYPE_CREATED, event_id="e1")),
            Record(
                envelope(
                    "l-1", listing_pb2.CHANGE_TYPE_DELETED, event_id="e2", seconds=2_000
                )
            ),
        ],
        rag,
    )
    # Run only the first record, observe, then the second.
    first = await source.getone()
    await consumer.process(first)
    assert "l-1" in await listing_ids(rag, "wireless mouse")

    await consumer.process(await source.getone())
    assert "l-1" not in await listing_ids(rag, "wireless mouse")
    assert dlq.sent == []


async def test_update_replaces_chunks_instead_of_appending() -> None:
    rag = real_rag()
    long_text = "bluetooth mouse " * 80  # several chunks
    consumer, _, _ = consumer_for([], rag)
    await consumer.process(
        Record(envelope("l-1", 1, event_id="e1", description=long_text))
    )
    before = len(rag.index_store.docstore.docs)
    await consumer.process(
        Record(envelope("l-1", 2, event_id="e2", description="short", seconds=2_000))
    )
    assert len(rag.index_store.docstore.docs) < before


async def test_unpublished_status_is_removed_from_rag() -> None:
    rag = real_rag()
    consumer, _, _ = consumer_for([], rag)
    await consumer.process(Record(envelope("l-1", 1, event_id="e1")))
    assert "l-1" in await listing_ids(rag, "mouse")
    await consumer.process(
        Record(
            envelope(
                "l-1",
                2,
                event_id="e2",
                status=listing_pb2.LISTING_STATUS_REJECTED,
                seconds=2_000,
            )
        )
    )
    assert "l-1" not in await listing_ids(rag, "mouse")


# --- idempotency ---------------------------------------------------------------


class CountingRag:
    def __init__(self) -> None:
        self.indexed: list[Any] = []
        self.deleted: list[str] = []

    async def index(self, documents: list[Any]) -> dict[str, int]:
        self.indexed.extend(documents)
        return {"chunk_count": len(documents)}

    async def delete(self, document_id: str) -> None:
        self.deleted.append(document_id)


async def test_redelivered_event_is_applied_once() -> None:
    rag = CountingRag()
    consumer, _, _ = consumer_for([], rag)
    record = Record(envelope("l-1", 1, event_id="same"))
    await consumer.process(record)
    await consumer.process(record)  # at-least-once redelivery
    assert len(rag.indexed) == 1


async def test_event_older_than_applied_one_is_skipped() -> None:
    rag = CountingRag()
    consumer, _, _ = consumer_for([], rag)
    await consumer.process(Record(envelope("l-1", 2, event_id="new", seconds=2_000)))
    await consumer.process(
        Record(envelope("l-1", 3, event_id="old-delete", seconds=1_000))
    )
    assert len(rag.indexed) == 1
    assert rag.deleted == ["l-1"]  # only the replace-delete of the applied update


# --- failure handling ------------------------------------------------------------


class FlakyRag(CountingRag):
    def __init__(self, failures: int) -> None:
        super().__init__()
        self.failures = failures

    async def index(self, documents: list[Any]) -> dict[str, int]:
        if self.failures > 0:
            self.failures -= 1
            raise ConnectionError("embed server down")
        return await super().index(documents)


async def test_transient_failure_is_retried_then_committed() -> None:
    rag = FlakyRag(failures=2)
    consumer, source, dlq = consumer_for([Record(envelope("l-1", 1))], rag, attempts=3)
    await run_to_end(consumer)
    assert len(rag.indexed) == 1
    assert len(source.committed) == 1 and dlq.sent == []


async def test_exhausted_retries_park_on_dlq_and_stream_continues() -> None:
    rag = FlakyRag(failures=99)
    bad = Record(envelope("l-1", 1, event_id="bad"))
    consumer, source, dlq = consumer_for([bad], rag, attempts=3)
    await run_to_end(consumer)
    assert dlq.sent == [(bad.key, bad.value)]
    assert source.committed == [bad]  # parked, so the offset moves on


async def test_undecodable_record_goes_straight_to_dlq() -> None:
    rag = CountingRag()
    poison = Record(b"\xff\xff garbage")
    consumer, source, dlq = consumer_for([poison], rag)
    await run_to_end(consumer)
    assert dlq.sent == [(poison.key, poison.value)] and rag.indexed == []
    assert source.committed == [poison]


async def test_unparkable_record_is_not_committed() -> None:
    rag = FlakyRag(failures=99)
    consumer, source, _ = consumer_for(
        [Record(envelope("l-1", 1))], rag, dlq=FakeDLQ(fail=True), attempts=2
    )
    with pytest.raises(ConnectionError):
        await consumer.run()
    assert source.committed == []  # redelivered after restart, not lost


async def test_other_listing_event_types_are_committed_and_ignored() -> None:
    rag = CountingRag()
    stock = Record(envelope("l-1", 1, type_="platform.listing.v1.ListingStockChanged"))
    consumer, source, dlq = consumer_for([stock], rag)
    await run_to_end(consumer)
    assert rag.indexed == [] and dlq.sent == [] and source.committed == [stock]


# --- lifecycle -------------------------------------------------------------------


async def test_supervisor_restarts_a_failed_consumer() -> None:
    built = 0

    class Boom:
        async def run(self) -> None:
            nonlocal built
            built += 1
            if built < 3:
                raise ConnectionError("broker gone")
            await asyncio.Event().wait()

    task = asyncio.create_task(supervise(lambda: Boom(), restart_delay=0))  # type: ignore[arg-type,return-value]
    for _ in range(50):
        if built >= 3:
            break
        await asyncio.sleep(0)
    task.cancel()
    await asyncio.gather(task, return_exceptions=True)
    assert built == 3


def test_indexer_requires_rag() -> None:
    settings = build_test_settings(LISTING_INDEXER_ENABLED=True, RAG_ENABLED=False)
    with pytest.raises(RuntimeError, match="RAG_ENABLED"):
        validate_core_resource_requirements(settings=settings, init_resources=True)


async def test_addon_is_gated_and_starts_and_stops_the_consumer(monkeypatch) -> None:
    off = build_test_settings()
    addon = ListingIndexerAddon()
    assert addon.is_enabled(off) is False

    on = build_test_settings(LISTING_INDEXER_ENABLED=True, RAG_ENABLED=True)
    assert addon.is_enabled(on) is True
    started: list[str] = []

    class Stub:
        async def run(self) -> None:
            started.append("run")
            await asyncio.Event().wait()

    monkeypatch.setattr(
        "app.modules.messaging.indexer.factory.build_listing_consumer",
        lambda settings, rag_service: Stub(),
    )
    resources = ApplicationResources(rag_service=real_rag())
    await addon.open(FastAPI(), resources, on)
    await asyncio.sleep(0)
    assert started == ["run"]
    await addon.close(FastAPI(), resources)
    assert addon._task is None


async def test_default_build_uses_aiokafka_with_manual_commit_and_dlq_topic() -> None:
    """The real Kafka adapters construct (no broker needed) and are configured per spec."""
    from app.modules.messaging.indexer.consumer import (
        AIOKafkaDeadLetter,
        AIOKafkaSource,
    )
    from app.modules.messaging.indexer.factory import build_listing_consumer

    settings = build_test_settings(
        LISTING_INDEXER_ENABLED=True, RAG_ENABLED=True, KAFKA_BROKERS="redpanda:9092"
    )
    consumer = build_listing_consumer(settings, rag_service=CountingRag())
    assert isinstance(consumer._source, AIOKafkaSource)
    assert isinstance(consumer._dlq, AIOKafkaDeadLetter)
    assert consumer._dlq._topic == "listing.events.dlq"
    assert consumer._source._consumer._enable_auto_commit is False
