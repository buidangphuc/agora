"""At-least-once Kafka consumption with bounded retry and a dead-letter topic.

An offset is committed only after the record was handled, or parked on the
dead-letter topic once it proved non-transient or exhausted its retries. When a
record can be neither handled nor parked (DLQ unreachable) the loop stops WITHOUT
committing, so the record is redelivered after a restart instead of lost.
"""

import asyncio
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import Protocol

from loguru import logger

from app.core.resilience import is_transient_error


@dataclass(frozen=True, slots=True)
class KafkaRecord:
    topic: str
    partition: int
    offset: int
    key: bytes | None
    value: bytes


class RecordSource(Protocol):
    async def start(self) -> None: ...

    async def stop(self) -> None: ...

    async def getone(self) -> KafkaRecord: ...

    async def commit(self, record: KafkaRecord) -> None: ...


class DeadLetterSink(Protocol):
    async def start(self) -> None: ...

    async def stop(self) -> None: ...

    async def send(self, record: KafkaRecord, *, error_type: str) -> None: ...


class RetryingConsumer:
    def __init__(
        self,
        source: RecordSource,
        dlq: DeadLetterSink,
        handle: Callable[[KafkaRecord], Awaitable[None]],
        *,
        max_attempts: int,
        base_backoff_seconds: float,
        max_backoff_seconds: float = 5.0,
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
    ) -> None:
        self._source = source
        self._dlq = dlq
        self._handle = handle
        self._max_attempts = max_attempts
        self._base_backoff_seconds = base_backoff_seconds
        self._max_backoff_seconds = max_backoff_seconds
        self._sleep = sleep

    async def run(self) -> None:
        """Consume until cancelled; raises when a record can be neither handled nor parked."""
        try:
            await self._source.start()
            await self._dlq.start()
            while True:
                record = await self._source.getone()
                await self._process(record)
                await self._source.commit(record)
        finally:
            try:
                await self._source.stop()
            finally:
                await self._dlq.stop()

    async def _process(self, record: KafkaRecord) -> None:
        """Return normally only when the record is safe to commit."""
        for attempt in range(1, self._max_attempts + 1):
            try:
                await self._handle(record)
                return
            except Exception as exc:
                logger.warning(
                    "kafka.attempt_failed topic={} offset={} attempt={}/{} err={}",
                    record.topic,
                    record.offset,
                    attempt,
                    self._max_attempts,
                    exc,
                )
                if not is_transient_error(exc) or attempt == self._max_attempts:
                    await self._dlq.send(record, error_type=type(exc).__name__)
                    return
                await self._sleep(
                    min(
                        self._max_backoff_seconds,
                        self._base_backoff_seconds * 2 ** (attempt - 1),
                    )
                )


async def supervise(
    build: Callable[[], RetryingConsumer], *, restart_delay_seconds: float = 5.0
) -> None:
    """Rebuild and rerun the consumer after any crash, until cancelled."""
    while True:
        try:
            await build().run()
        except Exception:
            logger.exception("kafka.consumer_crashed")
        await asyncio.sleep(restart_delay_seconds)


class _AIOKafkaSource:
    """``RecordSource`` over aiokafka; manual per-record commits."""

    def __init__(self, *, brokers: str, topic: str, group: str) -> None:
        from aiokafka import AIOKafkaConsumer

        self._consumer = AIOKafkaConsumer(
            topic,
            bootstrap_servers=brokers,
            group_id=group,
            enable_auto_commit=False,
            auto_offset_reset="earliest",
        )

    async def start(self) -> None:
        await self._consumer.start()

    async def stop(self) -> None:
        await self._consumer.stop()

    async def getone(self) -> KafkaRecord:
        message = await self._consumer.getone()
        return KafkaRecord(
            topic=message.topic,
            partition=message.partition,
            offset=message.offset,
            key=message.key,
            value=message.value or b"",
        )

    async def commit(self, record: KafkaRecord) -> None:
        from aiokafka import TopicPartition

        await self._consumer.commit(
            {TopicPartition(record.topic, record.partition): record.offset + 1}
        )


class _AIOKafkaDeadLetter:
    def __init__(self, *, brokers: str) -> None:
        from aiokafka import AIOKafkaProducer

        self._producer = AIOKafkaProducer(bootstrap_servers=brokers)

    async def start(self) -> None:
        await self._producer.start()

    async def stop(self) -> None:
        await self._producer.stop()

    async def send(self, record: KafkaRecord, *, error_type: str) -> None:
        await self._producer.send_and_wait(
            f"{record.topic}.dlq",
            value=record.value,
            key=record.key,
            headers=[("x-error-type", error_type.encode())],
        )


def build_aiokafka_consumer(*, brokers: str, topic: str, group: str) -> RecordSource:
    return _AIOKafkaSource(brokers=brokers, topic=topic, group=group)


def build_aiokafka_dlq(*, brokers: str) -> DeadLetterSink:
    return _AIOKafkaDeadLetter(brokers=brokers)
