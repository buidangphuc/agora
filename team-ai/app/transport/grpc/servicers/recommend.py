"""RecommendationService gRPC servicer — thin transport over the recommend module.

Parses the request, enforces scope, calls ``RecommendationService.recommend``,
and maps the result back to the platform contract. All retrieval/ranking/cache
logic lives in ``app.modules.business.recommend`` (proto-free and unit-tested);
this layer only touches the generated ``recommendation_pb2`` messages — mirroring
how ``search.py`` is a thin transport over the RAG machinery.

The generated ``recommendation_pb2*`` stubs are owned by add-recommendation-contract
and vendored under ``_pb`` in CI; this file therefore imports/runs only once those
stubs exist (server.py registers it defensively until then).
"""

from __future__ import annotations

from collections.abc import Callable
from typing import TYPE_CHECKING

import grpc

from app.core.errors import ServiceUnavailableError
from app.modules.business.recommend.schemas import RecommendQuery
from app.transport.grpc._pb.platform.recommendation.v1 import (  # type: ignore[import-not-found]
    recommendation_pb2,
    recommendation_pb2_grpc,
)
from app.transport.grpc.context import ensure_scopes

if TYPE_CHECKING:
    from app.modules.business.recommend.service import RecommendationService

RecommendationProvider = Callable[[], "RecommendationService | None"]


class RecommendationServicer(recommendation_pb2_grpc.RecommendationServiceServicer):
    def __init__(self, provider: RecommendationProvider) -> None:
        self._provider = provider

    async def Recommend(
        self,
        request: recommendation_pb2.RecommendRequest,
        context: grpc.aio.ServicerContext,
    ) -> recommendation_pb2.RecommendResponse:
        await ensure_scopes(context, "recommendations:read")

        service = self._provider()
        if service is None:
            await context.abort(
                grpc.StatusCode.UNAVAILABLE,
                "recommendations are not enabled (RECS_ENABLED=false)",
            )
            raise AssertionError("unreachable")

        query = RecommendQuery(
            user_id=request.user_id,
            anonymous_id=getattr(request, "anonymous_id", ""),
            seed_listing_id=request.seed_listing_id,
            context=getattr(request, "context", ""),
            limit=getattr(request, "limit", 0),
        )
        try:
            result = await service.recommend(query)
        except ServiceUnavailableError as exc:
            await context.abort(grpc.StatusCode.UNAVAILABLE, exc.message)
            raise AssertionError("unreachable") from exc

        return recommendation_pb2.RecommendResponse(
            recommendations=[
                recommendation_pb2.RecommendedItem(
                    listing_id=item.listing_id,
                    score=item.score,
                    rank=item.rank,
                )
                for item in result.items
            ],
            model_version=result.model_version,
        )
