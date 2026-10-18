from __future__ import annotations

from collections.abc import AsyncIterator, Callable
from contextlib import asynccontextmanager

from loguru import logger

from app.bootstrap.resources import ApplicationResources, opened
from app.core.config import Settings
from app.modules.platform.rate_limit.factory import build_redis_rate_limiter
from app.modules.platform.rate_limit.service import RedisRateLimiter
from app.transport.grpc.registry import GrpcServiceRegistration
from app.transport.grpc.server import build_grpc_server
from app.transport.grpc.servicers.ai import ai_service_registration
from app.transport.grpc.servicers.chat import chat_service_registration
from app.transport.grpc.servicers.recommendation import (
    recommendation_service_registration,
)
from app.transport.grpc.servicers.search import search_service_registration

GrpcServicesFactory = Callable[
    [ApplicationResources, Settings], tuple[GrpcServiceRegistration, ...]
]


def default_grpc_services_factory(
    resources: ApplicationResources, settings: Settings
) -> tuple[GrpcServiceRegistration, ...]:
    return (
        ai_service_registration(
            resources.ai_assistant,
            resources.tag_classifier,
            resources.screen,
            require_ai_use=settings.AI_USE_SCOPE_REQUIRED,
        ),
        chat_service_registration(
            resources.chat, require_ai_use=settings.AI_USE_SCOPE_REQUIRED
        ),
        recommendation_service_registration(resources.recommendation),
        search_service_registration(resources.rag_service),
    )


def _build_rate_limiter(
    resources: ApplicationResources, settings: Settings
) -> RedisRateLimiter:
    return build_redis_rate_limiter(
        opened(resources.redis, "redis"),
        limit=settings.RATE_LIMIT_PRINCIPAL_PER_MINUTE,
        window_seconds=settings.RATE_LIMIT_WINDOW_SECONDS,
        prefix=f"{settings.RATE_LIMIT_REDIS_PREFIX}:grpc",
    )


@asynccontextmanager
async def serve_grpc(
    resources: ApplicationResources,
    settings: Settings,
    *,
    services_factory: GrpcServicesFactory,
) -> AsyncIterator[int]:
    """Serve gRPC for the lifetime of the block; yields the bound port.

    Runs after ``open_runtime`` composed the business services (so the factory can
    read them) and exits before the runtime closes its resources.
    """
    server = build_grpc_server(
        registrations=services_factory(resources, settings),
        rate_limiter=_build_rate_limiter(resources, settings),
    )
    port = server.add_insecure_port(f"{settings.GRPC_HOST}:{settings.GRPC_PORT}")
    await server.start()
    logger.info("grpc.server.started host={} port={}", settings.GRPC_HOST, port)
    try:
        yield port
    finally:
        await server.stop(settings.GRPC_GRACE_SECONDS)
        logger.info("grpc.server.stopped")
