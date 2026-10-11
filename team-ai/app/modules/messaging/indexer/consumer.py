"""Kafka consumer loop for ``listing.events`` -> RAG index.

At-least-once with manual commits: an offset is committed only after the record was
applied, or parked on the dead-letter topic (``<topic>.dlq``) once it exhausted its
retries or proved undecodable. If a record can neither be applied nor parked (DLQ
unreachable) the loop stops WITHOUT committing, so the record is redelivered after a
restart/reconnect rather than lost. Mirrors team-search's consumer (AD1).
"""

from __future__ import annotations

import asyncio
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import Any, Protocol

from loguru import logger

from app.modules.messaging.indexer.decode import (
    UndecodableEventError,
    decode_listing_event,
)
from app.modules.messaging.indexer.handler import ListingEventIndexer


class KafkaRecord(Protocol):
    key: bytes | None
    value: bytes | None


class RecordSource(Protocol):
    async def start(self) -> None: ...
    async def stop(self) -> None: ...
    async def getone(self) -> Any: ...
    async def commit(self, record: Any) -> None: ...


class DeadLetterSink(Protocol):
    async def start(self) -> None: ...
    async def stop(self) -> None: ...
    async def send(self, key: bytes | None, value: bytes | None) -> None: ...


@dataclass(frozen=True)
class RetryPolicy:
    max_attempts: int = 5
    base_backoff_seconds: float = 0.1
    max_backoff_seconds: float = 5.0

    def backoff(self, attempt: int) -> float:
        """Delay before the next attempt (``attempt`` is 1-based): doubles, capped."""
        return min(
            self.max_backoff_seconds, self.base_backoff_seconds * 2 ** (attempt - 1)
        )


class ListingEventConsumer:
    def __init__(
        self,
        *,
        source: RecordSource,
        dead_letter: DeadLetterSink,
        indexer: ListingEventIndexer,
        retry: RetryPolicy | None = None,
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
    ) -> None:
        self._source = source
        self._dlq = dead_letter
        self._indexer = indexer
        self._retry = retry or RetryPolicy()
        self._sleep = sleep

    async def run(self) -> None:
        """Consume until cancelled; raises if a record could be neither applied nor parked."""
        await self._source.start()
        await self._dlq.start()
        try:
            while True:
                record = await self._source.getone()
                await self.process(record)
                await self._source.commit(record)
        finally:
            await self._source.stop()
            await self._dlq.stop()

    async def process(self, record: Any) -> None:
        """Apply one record, or park it. Returns normally only when it is safe to commit."""
        value: bytes = record.value or b""
        try:
            event = decode_listing_event(value)
        except UndecodableEventError as exc:
            logger.error("indexer.undecodable err={}", exc)
            await self._dlq.send(record.key, record.value)
            return
        if event is None:
            return  # a listing event type the RAG index does not consume
        last_exc: Exception | None = None
        for attempt in range(1, self._retry.max_attempts + 1):
            try:
                await self._indexer.handle_event(event)
                return
            except Exception as exc:
                last_exc = exc
                logger.warning(
                    "indexer.attempt_failed listing_id={} attempt={}/{} err={}",
                    event.listing_id,
                    attempt,
                    self._retry.max_attempts,
                    exc,
                )
                if attempt < self._retry.max_attempts:
                    await self._sleep(self._retry.backoff(attempt))
        logger.error("indexer.parked listing_id={} err={}", event.listing_id, last_exc)
        await self._dlq.send(record.key, record.value)


class AIOKafkaSource:
    """``RecordSource`` over aiokafka (the ``kafka`` extra); manual per-record commits."""

    def __init__(self, *, topic: str, brokers: str, group_id: str) -> None:
        from aiokafka import AIOKafkaConsumer

        self._consumer = AIOKafkaConsumer(
            topic,
            bootstrap_servers=brokers,
            group_id=group_id,
            enable_auto_commit=False,
            auto_offset_reset="earliest",
        )

    async def start(self) -> None:
        await self._consumer.start()

    async def stop(self) -> None:
        await self._consumer.stop()

    async def getone(self) -> Any:
        return await self._consumer.getone()

    async def commit(self, record: Any) -> None:
        from aiokafka import TopicPartition

        await self._consumer.commit(
            {TopicPartition(record.topic, record.partition): record.offset + 1}
        )


class AIOKafkaDeadLetter:
    def __init__(self, *, topic: str, brokers: str) -> None:
        from aiokafka import AIOKafkaProducer

        self._topic = topic
        self._producer = AIOKafkaProducer(bootstrap_servers=brokers)

    async def start(self) -> None:
        await self._producer.start()

    async def stop(self) -> None:
        await self._producer.stop()

    async def send(self, key: bytes | None, value: bytes | None) -> None:
        await self._producer.send_and_wait(self._topic, value=value, key=key)
