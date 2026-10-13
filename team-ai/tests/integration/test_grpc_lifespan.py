"""API lifespan: gRPC starts after service composition and stops before the runtime."""

from __future__ import annotations

import socket
from contextlib import AsyncExitStack

import grpc
import pytest
from grpc_health.v1 import health_pb2, health_pb2_grpc

from app.bootstrap.application import create_app
from app.bootstrap.resources import ApplicationResources
from app.core import grpc_metrics
from app.core.config import Settings
from app.transport.grpc.registry import GrpcServiceRegistration
from tests.factories import api_client_for, build_test_settings

ECHO = "/t.Svc/Echo"
PRINCIPAL_MD = (
    ("x-principal-id", "u1"),
    ("x-principal-type", "user"),
    ("x-principal-scopes", "listing.read"),
)


def _free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


async def _echo(request: bytes, context: grpc.aio.ServicerContext) -> bytes:
    return b"pong"


def _factory(resources: ApplicationResources, settings: Settings):
    handler = grpc.method_handlers_generic_handler(
        "t.Svc", {"Echo": grpc.unary_unary_rpc_method_handler(_echo)}
    )
    return (
        GrpcServiceRegistration(
            service_name="t.Svc",
            register=lambda server: server.add_generic_rpc_handlers((handler,)),
        ),
    )


def _grpc_settings(port: int, **overrides: object) -> Settings:
    return build_test_settings(
        GRPC_HOST="127.0.0.1",
        GRPC_PORT=port,
        GRPC_GRACE_SECONDS=0.0,
        **overrides,
    )


async def _health(port: int) -> int:
    async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
        stub = health_pb2_grpc.HealthStub(channel)
        reply = await stub.Check(health_pb2.HealthCheckRequest(), timeout=2)
    return reply.status


async def test_registered_service_is_reachable_through_interceptors():
    port = _free_port()
    app = create_app(settings=_grpc_settings(port), grpc_services_factory=_factory)

    async with api_client_for(app):
        async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
            call = channel.unary_unary(
                ECHO, request_serializer=bytes, response_deserializer=bytes
            )
            reply = await call(b"", metadata=PRINCIPAL_MD, timeout=2)
        status = await _health(port)

    assert reply == b"pong"
    assert status == health_pb2.HealthCheckResponse.SERVING


async def test_factory_receives_composed_resources():
    seen: list[ApplicationResources] = []

    def factory(resources: ApplicationResources, settings: Settings):
        seen.append(resources)
        return ()

    app = create_app(
        settings=_grpc_settings(_free_port()), grpc_services_factory=factory
    )
    async with api_client_for(app):
        assert seen == [app.state.resources]
        assert app.state.resources.completion_service is not None


async def test_not_started_without_init_resources():
    port = _free_port()
    app = create_app(
        settings=_grpc_settings(port),
        grpc_services_factory=_factory,
        init_resources=False,
    )
    async with api_client_for(app):
        with pytest.raises(grpc.aio.AioRpcError):
            await _health(port)


async def test_server_stops_before_runtime_resources_close():
    port = _free_port()
    observed: list[str] = []

    class RecordingAddon:
        name = "recording"

        def is_enabled(self, settings: Settings) -> bool:
            return True

        async def open(self, resources, settings, stack: AsyncExitStack) -> None:
            async def closed() -> None:
                try:
                    await _health(port)
                except grpc.aio.AioRpcError:
                    observed.append("grpc_down_before_runtime_close")
                else:
                    observed.append("grpc_still_up")

            stack.push_async_callback(closed)

    app = create_app(
        settings=_grpc_settings(port),
        grpc_services_factory=_factory,
        resource_addons=(RecordingAddon(),),
    )
    async with api_client_for(app):
        assert await _health(port) == health_pb2.HealthCheckResponse.SERVING

    assert observed == ["grpc_down_before_runtime_close"]


@pytest.mark.parametrize("path", ["/metrics", "/api/v1/metrics"])
async def test_metrics_endpoint_exposes_request_counter(path):
    grpc_metrics.reset()
    port = _free_port()
    app = create_app(settings=_grpc_settings(port), grpc_services_factory=_factory)

    async with api_client_for(app) as client:
        await _health(port)
        await _health(port)
        response = await client.get(path)

    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/plain; version=0.0.4")
    assert "# TYPE teamai_grpc_requests_total counter" in response.text
    assert "teamai_grpc_requests_total 2" in response.text
