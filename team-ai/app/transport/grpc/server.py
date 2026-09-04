"""Assemble and serve the team-ai gRPC server (platform-core contract).

Registers SearchService + ChatService behind the tracing and auth interceptors,
plus the standard health and (optional) reflection services. ``build_grpc_server``
returns a ready-but-unstarted server so tests can drive it in-process.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import grpc
from grpc_health.v1 import health_pb2, health_pb2_grpc
from loguru import logger

from app.core.config import Settings
from app.modules.business.ai_assistant.service import AIAssistantService
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc
from app.transport.grpc._pb.platform.chat.v1 import chat_pb2, chat_pb2_grpc
from app.transport.grpc._pb.platform.search.v1 import search_pb2, search_pb2_grpc
from app.transport.grpc.chat_stream import ChatStreamer
from app.transport.grpc.interceptors.auth import AuthInterceptor
from app.transport.grpc.interceptors.tracing import TracingInterceptor
from app.transport.grpc.servicers.ai import AIServicer
from app.transport.grpc.servicers.chat import ChatServicer
from app.transport.grpc.servicers.search import SearchServicer

if TYPE_CHECKING:
    from app.transport.grpc.servicers.recommend import RecommendationProvider
    from app.transport.grpc.servicers.search import RagProvider


class _AioHealthServicer(health_pb2_grpc.HealthServicer):
    """Async gRPC health servicer for the grpc.aio server.

    The stock ``grpc_health`` ``HealthServicer`` is synchronous, so its ``Check``
    result cannot be awaited by an aio server (kubelet probes fail with a
    TypeError). This reports SERVING for every service asynchronously.
    """

    async def Check(self, request, context):  # noqa: N802 (gRPC method name)
        return health_pb2.HealthCheckResponse(
            status=health_pb2.HealthCheckResponse.SERVING
        )

    async def Watch(self, request, context):  # noqa: N802 (gRPC method name)
        await context.write(
            health_pb2.HealthCheckResponse(
                status=health_pb2.HealthCheckResponse.SERVING
            )
        )


def build_grpc_server(
    *,
    settings: Settings,
    rag_provider: RagProvider,
    chat_streamer: ChatStreamer,
    recommendation_provider: RecommendationProvider | None = None,
) -> grpc.aio.Server:
    server = grpc.aio.server(
        interceptors=(TracingInterceptor(), AuthInterceptor(settings))
    )

    search_pb2_grpc.add_SearchServiceServicer_to_server(
        SearchServicer(rag_provider), server
    )
    chat_pb2_grpc.add_ChatServiceServicer_to_server(ChatServicer(chat_streamer), server)
    ai_pb2_grpc.add_AIServiceServicer_to_server(
        AIServicer(lambda: AIAssistantService(rag_service=rag_provider())), server
    )
    recs_service_name = _register_recommendation_service(
        server, recommendation_provider
    )

    service_names = _service_names()
    if recs_service_name is not None:
        service_names = (*service_names, recs_service_name)

    # The stock grpc_health HealthServicer is synchronous; on a grpc.aio server its
    # Check return value gets awaited (TypeError), so kubelet's gRPC health probe
    # fails. Serve health with an async servicer that reports SERVING.
    health_pb2_grpc.add_HealthServicer_to_server(_AioHealthServicer(), server)

    if settings.GRPC_REFLECTION_ENABLED:
        from grpc_reflection.v1alpha import reflection

        reflection.enable_server_reflection(
            (*service_names, reflection.SERVICE_NAME), server
        )

    return server


def _register_recommendation_service(
    server: grpc.aio.Server,
    recommendation_provider: RecommendationProvider | None,
) -> str | None:
    """Register RecommendationService, tolerating the not-yet-generated stubs.

    The ``recommendation_pb2*`` stubs are owned by add-recommendation-contract and
    vendored under ``_pb`` in CI. Until they land, the import fails and the service
    is simply skipped (logged) so the rest of the server — and its tests — keep
    working. Once present, the servicer is always registered; the provider returns
    ``None`` when ``RECS_ENABLED=false`` and the servicer aborts UNAVAILABLE.
    """
    if recommendation_provider is None:
        recommendation_provider = lambda: None
    try:
        from app.transport.grpc._pb.platform.recommendation.v1 import (
            recommendation_pb2,
            recommendation_pb2_grpc,
        )
        from app.transport.grpc.servicers.recommend import RecommendationServicer
    except ImportError:
        logger.warning(
            "recommendation_pb2 not generated yet; RecommendationService not registered"
        )
        return None

    recommendation_pb2_grpc.add_RecommendationServiceServicer_to_server(
        RecommendationServicer(recommendation_provider), server
    )
    return recommendation_pb2.DESCRIPTOR.services_by_name[
        "RecommendationService"
    ].full_name


def _service_names() -> tuple[str, ...]:
    return (
        search_pb2.DESCRIPTOR.services_by_name["SearchService"].full_name,
        chat_pb2.DESCRIPTOR.services_by_name["ChatService"].full_name,
        ai_pb2.DESCRIPTOR.services_by_name["AIService"].full_name,
        health_pb2.DESCRIPTOR.services_by_name["Health"].full_name,
    )


async def serve(
    *,
    settings: Settings,
    rag_provider: RagProvider,
    chat_streamer: ChatStreamer,
    recommendation_provider: RecommendationProvider | None = None,
) -> None:
    server = build_grpc_server(
        settings=settings,
        rag_provider=rag_provider,
        chat_streamer=chat_streamer,
        recommendation_provider=recommendation_provider,
    )
    bind = f"{settings.GRPC_HOST}:{settings.GRPC_PORT}"
    server.add_insecure_port(bind)
    await server.start()
    logger.info(
        "grpc.server.started bind={} reflection={}",
        bind,
        settings.GRPC_REFLECTION_ENABLED,
    )
    try:
        await server.wait_for_termination()
    finally:
        await server.stop(settings.GRPC_GRACE_SECONDS)
        logger.info("grpc.server.stopped")
