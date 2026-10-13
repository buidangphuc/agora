"""Tracing, Auth and RateLimit interceptors through a real in-process grpc.aio server."""

from __future__ import annotations

import json
from collections.abc import AsyncIterator, Awaitable
from contextlib import asynccontextmanager
from typing import Any

import grpc
import pytest

from app.core import grpc_metrics
from app.core.request_context import get_request_id
from app.modules.platform.rate_limit.service import RateLimitResult
from app.transport.grpc.context import current_client_ip, current_principal
from app.transport.grpc.interceptors.auth import AuthInterceptor
from app.transport.grpc.interceptors.rate_limit import RateLimitInterceptor
from app.transport.grpc.interceptors.tracing import TracingInterceptor

UNARY = "/t.Svc/Echo"
STREAM = "/t.Svc/Stream"
HEALTH = "/grpc.health.v1.Health/Check"
REFLECTION = "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo"


def _snapshot() -> bytes:
    principal = current_principal()
    return json.dumps(
        {
            "principal": principal.model_dump() if principal else None,
            "request_id": get_request_id(),
            "client_ip": current_client_ip(),
        }
    ).encode()


async def _echo(request: bytes, context: grpc.aio.ServicerContext) -> bytes:
    return _snapshot()


async def _stream(
    request: bytes, context: grpc.aio.ServicerContext
) -> AsyncIterator[bytes]:
    yield _snapshot()
    yield _snapshot()


def _identity(value: bytes) -> bytes:
    return value


def _unary_handler() -> grpc.RpcMethodHandler:
    return grpc.unary_unary_rpc_method_handler(
        _echo, request_deserializer=_identity, response_serializer=_identity
    )


def _generic_handlers() -> tuple[grpc.GenericRpcHandler, ...]:
    stream = grpc.unary_stream_rpc_method_handler(
        _stream, request_deserializer=_identity, response_serializer=_identity
    )
    return (
        grpc.method_handlers_generic_handler(
            "t.Svc", {"Echo": _unary_handler(), "Stream": stream}
        ),
        grpc.method_handlers_generic_handler(
            "grpc.health.v1.Health", {"Check": _unary_handler()}
        ),
        grpc.method_handlers_generic_handler(
            "grpc.reflection.v1alpha.ServerReflection",
            {"ServerReflectionInfo": _unary_handler()},
        ),
    )


@asynccontextmanager
async def _serve(
    *interceptors: grpc.aio.ServerInterceptor,
) -> AsyncIterator[grpc.aio.Channel]:
    server = grpc.aio.server(interceptors=interceptors)
    server.add_generic_rpc_handlers(_generic_handlers())
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    channel = grpc.aio.insecure_channel(f"127.0.0.1:{port}")
    try:
        yield channel
    finally:
        await channel.close()
        await server.stop(None)


def _principal_md(
    pid: str = "u1", ptype: str = "user", scopes: str = "listing.read, listing.write"
) -> tuple[tuple[str, str], ...]:
    return (
        ("x-principal-id", pid),
        ("x-principal-type", ptype),
        ("x-principal-scopes", scopes),
    )


async def _call(channel: grpc.aio.Channel, metadata: Any = ()) -> dict[str, Any]:
    raw = await channel.unary_unary(UNARY)(b"", metadata=metadata)
    return json.loads(raw)


async def _failure_of(call: Awaitable[Any]) -> grpc.aio.AioRpcError:
    with pytest.raises(grpc.aio.AioRpcError) as info:
        await call
    return info.value


# --- Tracing -------------------------------------------------------------------


async def test_incoming_request_id_is_kept() -> None:
    async with _serve(TracingInterceptor()) as channel:
        seen = await _call(channel, (("x-request-id", "abc"),))
    assert seen["request_id"] == "abc"


async def test_missing_request_id_is_minted() -> None:
    async with _serve(TracingInterceptor()) as channel:
        seen = await _call(channel)
    assert seen["request_id"].startswith("req_")
    assert len(seen["request_id"]) == len("req_") + 32


async def test_request_id_does_not_leak_after_the_call() -> None:
    async with _serve(TracingInterceptor()) as channel:
        await _call(channel, (("x-request-id", "abc"),))
    assert get_request_id() == "-"


async def test_every_rpc_is_counted_including_health_and_streams() -> None:
    grpc_metrics.reset()
    async with _serve(TracingInterceptor()) as channel:
        await _call(channel)
        await channel.unary_unary(HEALTH)(b"")
        async for _ in channel.unary_stream(STREAM)(b""):
            pass
    assert "teamai_grpc_requests_total 3" in grpc_metrics.render()


# --- Auth ----------------------------------------------------------------------


async def test_forwarded_principal_is_trusted_on_unary_and_stream() -> None:
    expected = {
        "id": "u1",
        "type": "user",
        "scopes": ["listing.read", "listing.write"],
    }
    async with _serve(AuthInterceptor()) as channel:
        assert (await _call(channel, _principal_md()))["principal"] == expected
        frames = [
            json.loads(f)
            async for f in channel.unary_stream(STREAM)(b"", metadata=_principal_md())
        ]
    assert [f["principal"] for f in frames] == [expected, expected]


async def test_principal_does_not_leak_after_the_call() -> None:
    async with _serve(AuthInterceptor()) as channel:
        await _call(channel, _principal_md())
    assert current_principal() is None


@pytest.mark.parametrize(
    ("ptype", "pid", "expected_id"),
    [("root", "u1", "u1"), ("user", "  ", "anonymous"), ("service", "", "anonymous")],
)
async def test_unknown_type_or_blank_id_fails_closed(
    ptype: str, pid: str, expected_id: str
) -> None:
    async with _serve(AuthInterceptor()) as channel:
        seen = await _call(channel, _principal_md(pid, ptype, "admin"))
    assert seen["principal"]["type"] == "anonymous"
    assert seen["principal"]["id"] == expected_id


async def test_missing_principal_is_unauthenticated() -> None:
    async with _serve(AuthInterceptor()) as channel:
        err = await _failure_of(channel.unary_unary(UNARY)(b""))
    assert err.code() is grpc.StatusCode.UNAUTHENTICATED
    assert err.details() == "Missing gateway principal"


@pytest.mark.parametrize("method", [HEALTH, REFLECTION])
async def test_health_and_reflection_are_exempt(method: str) -> None:
    async with _serve(AuthInterceptor()) as channel:
        assert await channel.unary_unary(method)(b"")


async def test_client_ip_is_bound_stripped() -> None:
    md = (*_principal_md(), ("x-client-ip", " 203.0.113.7 "))
    async with _serve(AuthInterceptor()) as channel:
        seen = await _call(channel, md)
    assert seen["client_ip"] == "203.0.113.7"


@pytest.mark.parametrize("ip", ["x" * 65, "1.2.3.4 evil"])
async def test_bad_client_ip_binds_empty(ip: str) -> None:
    async with _serve(AuthInterceptor()) as channel:
        seen = await _call(channel, (*_principal_md(), ("x-client-ip", ip)))
    assert seen["client_ip"] == ""


async def test_client_ip_does_not_leak_to_a_call_without_one() -> None:
    async with _serve(AuthInterceptor()) as channel:
        await _call(channel, (*_principal_md(), ("x-client-ip", "203.0.113.7")))
        seen = await _call(channel, _principal_md())
    assert seen["client_ip"] == ""


# --- Rate limit ----------------------------------------------------------------


class _FakeLimiter:
    def __init__(
        self, result: RateLimitResult | None = None, error: Exception | None = None
    ) -> None:
        self.result = result or RateLimitResult(allowed=True, remaining=1)
        self.error = error
        self.keys: list[str] = []

    async def check(self, key: str) -> RateLimitResult:
        self.keys.append(key)
        if self.error:
            raise self.error
        return self.result


def _chain(
    limiter: _FakeLimiter, methods: frozenset[str] = frozenset({UNARY, STREAM})
) -> tuple[grpc.aio.ServerInterceptor, ...]:
    return (AuthInterceptor(), RateLimitInterceptor(limiter, methods=methods))


async def test_denied_call_is_resource_exhausted_with_retry_after() -> None:
    limiter = _FakeLimiter(
        RateLimitResult(allowed=False, remaining=0, retry_after_seconds=7)
    )
    async with _serve(*_chain(limiter)) as channel:
        err = await _failure_of(
            channel.unary_unary(UNARY)(b"", metadata=_principal_md())
        )
    assert err.code() is grpc.StatusCode.RESOURCE_EXHAUSTED
    assert err.details() == "rate limit exceeded"
    assert dict(err.trailing_metadata() or ())["retry-after"] == "7"
    assert limiter.keys == ["user:u1"]


async def test_denied_without_retry_after_defaults_to_one() -> None:
    limiter = _FakeLimiter(RateLimitResult(allowed=False, remaining=0))
    async with _serve(*_chain(limiter)) as channel:
        err = await _failure_of(
            channel.unary_unary(UNARY)(b"", metadata=_principal_md())
        )
    assert dict(err.trailing_metadata() or ())["retry-after"] == "1"


async def test_denied_stream_is_resource_exhausted() -> None:
    limiter = _FakeLimiter(RateLimitResult(allowed=False, remaining=0))
    async with _serve(*_chain(limiter)) as channel:
        stream = channel.unary_stream(STREAM)(b"", metadata=_principal_md())
        err = await _failure_of(stream.read())
    assert err.code() is grpc.StatusCode.RESOURCE_EXHAUSTED


async def test_unlisted_method_never_consults_the_limiter() -> None:
    limiter = _FakeLimiter(RateLimitResult(allowed=False, remaining=0))
    async with _serve(*_chain(limiter, methods=frozenset({STREAM}))) as channel:
        seen = await _call(channel, _principal_md())
    assert seen["principal"]["id"] == "u1"
    assert limiter.keys == []


async def test_limiter_outage_fails_open() -> None:
    limiter = _FakeLimiter(error=RuntimeError("redis down"))
    async with _serve(*_chain(limiter)) as channel:
        seen = await _call(channel, _principal_md())
    assert seen["principal"]["id"] == "u1"
    assert limiter.keys == ["user:u1"]


async def test_anonymous_caller_is_keyed_by_forwarded_ip() -> None:
    limiter = _FakeLimiter()
    md = (
        *_principal_md("anonymous", "anonymous", ""),
        ("x-client-ip", "203.0.113.7"),
    )
    async with _serve(*_chain(limiter)) as channel:
        await _call(channel, md)
    assert limiter.keys == ["anonymous:ip:203.0.113.7"]
