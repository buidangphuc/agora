"""AUE-28..AUE-32: team-ai erasure consumer over fakeredis and fake Kafka source and DLQ sink.

Contract the implementation must offer:

    app.modules.messaging.erasure.consumer.ErasureConsumer(
        source, dlq, redis, *, chat_prefix, quota_prefix, rate_limit_prefix, sleep=asyncio.sleep)
    await .run(): consumes until the source raises; commits via source.commit(record) only after the
    deletions (or the DLQ send); a Redis error is retried on the same record after sleep(delay) and
    never commits or parks the record meanwhile.
"""

from __future__ import annotations

from urllib.parse import quote

import pytest
from fakeredis import FakeAsyncRedis
from loguru import logger
from redis.exceptions import ConnectionError as RedisConnectionError

from app.modules.messaging.erasure.consumer import ErasureConsumer
from app.modules.messaging.kafka.consumer import KafkaRecord

USER = "user-7f3a-secret-id"
OTHER = "user-9b21"
CHAT = "chat:session"
QUOTA = "quota"
RATE = "rate-limit"
DLQ_TOPIC = "identity.events.team-ai.dlq"


def _varint(n: int) -> bytes:
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        out.append(b | (0x80 if n else 0))
        if not n:
            return bytes(out)


def _ld(number: int, value: bytes) -> bytes:
    return _varint(number << 3 | 2) + _varint(len(value)) + value


def envelope(type_: str, payload: bytes) -> bytes:
    return _ld(1, b"evt-1") + _ld(2, type_.encode()) + _ld(7, payload)


def user_erased(user_id: str) -> bytes:
    payload = _ld(1, user_id.encode()) if user_id else b""
    return envelope("platform.identity.v1.UserErased", payload)


GARBAGE = b"\x0a\x05ab"
STATUS_CHANGED = envelope("platform.identity.v1.UserStatusChanged", _ld(1, USER.encode()))


def record(value: bytes, offset: int = 0) -> KafkaRecord:
    return KafkaRecord(
        topic="identity.events", partition=0, offset=offset, key=None, value=value
    )


class _Drained(Exception):
    pass


class Source:
    def __init__(self, values: list[bytes]) -> None:
        self.records = [record(v, i) for i, v in enumerate(values)]
        self.commits: list[int] = []

    async def start(self) -> None: ...

    async def stop(self) -> None: ...

    async def getone(self) -> KafkaRecord:
        if not self.records:
            raise _Drained
        return self.records.pop(0)

    async def commit(self, rec: KafkaRecord) -> None:
        self.commits.append(rec.offset)


class Dlq:
    def __init__(self) -> None:
        self.sent: list[tuple[bytes, str]] = []

    async def start(self) -> None: ...

    async def stop(self) -> None: ...

    async def send(self, rec: KafkaRecord, *, error_type: str) -> None:
        self.sent.append((rec.value, error_type))


class FlakyRedis:
    """fakeredis whose mutating and scanning calls raise while ``failures`` > 0."""

    def __init__(self, failures: int = 0) -> None:
        self.inner = FakeAsyncRedis(decode_responses=True)
        self.failures = failures
        self.failed = 0

    def __getattr__(self, name: str):
        attr = getattr(self.inner, name)
        if name not in {"scan", "scan_iter", "delete", "unlink"}:
            return attr

        def guarded(*args, **kwargs):
            if self.failures > 0:
                self.failures -= 1
                self.failed += 1
                raise RedisConnectionError("redis down")
            return attr(*args, **kwargs)

        return guarded


def chat_key(user: str, session: str) -> str:
    return f"{CHAT}:{quote(user, safe='')}:{quote(session, safe='')}"


def user_keys(user: str) -> list[str]:
    return [
        chat_key(user, "s1"),
        chat_key(user, "s2"),
        chat_key(user, "s3"),
        f"{QUOTA}:c:user:{user}:chat:2026-11-15",
        f"{QUOTA}:i:user:{user}:chat:2026-11-15:res-1",
        f"{RATE}:user:{user}:29000",
        f"{RATE}:user:{user}",
    ]


async def seed(redis: FlakyRedis, *users: str) -> None:
    for u in users:
        for k in user_keys(u):
            await redis.inner.set(k, "x")


async def all_keys(redis: FlakyRedis) -> set[str]:
    return {k async for k in redis.inner.scan_iter(match="*")}


async def no_sleep(_: float) -> None: ...


def build(source: Source, dlq: Dlq, redis: FlakyRedis, sleep=no_sleep) -> ErasureConsumer:
    return ErasureConsumer(
        source,
        dlq,
        redis,
        chat_prefix=CHAT,
        quota_prefix=QUOTA,
        rate_limit_prefix=RATE,
        sleep=sleep,
    )


async def run(consumer: ErasureConsumer) -> None:
    with pytest.raises(_Drained):
        await consumer.run()


async def test_aue_28_chat_quota_rate_limit_keys_removed() -> None:
    redis = FlakyRedis()
    await seed(redis, USER, OTHER)
    source, dlq = Source([user_erased(USER)]), Dlq()

    await run(build(source, dlq, redis))

    keys = await all_keys(redis)
    assert keys == set(user_keys(OTHER))
    assert source.commits == [0]
    assert dlq.sent == []


async def test_aue_28_redelivery_and_missing_keys_are_noops() -> None:
    redis = FlakyRedis()
    await seed(redis, OTHER)
    source, dlq = Source([user_erased(USER), user_erased(USER)]), Dlq()

    await run(build(source, dlq, redis))

    assert await all_keys(redis) == set(user_keys(OTHER))
    assert source.commits == [0, 1]
    assert dlq.sent == []


async def test_aue_28_other_envelope_types_are_skipped_and_committed() -> None:
    redis = FlakyRedis()
    await seed(redis, USER)
    source, dlq = Source([STATUS_CHANGED]), Dlq()

    await run(build(source, dlq, redis))

    assert await all_keys(redis) == set(user_keys(USER))
    assert source.commits == [0]
    assert dlq.sent == []


async def test_aue_29_prefix_sharing_id_is_not_matched() -> None:
    redis = FlakyRedis()
    await seed(redis, "abc", "abcd")

    await run(build(Source([user_erased("abc")]), Dlq(), redis))

    assert await all_keys(redis) == set(user_keys("abcd"))


async def test_aue_29_glob_characters_in_id_match_literally() -> None:
    redis = FlakyRedis()
    await seed(redis, "a*c", "abc", "a?c", "a[b]c")

    await run(build(Source([user_erased("a*c")]), Dlq(), redis))

    expected = set(user_keys("abc")) | set(user_keys("a?c")) | set(user_keys("a[b]c"))
    assert await all_keys(redis) == expected


async def test_aue_29_id_with_colon_does_not_hit_other_chat_principal() -> None:
    redis = FlakyRedis()
    await seed(redis, "a:b", "a")

    await run(build(Source([user_erased("a")]), Dlq(), redis))

    assert await all_keys(redis) == set(user_keys("a:b"))


async def test_aue_30_redis_failure_retries_without_commit() -> None:
    redis = FlakyRedis(failures=2)
    await seed(redis, USER, OTHER)
    source, dlq = Source([user_erased(USER)]), Dlq()
    commits_seen_during_retry: list[list[int]] = []

    async def sleep(_: float) -> None:
        commits_seen_during_retry.append(list(source.commits))

    await run(build(source, dlq, redis, sleep=sleep))

    assert redis.failed == 2
    assert commits_seen_during_retry
    assert all(c == [] for c in commits_seen_during_retry)
    assert await all_keys(redis) == set(user_keys(OTHER))
    assert source.commits == [0]
    assert dlq.sent == []


async def test_aue_30_persistent_redis_failure_never_commits_or_parks() -> None:
    redis = FlakyRedis(failures=10_000)
    await seed(redis, USER)
    source, dlq = Source([user_erased(USER)]), Dlq()
    sleeps = 0

    class _Stop(Exception):
        pass

    async def sleep(_: float) -> None:
        nonlocal sleeps
        sleeps += 1
        if sleeps == 5:
            raise _Stop

    with pytest.raises(_Stop):
        await build(source, dlq, redis, sleep=sleep).run()

    assert source.commits == []
    assert dlq.sent == []
    assert set(user_keys(USER)) <= await all_keys(redis)


async def test_aue_31_poison_records_go_to_dlq_and_offsets_advance() -> None:
    redis = FlakyRedis()
    await seed(redis, USER, OTHER)
    values = [GARBAGE, user_erased("")]
    source, dlq = Source(values), Dlq()

    await run(build(source, dlq, redis))

    assert [v for v, _ in dlq.sent] == values
    assert all(error_type for _, error_type in dlq.sent)
    assert source.commits == [0, 1]
    assert await all_keys(redis) == set(user_keys(USER)) | set(user_keys(OTHER))


async def test_aue_32_erasure_logs_carry_no_raw_id() -> None:
    redis = FlakyRedis(failures=1)
    await seed(redis, USER)
    lines: list[str] = []
    sink = logger.add(lambda m: lines.append(str(m)), level="DEBUG", format="{message} {extra}")
    try:
        await run(build(Source([user_erased(USER), GARBAGE, user_erased("")]), Dlq(), redis))
    finally:
        logger.remove(sink)

    assert lines
    assert not [line for line in lines if USER in line or quote(USER, safe="") in line]
