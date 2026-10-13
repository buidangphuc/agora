import asyncio

import pytest
from fakeredis import aioredis

from app.modules.ai.llm.session import (
    InMemorySessionStore,
    RedisSessionStore,
    Turn,
    bound_turns,
)


class _Clock:
    def __init__(self) -> None:
        self.now = 0.0

    def __call__(self) -> float:
        return self.now


def _memory(clock: _Clock | None = None, **kw) -> InMemorySessionStore:
    return InMemorySessionStore(
        ttl_seconds=kw.get("ttl", 100),
        max_turns=kw.get("turns", 5),
        max_chars=kw.get("chars", 1000),
        clock=clock or _Clock(),
    )


def _redis(**kw):
    client = aioredis.FakeRedis(decode_responses=True)
    store = RedisSessionStore(
        client,
        ttl_seconds=kw.get("ttl", 100),
        max_turns=kw.get("turns", 5),
        max_chars=kw.get("chars", 1000),
    )
    return store, client


@pytest.fixture(params=["memory", "redis"])
def store(request):
    return _memory() if request.param == "memory" else _redis()[0]


async def test_turns_come_back_in_order(store):
    await store.append("p", "s", Turn("q1", "a1"))
    await store.append("p", "s", Turn("q2", "a2"))

    assert await store.load("p", "s") == [Turn("q1", "a1"), Turn("q2", "a2")]


async def test_history_is_scoped_by_principal_and_session(store):
    await store.append("alice", "s1", Turn("secret", "reply"))

    assert await store.load("bob", "s1") == []
    assert await store.load("alice", "s2") == []
    assert await store.load("alice", "s1") == [Turn("secret", "reply")]


async def test_key_format_is_unchanged():
    store, client = _redis()

    await store.append("u:1", "s/1", Turn("q", "a"))

    assert await client.keys("*") == ["chat:session:u%3A1:s%2F1"]
    assert await client.lrange("chat:session:u%3A1:s%2F1", 0, -1) == [  # type: ignore[misc]
        '{"u": "q", "a": "a"}'
    ]


async def test_colon_in_ids_cannot_collide():
    store, _ = _redis()
    await store.append("a", "b:c", Turn("x", "y"))

    assert await store.load("a:b", "c") == []


async def test_oldest_turns_drop_over_the_turn_bound():
    for store in (_memory(turns=2), _redis(turns=2)[0]):
        for i in range(4):
            await store.append("p", "s", Turn(f"q{i}", f"a{i}"))

        assert await store.load("p", "s") == [Turn("q2", "a2"), Turn("q3", "a3")]


async def test_oldest_turns_drop_over_the_char_bound():
    for store in (_memory(chars=20), _redis(chars=20)[0]):
        await store.append("p", "s", Turn("aaaaa", "bbbbb"))
        await store.append("p", "s", Turn("ccccc", "ddddd"))
        await store.append("p", "s", Turn("eeeee", "fffff"))

        assert await store.load("p", "s") == [
            Turn("ccccc", "ddddd"),
            Turn("eeeee", "fffff"),
        ]


async def test_zero_turns_keeps_nothing_and_deletes_the_redis_key():
    memory = _memory(turns=0)
    redis_store, client = _redis(turns=0)
    await client.rpush(redis_store.key("p", "s"), '{"u": "old", "a": "old"}')  # type: ignore[misc]

    for store in (memory, redis_store):
        await store.append("p", "s", Turn("hi", "yo"))
        assert await store.load("p", "s") == []
    assert await client.exists(redis_store.key("p", "s")) == 0
    assert bound_turns([Turn("a", "b")], max_turns=0, max_chars=100) == []


async def test_memory_ttl_expires_and_append_refreshes_it():
    clock = _Clock()
    store = _memory(clock, ttl=100)
    await store.append("p", "s", Turn("q", "a"))

    clock.now = 90
    await store.append("p", "s", Turn("q2", "a2"))
    clock.now = 180
    assert len(await store.load("p", "s")) == 2
    clock.now = 191
    assert await store.load("p", "s") == []


async def test_redis_sets_a_ttl_and_expires():
    store, client = _redis(ttl=1)
    await store.append("p", "s", Turn("q", "a"))

    assert 0 < await client.ttl(store.key("p", "s")) <= 1
    await asyncio.sleep(1.2)
    assert await store.load("p", "s") == []


async def test_corrupt_element_is_skipped():
    store, client = _redis()
    await store.append("p", "s", Turn("q", "a"))
    await client.rpush(store.key("p", "s"), "not-json")  # type: ignore[misc]
    await client.rpush(store.key("p", "s"), '{"u": "only user"}')  # type: ignore[misc]

    assert await store.load("p", "s") == [Turn("q", "a")]


async def test_an_oversized_turn_is_truncated_on_write():
    store, client = _redis(chars=5)

    await store.append("p", "s", Turn("abcdefgh", "xy"))

    assert await client.lrange(store.key("p", "s"), 0, -1) == [  # type: ignore[misc]
        '{"u": "abcde", "a": "xy"}'
    ]


async def test_the_shared_client_is_never_closed():
    store, client = _redis()
    await store.append("p", "s", Turn("q", "a"))

    assert not hasattr(store, "aclose")
    assert await client.ping()
