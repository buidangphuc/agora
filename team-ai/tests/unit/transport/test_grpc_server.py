"""build_grpc_server: interceptor order, registrations, health and reflection."""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import grpc
import pytest
from grpc_health.v1 import health_pb2, health_pb2_grpc

from app.core import grpc_metrics
from app.modules.platform.rate_limit.service import RateLimitResult
from app.transport.grpc.context import current_principal
from app.transport.grpc.registry import GrpcServiceRegistration
from app.transport.grpc.server import build_grpc_server
from tests.factories import allow_all_limiter

ECHO = "/t.Svc/Echo"
OPEN = "/t.Svc/Open"
SERVICE = "t.Svc"
PRINCIPAL_MD = (
    ("x-principal-id", "u1"),
    ("x-principal-type", "user"),
    ("x-principal-scopes", "listing.read"),
)


async def _echo(request: bytes, context: grpc.aio.ServicerContext) -> bytes:
    principal = current_principal()
    return (principal.id if principal else "none").encode()


def _registration(*, limited: frozenset[str] = frozenset()) -> GrpcServiceRegistration:
    handler = grpc.method_handlers_generic_handler(
        SERVICE,
        {
            "Echo": grpc.unary_unary_rpc_method_handler(_echo),
            "Open": grpc.unary_unary_rpc_method_handler(_echo),
        },
    )
    return GrpcServiceRegistration(
        service_name=SERVICE,
        register=lambda server: server.add_generic_rpc_handlers((handler,)),
        rate_limited_methods=limited,
    )


class _Limiter:
    def __init__(self, result: RateLimitResult) -> None:
        self.result = result
        self.keys: list[str] = []

    async def check(self, key: str) -> RateLimitResult:
        self.keys.append(key)
        return self.result


@asynccontextmanager
async def _serving(**kwargs) -> AsyncIterator[grpc.aio.Channel]:
    kwargs.setdefault("registrations", (_registration(),))
    kwargs.setdefault("rate_limiter", allow_all_limiter())
    server = build_grpc_server(**kwargs)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
            yield channel
    finally:
        await server.stop(None)


def _unary(channel: grpc.aio.Channel, path: str):
    return channel.unary_unary(
        path, request_serializer=bytes, response_deserializer=bytes
    )


async def test_registered_service_receives_forwarded_principal():
    async with _serving() as channel:
        reply = await _unary(channel, ECHO)(b"", metadata=PRINCIPAL_MD)

    assert reply == b"u1"


async def test_health_is_serving_without_metadata():
    async with _serving() as channel:
        stub = health_pb2_grpc.HealthStub(channel)
        reply = await stub.Check(health_pb2.HealthCheckRequest())

    assert reply.status == health_pb2.HealthCheckResponse.SERVING


async def test_health_watch_writes_serving_once():
    async with _serving() as channel:
        stub = health_pb2_grpc.HealthStub(channel)
        first = await stub.Watch(health_pb2.HealthCheckRequest()).read()

    assert first.status == health_pb2.HealthCheckResponse.SERVING


async def test_missing_principal_is_unauthenticated():
    async with _serving() as channel:
        with pytest.raises(grpc.aio.AioRpcError) as exc:
            await _unary(channel, ECHO)(b"")

    assert exc.value.code() == grpc.StatusCode.UNAUTHENTICATED


async def test_tracing_runs_before_auth_so_rejected_calls_are_counted():
    grpc_metrics.reset()
    async with _serving() as channel:
        with pytest.raises(grpc.aio.AioRpcError):
            await _unary(channel, ECHO)(b"")

    assert "teamai_grpc_requests_total 1" in grpc_metrics.render()


async def test_rate_limit_runs_after_auth_and_only_on_registered_methods():
    limiter = _Limiter(
        RateLimitResult(allowed=False, remaining=0, retry_after_seconds=7)
    )
    async with _serving(
        registrations=(_registration(limited=frozenset({ECHO})),),
        rate_limiter=limiter,
    ) as channel:
        with pytest.raises(grpc.aio.AioRpcError) as exc:
            await _unary(channel, ECHO)(b"", metadata=PRINCIPAL_MD)
        open_reply = await _unary(channel, OPEN)(b"", metadata=PRINCIPAL_MD)

    assert exc.value.code() == grpc.StatusCode.RESOURCE_EXHAUSTED
    assert dict(exc.value.trailing_metadata())["retry-after"] == "7"
    assert limiter.keys == ["user:u1"]
    assert open_reply == b"u1"


async def test_reflection_lists_registered_services_and_health():
    from grpc_reflection.v1alpha import reflection_pb2, reflection_pb2_grpc

    async with _serving() as channel:
        stub = reflection_pb2_grpc.ServerReflectionStub(channel)
        call = stub.ServerReflectionInfo(
            iter([reflection_pb2.ServerReflectionRequest(list_services="")])
        )
        reply = await call.read()

    names = {s.name for s in reply.list_services_response.service}
    assert {SERVICE, "grpc.health.v1.Health"} <= names
