"""Erase a user's chat sessions, quota counters and rate-limit keys on ``UserErased``.

The offset is committed only after the deletions (or the DLQ send). A Redis failure
is retried on the same record and never commits or parks it. Logs carry only a
fingerprint of the user id.
"""

import asyncio
import hashlib
import re
from collections.abc import Awaitable, Callable
from typing import Any
from urllib.parse import quote

from loguru import logger
from redis.exceptions import RedisError

from app.modules.messaging.erasure.decode import (
    UndecodableEventError,
    decode_user_erased,
)
from app.modules.messaging.kafka.consumer import (
    DeadLetterSink,
    KafkaRecord,
    RecordSource,
)

_GLOB_SPECIAL = re.compile(r"([\\*?\[\]])")
_DELETE_BATCH = 500


def fingerprint(user_id: str) -> str:
    return hashlib.sha256(user_id.encode()).hexdigest()[:12]


def _glob(text: str) -> str:
    return _GLOB_SPECIAL.sub(r"\\\1", text)


class ErasureConsumer:
    def __init__(
        self,
        source: RecordSource,
        dlq: DeadLetterSink,
        redis: Any,
        *,
        chat_prefix: str,
        quota_prefix: str,
        rate_limit_prefix: str,
        base_backoff_seconds: float = 0.5,
        max_backoff_seconds: float = 5.0,
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
    ) -> None:
        self._source = source
        self._dlq = dlq
        self._redis = redis
        self._chat_prefix = chat_prefix
        self._quota_prefix = quota_prefix
        self._rate_limit_prefix = rate_limit_prefix
        self._base = base_backoff_seconds
        self._max = max_backoff_seconds
        self._sleep = sleep

    async def run(self) -> None:
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
        try:
            user_id = decode_user_erased(record.value)
        except UndecodableEventError as exc:
            logger.warning(
                "erasure.undecodable topic={} offset={} err={}",
                record.topic,
                record.offset,
                type(exc).__name__,
            )
            await self._dlq.send(record, error_type=type(exc).__name__)
            return
        if user_id is None:
            return
        fp = fingerprint(user_id)
        attempt = 0
        while True:
            try:
                removed = await self._erase(user_id)
            except (RedisError, OSError) as exc:
                attempt += 1
                logger.warning(
                    "erasure.redis_failed user={} attempt={} err={}",
                    fp,
                    attempt,
                    type(exc).__name__,
                )
                await self._sleep(min(self._max, self._base * 2 ** (attempt - 1)))
                continue
            logger.info("erasure.done user={} keys={}", fp, removed)
            return

    async def _erase(self, user_id: str) -> int:
        uid = re.escape(user_id)
        chat = f"{self._chat_prefix}:{quote(user_id, safe='')}:"
        quota = self._quota_prefix
        rate = self._rate_limit_prefix
        targets = [
            (
                f"{_glob(chat)}*",
                lambda k: k.startswith(chat) and k.count(":") == chat.count(":"),
            ),
            (
                f"{_glob(quota)}:?:user:{_glob(user_id)}:*",
                re.compile(
                    rf"{re.escape(quota)}:(?:c:user:{uid}:[^:]+:[^:]+|i:user:{uid}:[^:]+:[^:]+:[^:]+)"
                ).fullmatch,
            ),
            (
                f"{_glob(rate)}:*user:{_glob(user_id)}*",
                re.compile(rf"{re.escape(rate)}:(?:.*:)?user:{uid}(?::\d+)?").fullmatch,
            ),
        ]
        removed = 0
        for pattern, accept in targets:
            keys = [k async for k in self._redis.scan_iter(match=pattern) if accept(k)]
            for start in range(0, len(keys), _DELETE_BATCH):
                removed += await self._redis.delete(
                    *keys[start : start + _DELETE_BATCH]
                )
        return removed


class _AIOKafkaTopicDeadLetter:
    def __init__(self, *, brokers: str, topic: str) -> None:
        from aiokafka import AIOKafkaProducer

        self._producer = AIOKafkaProducer(bootstrap_servers=brokers)
        self._topic = topic

    async def start(self) -> None:
        await self._producer.start()

    async def stop(self) -> None:
        await self._producer.stop()

    async def send(self, record: KafkaRecord, *, error_type: str) -> None:
        await self._producer.send_and_wait(
            self._topic,
            value=record.value,
            key=record.key,
            headers=[("x-error-type", error_type.encode())],
        )


def build_topic_dlq(*, brokers: str, topic: str) -> DeadLetterSink:
    return _AIOKafkaTopicDeadLetter(brokers=brokers, topic=topic)
