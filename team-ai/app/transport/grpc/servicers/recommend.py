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

import uuid
from collections.abc import Callable
from typing import TYPE_CHECKING

import grpc
from loguru import logger

from app.core.errors import ServiceUnavailableError
from app.modules.business.recommend.schemas import RecommendQuery
from app.modules.platform.identity.schemas import Principal
from app.transport.grpc._pb.platform.recommendation.v1 import (  # type: ignore[import-not-found]
    recommendation_pb2,
    recommendation_pb2_grpc,
)
from app.transport.grpc.context import ensure_scopes

if TYPE_CHECKING:
    from app.modules.business.recommend.service import RecommendationService

RecommendationProvider = Callable[[], "RecommendationService | None"]

# RecommendationContext → placement (config/placements.yaml); unset/unknown
# contexts leave the placement to the module's own default.
_PLACEMENT_BY_CONTEXT = {
    recommendation_pb2.RECOMMENDATION_CONTEXT_HOMEPAGE: "home_feed",
    recommendation_pb2.RECOMMENDATION_CONTEXT_SIMILAR_ITEMS: "similar_items",
    recommendation_pb2.RECOMMENDATION_CONTEXT_CART: "cart_cross_sell",
}


def _bound_identity(
    principal: Principal, user_id: str, anonymous_id: str
) -> tuple[str, str]:
    """Bind the recommendation subject to the caller, not to the request body.

    A ``user`` principal is always served as itself (the request ``user_id`` and
    ``anonymous_id`` are ignored); an anonymous principal can never claim a user
    (``user_id`` dropped, device ``anonymous_id`` kept); ``admin`` or ``service``
    callers may request on behalf of a user.
    """
    if "admin" in principal.scopes or principal.type == "service":
        return user_id, anonymous_id
    if principal.type == "user":
        return principal.id, ""
    return "", anonymous_id


class RecommendationServicer(recommendation_pb2_grpc.RecommendationServiceServicer):
    def __init__(self, provider: RecommendationProvider) -> None:
        self._provider = provider

    async def Recommend(
        self,
        request: recommendation_pb2.RecommendRequest,
        context: grpc.aio.ServicerContext,
    ) -> recommendation_pb2.RecommendResponse:
        # Recommendations are listing ids for anyone who may browse listings
        # (anonymous visitors included), so the public read scope the gateway
        # forwards gates them; no identity role grants a dedicated scope.
        principal = await ensure_scopes(context, "listing.read")

        service = self._provider()
        if service is None:
            await context.abort(
                grpc.StatusCode.UNAVAILABLE,
                "recommendations are not enabled (RECS_ENABLED=false)",
            )
            raise AssertionError("unreachable")

        user_id, anonymous_id = _bound_identity(
            principal, request.user_id, request.anonymous_id
        )
        query = RecommendQuery(
            user_id=user_id,
            anonymous_id=anonymous_id,
            seed_listing_id=request.seed_listing_id,
            context=recommendation_pb2.RecommendationContext.Name(request.context),
            limit=request.limit,
            placement_id=_PLACEMENT_BY_CONTEXT.get(request.context, ""),
        )
        try:
            result = await service.recommend(query)
        except ServiceUnavailableError as exc:
            await context.abort(grpc.StatusCode.UNAVAILABLE, exc.message)
            raise AssertionError("unreachable") from exc

        request_id = uuid.uuid4().hex
        placement_id = getattr(result, "placement_id", "") or ""
        # One line per response so impressions/clicks can be joined to what was
        # served. Ids only: no user id, no token.
        logger.bind(
            request_id=request_id,
            placement_id=placement_id,
            model_version=result.model_version,
            listing_ids=[item.listing_id for item in result.items],
            fallback=bool(getattr(result, "fallback", False)),
            principal_type=principal.type,
        ).info("recs.served")

        return recommendation_pb2.RecommendResponse(
            items=[
                recommendation_pb2.RecommendedItem(
                    listing_id=item.listing_id,
                    score=item.score,
                    rank=item.rank,
                )
                for item in result.items
            ],
            model_version=result.model_version,
            placement_id=placement_id,
            request_id=request_id,
        )
