"""SessionStore (memory + redis) and its use by LLMRouterChatStreamer."""

from __future__ import annotations

import asyncio

import pytest
from fakeredis import aioredis

from app.modules.ai.llm.router import ModelRouter
from app.modules.ai.llm.session import (
    InMemorySessionStore,
    RedisSessionStore,
    Turn,
    bound_turns,
)
from app.modules.ai.llm.testing import Script, ScriptedProvider, StatusError
from app.transport.grpc.chat_stream import (
    ChainExhausted,
    LLMRouterChatStreamer,
    StreamInterrupted,
)
from tests.factories import build_test_settings


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


def _redis(**kw) -> tuple[RedisSessionStore, object]:
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


# --- store contract (both backends) ------------------------------------------


async def test_store_returns_turns_in_order(store):
    await store.append("p", "s", Turn("q1", "a1"))
    await store.append("p", "s", Turn("q2", "a2"))

    assert await store.load("p", "s") == [Turn("q1", "a1"), Turn("q2", "a2")]


async def test_store_is_scoped_by_principal_and_session(store):
    await store.append("alice", "s1", Turn("secret", "reply"))

    assert await store.load("bob", "s1") == []
    assert await store.load("alice", "s2") == []
    assert await store.load("alice", "s1") == [Turn("secret", "reply")]


async def test_store_keys_with_colons_cannot_collide():
    redis_store, _ = _redis()
    await redis_store.append("a", "b:c", Turn("x", "y"))

    assert await redis_store.load("a:b", "c") == []
    assert redis_store.key("a", "b:c") != redis_store.key("a:b", "c")


async def test_store_drops_oldest_turns_over_the_turn_bound():
    for store_ in (_memory(turns=2), _redis(turns=2)[0]):
        for i in range(4):
            await store_.append("p", "s", Turn(f"q{i}", f"a{i}"))

        assert await store_.load("p", "s") == [Turn("q2", "a2"), Turn("q3", "a3")]


async def test_store_drops_oldest_turns_over_the_char_bound():
    for store_ in (_memory(chars=20), _redis(chars=20)[0]):
        await store_.append("p", "s", Turn("aaaaa", "bbbbb"))  # 10 chars
        await store_.append("p", "s", Turn("ccccc", "ddddd"))  # 20 total: fits
        await store_.append("p", "s", Turn("eeeee", "fffff"))  # 30: drop oldest

        assert await store_.load("p", "s") == [
            Turn("ccccc", "ddddd"),
            Turn("eeeee", "fffff"),
        ]


@pytest.mark.parametrize("backend", ["memory", "redis"])
async def test_zero_max_turns_keeps_nothing(backend):
    if backend == "memory":
        zero = _memory(turns=0)
    else:
        zero, client = _redis(turns=0)
    await zero.append("p", "s", Turn("hi", "yo"))
    await zero.append("p", "s", Turn("again", "yo2"))
    assert await zero.load("p", "s") == []
    if backend == "redis":
        assert await client.llen(zero.key("p", "s")) == 0


def test_bound_turns_zero_max_turns_keeps_nothing():
    assert bound_turns([Turn("a", "b")], max_turns=0, max_chars=100) == []


async def test_memory_ttl_expiry_clears_history_and_append_refreshes_it():
    clock = _Clock()
    store_ = _memory(clock, ttl=100)
    await store_.append("p", "s", Turn("q", "a"))

    clock.now = 90
    await store_.append("p", "s", Turn("q2", "a2"))  # refreshes the TTL
    clock.now = 180
    assert len(await store_.load("p", "s")) == 2
    clock.now = 191
    assert await store_.load("p", "s") == []


async def test_redis_sets_ttl_and_expires():
    store_, client = _redis(ttl=1)
    await store_.append("p", "s", Turn("q", "a"))

    assert 0 < await client.ttl(store_.key("p", "s")) <= 1  # type: ignore[attr-defined]
    await asyncio.sleep(1.2)
    assert await store_.load("p", "s") == []


async def test_redis_ignores_corrupt_elements():
    store_, client = _redis()
    await store_.append("p", "s", Turn("q", "a"))
    await client.rpush(store_.key("p", "s"), "not-json")  # type: ignore[attr-defined]

    assert await store_.load("p", "s") == [Turn("q", "a")]


# --- streamer integration ------------------------------------------------------


def _streamer(provider: ScriptedProvider, store_, *, fallbacks: str = ""):
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS=fallbacks),
        model_builder=provider.builder,
    )
    return LLMRouterChatStreamer(router, session_store=store_)


async def _chat(streamer, message, *, principal="alice", session="s1"):
    return [
        d
        async for d in streamer.astream(
            message, session_id=session, principal_id=principal
        )
    ]


def _texts(call) -> list[tuple[str, str]]:
    return [(m.type, str(m.content)) for m in call.messages]


async def test_follow_up_sees_prior_turns_in_order(store):
    provider = ScriptedProvider()
    provider.queue("a", Script(chunks=("Hi ", "there")), Script(chunks=("ok",)))
    streamer = _streamer(provider, store)

    await _chat(streamer, "hello")
    await _chat(streamer, "and then?")

    second = _texts(provider.calls[1])
    assert [t for t, _ in second] == ["system", "human", "ai", "human"]
    assert second[1:] == [
        ("human", "hello"),
        ("ai", "Hi there"),
        ("human", "and then?"),
    ]


async def test_other_principal_cannot_read_the_session(store):
    provider = ScriptedProvider().queue("a", Script(chunks=("ok",)), Script())
    streamer = _streamer(provider, store)

    await _chat(streamer, "my private question", principal="alice")
    await _chat(streamer, "hi", principal="mallory")  # same session_id

    assert [t for t, _ in _texts(provider.calls[1])] == ["system", "human"]


async def test_no_session_id_or_no_principal_is_stateless(store):
    provider = ScriptedProvider()
    streamer = _streamer(provider, store)

    await _chat(streamer, "one", session="")
    await _chat(streamer, "two", session="")
    await _chat(streamer, "three", principal="")
    await _chat(streamer, "four", principal="")

    assert all(len(c.messages) == 2 for c in provider.calls)
    assert await store.load("alice", "") == []


async def test_failed_reply_before_first_chunk_stores_nothing(store):
    provider = ScriptedProvider()
    provider.queue("a", *[Script(fail_at=0, error=StatusError(500))] * 5)
    streamer = _streamer(provider, store)

    with pytest.raises(ChainExhausted):
        await _chat(streamer, "question")

    assert await store.load("alice", "s1") == []


async def test_partial_reply_after_first_chunk_stores_nothing(store):
    provider = ScriptedProvider()
    provider.queue(
        "a", Script(chunks=("par", "tial"), fail_at=1, error=StatusError(503))
    )
    streamer = _streamer(provider, store)

    with pytest.raises(StreamInterrupted):
        await _chat(streamer, "question")

    assert await store.load("alice", "s1") == []


async def test_client_disconnect_mid_reply_stores_nothing(store):
    provider = ScriptedProvider().queue("a", Script(chunks=("a", "b", "c")))
    streamer = _streamer(provider, store)

    gen = streamer.astream("question", session_id="s1", principal_id="alice")
    await gen.__anext__()
    await gen.aclose()

    assert await store.load("alice", "s1") == []


async def test_fallback_reply_is_stored_once_from_the_serving_model(store):
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(429)))
    provider.queue("b", Script(chunks=("from-b",)))
    streamer = _streamer(provider, store, fallbacks="b")

    await _chat(streamer, "question")

    assert await store.load("alice", "s1") == [Turn("question", "from-b")]


async def test_store_failures_degrade_to_stateless_instead_of_failing_chat():
    class Broken:
        async def load(self, principal_id, session_id):
            raise ConnectionError("redis down")

        async def append(self, principal_id, session_id, turn):
            raise ConnectionError("redis down")

    provider = ScriptedProvider().queue("a", Script(chunks=("ok",)))
    streamer = _streamer(provider, Broken())

    assert await _chat(streamer, "hi") == ["ok"]


# --- shutdown: owned Redis client is closed ----------------------------------


async def test_redis_store_closes_only_a_client_it_owns():
    from unittest.mock import AsyncMock

    owned = AsyncMock()
    shared = AsyncMock()

    await RedisSessionStore(
        owned, ttl_seconds=1, max_turns=1, max_chars=1, owns_client=True
    ).aclose()
    await RedisSessionStore(shared, ttl_seconds=1, max_turns=1, max_chars=1).aclose()

    owned.aclose.assert_awaited_once()
    shared.aclose.assert_not_awaited()


async def test_streamer_aclose_closes_the_session_store_and_is_safe_without_one():
    from unittest.mock import AsyncMock

    from app.transport.grpc.chat_stream import MockChatStreamer, close_chat_streamer

    store_ = AsyncMock()
    streamer = _streamer(ScriptedProvider(), store_)

    await streamer.aclose()
    await close_chat_streamer(streamer)
    await close_chat_streamer(MockChatStreamer())  # owns nothing

    assert store_.aclose.await_count == 2


async def test_build_chat_streamer_with_redis_enabled_owns_and_closes_its_client(
    monkeypatch,
):
    from unittest.mock import AsyncMock

    import app.core.redis as redis_module
    from app.transport.grpc.chat_stream import build_chat_streamer, close_chat_streamer

    client = AsyncMock()
    monkeypatch.setattr(redis_module, "build_redis_client", lambda settings: client)
    settings = build_test_settings(
        CHAT_BACKEND="llm_router",
        CHAT_MODEL="a",
        REDIS_ENABLED=True,
    )
    streamer = build_chat_streamer(settings)

    await close_chat_streamer(streamer)

    client.aclose.assert_awaited_once()


def test_build_chat_streamer_uses_in_process_history_without_redis():
    from app.modules.ai.llm.session import InMemorySessionStore
    from app.transport.grpc.chat_stream import build_chat_streamer

    streamer = build_chat_streamer(
        build_test_settings(CHAT_BACKEND="llm_router", CHAT_MODEL="a")
    )

    assert isinstance(streamer._sessions, InMemorySessionStore)  # type: ignore[attr-defined]


async def test_history_is_bounded_to_the_newest_turns_in_the_model_input(store):
    provider = ScriptedProvider()
    streamer = _streamer(provider, store)  # store bound: 5 turns

    for i in range(8):
        await _chat(streamer, f"q{i}")

    last = _texts(provider.calls[-1])
    humans = [c for t, c in last if t == "human"]
    # the 5 newest earlier turns (q2..q6), then the current message
    assert humans == ["q2", "q3", "q4", "q5", "q6", "q7"]


async def test_history_stores_redacted_user_text(store):
    provider = ScriptedProvider().queue("a", Script(chunks=("ok",)), Script())
    streamer = _streamer(provider, store)

    await _chat(streamer, "call 0912345678")
    await _chat(streamer, "again")

    assert ("human", "call [phone]") in _texts(provider.calls[1])
    assert all("0912345678" not in c for _, c in _texts(provider.calls[1]))
