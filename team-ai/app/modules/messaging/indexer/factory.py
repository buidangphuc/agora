"""Bootstrap addon: run the ``listing.events`` -> RAG consumer for the app's lifetime.

Registered on every boot, opened only when ``LISTING_INDEXER_ENABLED=true`` (which
requires ``RAG_ENABLED``: the indexer writes into the RAG service). The consumer runs
as a supervised background task in the same process as the API (the service has no
separate worker for events); if it dies it is restarted after a delay, and its own
retry/DLQ policy decides what a bad record does, so one poison event never stalls it.
"""

from __future__ import annotations

import asyncio
from typing import TYPE_CHECKING

from loguru import logger

from app.modules.messaging.indexer.consumer import (
    AIOKafkaDeadLetter,
    AIOKafkaSource,
    DeadLetterSink,
    ListingEventConsumer,
    RecordSource,
    RetryPolicy,
)
from app.modules.messaging.indexer.handler import ListingEventIndexer

if TYPE_CHECKING:
    from collections.abc import Callable

    from fastapi import FastAPI

    from app.bootstrap.resources import ApplicationResources
    from app.core.config import Settings

_RESTART_DELAY_SECONDS = 5.0


def build_listing_consumer(
    settings: Settings,
    *,
    rag_service: object,
    source: RecordSource | None = None,
    dead_letter: DeadLetterSink | None = None,
) -> ListingEventConsumer:
    """Compose the consumer; ``source``/``dead_letter`` default to aiokafka."""
    topic = settings.LISTING_INDEXER_TOPIC
    brokers = settings.KAFKA_BROKERS
    return ListingEventConsumer(
        source=source
        or AIOKafkaSource(
            topic=topic, brokers=brokers, group_id=settings.LISTING_INDEXER_GROUP
        ),
        dead_letter=dead_letter
        or AIOKafkaDeadLetter(topic=f"{topic}.dlq", brokers=brokers),
        indexer=ListingEventIndexer(rag_service=rag_service),
        retry=RetryPolicy(
            max_attempts=settings.LISTING_INDEXER_MAX_ATTEMPTS,
            base_backoff_seconds=settings.LISTING_INDEXER_BASE_BACKOFF_SECONDS,
        ),
    )


async def supervise(
    make_consumer: Callable[[], ListingEventConsumer],
    *,
    restart_delay: float = _RESTART_DELAY_SECONDS,
) -> None:
    """Run the consumer forever, rebuilding it after a failure; stops on cancellation."""
    while True:
        try:
            await make_consumer().run()
        except asyncio.CancelledError:
            raise
        except Exception as exc:
            logger.error(
                "indexer.consumer_stopped err={} restart_in={}s", exc, restart_delay
            )
            await asyncio.sleep(restart_delay)


class ListingIndexerAddon:
    name = "listing_indexer"

    def __init__(self) -> None:
        self._task: asyncio.Task[None] | None = None

    def is_enabled(self, settings: Settings) -> bool:
        return settings.LISTING_INDEXER_ENABLED

    async def open(
        self,
        app: FastAPI,
        resources: ApplicationResources,
        settings: Settings,
    ) -> None:
        rag = resources.rag_service
        if rag is None:
            raise RuntimeError("LISTING_INDEXER_ENABLED requires RAG_ENABLED")
        self._task = asyncio.create_task(
            supervise(lambda: build_listing_consumer(settings, rag_service=rag)),
            name="listing-indexer",
        )
        logger.info(
            "indexer.started topic={} group={}",
            settings.LISTING_INDEXER_TOPIC,
            settings.LISTING_INDEXER_GROUP,
        )

    async def close(self, app: FastAPI, resources: ApplicationResources) -> None:
        task, self._task = self._task, None
        if task is None:
            return
        task.cancel()
        await asyncio.gather(task, return_exceptions=True)
