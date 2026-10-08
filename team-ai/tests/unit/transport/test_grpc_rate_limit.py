"""RateLimitInterceptor through a real in-process gRPC server."""

from __future__ import annotations

import grpc
import pytest

from app.modules.platform.rate_limit.factory import build_principal_rate_limiter
from app.modules.platform.rate_limit.service import InMemoryRateLimiter
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc
from app.transport.grpc._pb.platform.chat.v1 import chat_pb2, chat_pb2_grpc
from app.transport.grpc.chat_stream import build_chat_streamer
from app.transport.grpc.interceptors.rate_limit import RATE_LIMITED_METHODS
from app.transport.grpc.server import build_grpc_server
from tests.factories import build_test_settings


def _principal(pid: str):
    return (
        ("x-principal-id", pid),
        ("x-principal-type", "user"),
        # Scopes the gated RPCs under test require; the limiter keys on the id only.
        ("x-principal-scopes", "ai:use,listing.write,listing.read"),
    )


def _settings(**overrides):
    base = {
        "AUTH_BEARER_TOKEN": "secret",
        "GRPC_REFLECTION_ENABLED": False,
        "CHAT_BACKEND": "mock",
    }
    base.update(overrides)
    return build_test_settings(**base)


async def _start(limiter, settings=None):
    settings = settings or _settings()
    server = build_grpc_server(
        settings=settings,
        rag_provider=lambda: None,
        chat_streamer=build_chat_streamer(settings),
        rate_limiter=limiter,
    )
    port = server.add_insecure_port("localhost:0")
    await server.start()
    return server, port


async def _stream_chat(channel, pid: str) -> list[str]:
    stub = chat_pb2_grpc.ChatServiceStub(channel)
    return [
        r.delta
        async for r in stub.StreamChat(
            chat_pb2.StreamChatRequest(session_id="s", message="hi"),
            metadata=_principal(pid),
        )
    ]


async def _assistant(channel, pid: str):
    stub = ai_pb2_grpc.AIServiceStub(channel)
    return await stub.ShoppingAssistant(
        ai_pb2.ShoppingAssistantRequest(message="ao thun"), metadata=_principal(pid)
    )


def test_limited_methods_match_the_generated_service_descriptors():
    assert (
        f"/{chat_pb2.DESCRIPTOR.services_by_name['ChatService'].full_name}/StreamChat"
        in RATE_LIMITED_METHODS
    )
    assert (
        f"/{ai_pb2.DESCRIPTOR.services_by_name['AIService'].full_name}/ShoppingAssistant"
        in RATE_LIMITED_METHODS
    )


async def test_one_principal_is_throttled_and_another_is_unaffected():
    server, port = await _start(InMemoryRateLimiter(limit=2, window_seconds=60))
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            await _stream_chat(channel, "alice")
            await _stream_chat(channel, "alice")
            with pytest.raises(grpc.aio.AioRpcError) as exc:
                await _stream_chat(channel, "alice")
            assert exc.value.code() == grpc.StatusCode.RESOURCE_EXHAUSTED
            trailing = dict(exc.value.trailing_metadata() or ())
            assert int(trailing["retry-after"]) >= 1

            # bob has his own budget
            assert await _stream_chat(channel, "bob")
    finally:
        await server.stop(None)


async def test_unary_and_stream_methods_both_count_against_the_limit():
    server, port = await _start(InMemoryRateLimiter(limit=2, window_seconds=60))
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            await _stream_chat(channel, "alice")
            await _assistant(channel, "alice")
            with pytest.raises(grpc.aio.AioRpcError) as stream_exc:
                await _stream_chat(channel, "alice")
            with pytest.raises(grpc.aio.AioRpcError) as unary_exc:
                await _assistant(channel, "alice")
        assert stream_exc.value.code() == grpc.StatusCode.RESOURCE_EXHAUSTED
        assert unary_exc.value.code() == grpc.StatusCode.RESOURCE_EXHAUSTED
    finally:
        await server.stop(None)


async def test_other_methods_are_not_rate_limited():
    server, port = await _start(InMemoryRateLimiter(limit=1, window_seconds=60))
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            stub = ai_pb2_grpc.AIServiceStub(channel)
            for _ in range(3):
                await stub.ChatCopilot(
                    ai_pb2.ChatCopilotRequest(buyer_message="con hang khong"),
                    metadata=_principal("alice"),
                )
    finally:
        await server.stop(None)


async def test_unauthenticated_calls_are_rejected_before_rate_limiting():
    limiter = InMemoryRateLimiter(limit=1, window_seconds=60)
    server, port = await _start(limiter)
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            for _ in range(2):
                with pytest.raises(grpc.aio.AioRpcError) as exc:
                    stub = chat_pb2_grpc.ChatServiceStub(channel)
                    _ = [
                        r
                        async for r in stub.StreamChat(
                            chat_pb2.StreamChatRequest(session_id="s", message="hi")
                        )
                    ]
                assert exc.value.code() == grpc.StatusCode.UNAUTHENTICATED
    finally:
        await server.stop(None)


async def test_limiter_backend_failure_fails_open():
    class Broken:
        async def check(self, key: str):
            raise ConnectionError("redis down")

    server, port = await _start(Broken())
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            assert await _stream_chat(channel, "alice")
    finally:
        await server.stop(None)


async def test_grpc_rate_limit_can_be_disabled():
    settings = _settings(
        GRPC_RATE_LIMIT_ENABLED=False, RATE_LIMIT_PRINCIPAL_PER_MINUTE=1
    )
    server, port = await _start(None, settings)
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            for _ in range(3):
                assert await _stream_chat(channel, "alice")
    finally:
        await server.stop(None)


async def test_default_limiter_comes_from_the_factory_settings():
    settings = _settings(
        GRPC_RATE_LIMIT_ENABLED=True, RATE_LIMIT_PRINCIPAL_PER_MINUTE=1
    )
    server, port = await _start(None, settings)  # no explicit limiter
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            await _stream_chat(channel, "alice")
            with pytest.raises(grpc.aio.AioRpcError) as exc:
                await _stream_chat(channel, "alice")
        assert exc.value.code() == grpc.StatusCode.RESOURCE_EXHAUSTED
    finally:
        await server.stop(None)


# --- shared (Redis) backend across replicas -------------------------------------


class _SharedRedis:
    """Stub of the Lua check script, shared by every limiter ("replica")."""

    def __init__(self) -> None:
        self.values: dict[str, int] = {}

    async def eval(
        self, script, num_keys, current_key, previous_key, window, limit, elapsed
    ):
        current = self.values.get(current_key, 0)
        previous = self.values.get(previous_key, 0)
        weighted = current + int(previous * max(1.0 - elapsed / window, 0.0))
        if weighted >= limit:
            return [weighted, 1, 0]
        self.values[current_key] = current + 1
        return [current + 1, 1, 1]


async def test_redis_backend_shares_one_limit_across_replicas():
    redis = _SharedRedis()
    settings = _settings(
        RATE_LIMIT_BACKEND="redis",
        REDIS_ENABLED=True,
        RATE_LIMIT_PRINCIPAL_PER_MINUTE=2,
    )
    replica_a = build_principal_rate_limiter(settings, redis=redis)  # type: ignore[arg-type]
    replica_b = build_principal_rate_limiter(settings, redis=redis)  # type: ignore[arg-type]

    assert (await replica_a.check("user:alice")).allowed is True
    assert (await replica_b.check("user:alice")).allowed is True
    assert (await replica_a.check("user:alice")).allowed is False
    assert (await replica_b.check("user:alice")).allowed is False
    assert (await replica_b.check("user:bob")).allowed is True


def test_redis_limiter_uses_wall_clock_so_replicas_agree_on_the_window():
    import time

    limiter = build_principal_rate_limiter(
        _settings(RATE_LIMIT_BACKEND="redis", REDIS_ENABLED=True),
        redis=_SharedRedis(),  # type: ignore[arg-type]
    )

    assert limiter.clock is time.time


async def test_stop_grpc_server_closes_the_redis_client_it_built(monkeypatch):
    from app.transport.grpc.server import stop_grpc_server

    class _Redis:
        closed = False

        async def aclose(self):
            self.closed = True

    fake = _Redis()
    monkeypatch.setattr("app.core.redis.build_redis_client", lambda _s: fake)
    settings = _settings(
        GRPC_RATE_LIMIT_ENABLED=True,
        RATE_LIMIT_BACKEND="redis",
        REDIS_ENABLED=True,
    )
    server, _ = await _start(None, settings)
    assert fake.closed is False
    await stop_grpc_server(server, None)
    assert fake.closed is True
