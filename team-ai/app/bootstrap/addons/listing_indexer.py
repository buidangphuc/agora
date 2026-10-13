"""Lifecycle addon for the listing indexer: supervises the Kafka consumer for the app's lifetime."""

from __future__ import annotations

import asyncio
import contextlib
from collections.abc import Callable
from contextlib import AsyncExitStack
from functools import partial

from app.bootstrap.resources import ApplicationResources, opened
from app.core.config import Settings
from app.modules.messaging.kafka.consumer import (
    DeadLetterSink,
    KafkaRecord,
    RecordSource,
    RetryingConsumer,
    build_aiokafka_consumer,
    build_aiokafka_dlq,
    supervise,
)
from app.modules.messaging.listing_indexer.decode import decode_listing_event
from app.modules.messaging.listing_indexer.handler import ListingEventIndexer


async def _cancel(task: asyncio.Task[None]) -> None:
    task.cancel()
    with contextlib.suppress(asyncio.CancelledError):
        await task


class ListingIndexerAddon:
    name = "listing_indexer"

    def __init__(
        self,
        *,
        build_source: Callable[..., RecordSource] = build_aiokafka_consumer,
        build_dlq: Callable[..., DeadLetterSink] = build_aiokafka_dlq,
    ) -> None:
        self._build_source = build_source
        self._build_dlq = build_dlq

    def is_enabled(self, settings: Settings) -> bool:
        return settings.RAG_ENABLED

    async def open(
        self,
        resources: ApplicationResources,
        settings: Settings,
        stack: AsyncExitStack,
    ) -> None:
        indexer = ListingEventIndexer(opened(resources.rag_service, "rag"))

        async def handle(record: KafkaRecord) -> None:
            payload = decode_listing_event(record.value)
            if payload is not None:
                await indexer.handle(payload)

        def build() -> RetryingConsumer:
            return RetryingConsumer(
                self._build_source(
                    brokers=settings.KAFKA_BROKERS,
                    topic=settings.LISTING_INDEXER_TOPIC,
                    group=settings.LISTING_INDEXER_GROUP,
                ),
                self._build_dlq(brokers=settings.KAFKA_BROKERS),
                handle,
                max_attempts=settings.LISTING_INDEXER_MAX_ATTEMPTS,
                base_backoff_seconds=settings.LISTING_INDEXER_BASE_BACKOFF_SECONDS,
            )

        task = asyncio.create_task(supervise(build), name="listing-indexer")
        stack.push_async_callback(partial(_cancel, task))
