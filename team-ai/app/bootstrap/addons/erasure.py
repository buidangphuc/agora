"""Lifecycle addon for user erasure: supervises the identity.events consumer."""

from __future__ import annotations

import asyncio
import contextlib
from collections.abc import Callable
from contextlib import AsyncExitStack
from functools import partial

from app.bootstrap.resources import ApplicationResources, opened
from app.core.config import Settings
from app.modules.messaging.erasure.consumer import ErasureConsumer, build_topic_dlq
from app.modules.messaging.kafka.consumer import (
    DeadLetterSink,
    RecordSource,
    build_aiokafka_consumer,
    supervise,
)

CHAT_SESSION_PREFIX = "chat:session"


async def _cancel(task: asyncio.Task[None]) -> None:
    task.cancel()
    with contextlib.suppress(asyncio.CancelledError):
        await task


class ErasureAddon:
    name = "erasure"

    def __init__(
        self,
        *,
        build_source: Callable[..., RecordSource] = build_aiokafka_consumer,
        build_dlq: Callable[..., DeadLetterSink] = build_topic_dlq,
    ) -> None:
        self._build_source = build_source
        self._build_dlq = build_dlq

    def is_enabled(self, settings: Settings) -> bool:
        return True

    async def open(
        self,
        resources: ApplicationResources,
        settings: Settings,
        stack: AsyncExitStack,
    ) -> None:
        redis = opened(resources.redis, "redis")

        def build() -> ErasureConsumer:
            return ErasureConsumer(
                self._build_source(
                    brokers=settings.KAFKA_BROKERS,
                    topic=settings.ERASURE_TOPIC,
                    group=settings.ERASURE_GROUP,
                ),
                self._build_dlq(
                    brokers=settings.KAFKA_BROKERS, topic=settings.ERASURE_DLQ_TOPIC
                ),
                redis,
                chat_prefix=CHAT_SESSION_PREFIX,
                quota_prefix=settings.QUOTA_REDIS_PREFIX,
                rate_limit_prefix=settings.RATE_LIMIT_REDIS_PREFIX,
            )

        task = asyncio.create_task(supervise(build), name="user-erasure")  # type: ignore[arg-type]
        stack.push_async_callback(partial(_cancel, task))
