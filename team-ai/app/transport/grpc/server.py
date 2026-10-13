"""Assemble the gRPC server: interceptor chain, registrations, health, reflection.

``build_grpc_server`` returns a configured but unstarted ``grpc.aio`` server. The
bootstrap layer (``app/bootstrap/grpc.py``) decides when to bind, start and stop it.
"""

from collections.abc import Iterable

import grpc
from grpc_health.v1 import health_pb2, health_pb2_grpc
from grpc_reflection.v1alpha import reflection

from app.transport.grpc.interceptors.auth import AuthInterceptor
from app.transport.grpc.interceptors.rate_limit import (
    RateLimiter,
    RateLimitInterceptor,
)
from app.transport.grpc.interceptors.tracing import TracingInterceptor
from app.transport.grpc.registry import GrpcServiceRegistration

_HEALTH_SERVICE = health_pb2.DESCRIPTOR.services_by_name["Health"].full_name


class _HealthServicer(health_pb2_grpc.HealthServicer):
    """Async health servicer that always reports SERVING.

    The stock ``grpc_health`` servicer is synchronous, so its result cannot be awaited
    by an aio server and kubelet probes fail with a TypeError.
    """

    async def Check(self, request, context):
        return health_pb2.HealthCheckResponse(
            status=health_pb2.HealthCheckResponse.SERVING
        )

    async def Watch(self, request, context):
        # An async generator: the interceptor chain wraps stream handlers as such.
        yield health_pb2.HealthCheckResponse(
            status=health_pb2.HealthCheckResponse.SERVING
        )


def build_grpc_server(
    *,
    registrations: Iterable[GrpcServiceRegistration],
    rate_limiter: RateLimiter,
) -> grpc.aio.Server:
    registrations = tuple(registrations)
    # Order matters: tracing binds the request id first, auth binds the principal,
    # and the limiter keys on that principal.
    methods = frozenset().union(*(r.rate_limited_methods for r in registrations))
    server = grpc.aio.server(
        interceptors=(
            TracingInterceptor(),
            AuthInterceptor(),
            RateLimitInterceptor(rate_limiter, methods=methods),
        )
    )
    for registration in registrations:
        registration.register(server)
    health_pb2_grpc.add_HealthServicer_to_server(_HealthServicer(), server)

    names = (
        *(r.service_name for r in registrations),
        _HEALTH_SERVICE,
        reflection.SERVICE_NAME,
    )
    reflection.enable_server_reflection(names, server)
    return server
