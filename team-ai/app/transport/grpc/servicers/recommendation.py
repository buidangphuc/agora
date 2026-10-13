"""RecommendationService gRPC servicer: a thin transport over ``RecommendationService``.

``Recommend`` needs ``listing.read``, which the gateway forwards for anyone who may browse
listings, anonymous visitors included. It is not rate limited. The caller, not the request
body, decides whose recommendations are served (see ``_bound_identity``).
"""

from __future__ import annotations

import uuid

import grpc
from loguru import logger

from app.core.errors import ServiceUnavailableError
from app.modules.business.recommendation.schemas import RecommendationQuery
from app.modules.business.recommendation.service import RecommendationService
from app.modules.platform.identity.schemas import Principal
from app.transport.grpc._pb.platform.recommendation.v1 import (
    recommendation_pb2,
    recommendation_pb2_grpc,
)
from app.transport.grpc.context import ensure_scopes
from app.transport.grpc.registry import GrpcServiceRegistration
from app.transport.grpc.scopes import ADMIN_SCOPE, LISTING_READ_SCOPE

_SERVICE_NAME = "platform.recommendation.v1.RecommendationService"

# Contexts without an entry leave the placement to the service.
_PLACEMENT_BY_CONTEXT = {
    recommendation_pb2.RECOMMENDATION_CONTEXT_HOMEPAGE: "home_feed",
    recommendation_pb2.RECOMMENDATION_CONTEXT_SIMILAR_ITEMS: "similar_items",
    recommendation_pb2.RECOMMENDATION_CONTEXT_CART: "cart_cross_sell",
}


def _bound_identity(
    principal: Principal, user_id: str, anonymous_id: str
) -> tuple[str, str]:
    """Bind the subject to the caller: admin and service pass the request through, a user is
    served as itself, an anonymous caller keeps only its device id."""
    if ADMIN_SCOPE in principal.scopes or principal.type == "service":
        return user_id, anonymous_id
    if principal.type == "user":
        return principal.id, ""
    return "", anonymous_id


class RecommendationServicer(recommendation_pb2_grpc.RecommendationServiceServicer):
    def __init__(self, service: RecommendationService) -> None:
        self._service = service

    async def Recommend(
        self,
        request: recommendation_pb2.RecommendRequest,
        context: grpc.aio.ServicerContext,
    ) -> recommendation_pb2.RecommendResponse:
        principal = await ensure_scopes(context, LISTING_READ_SCOPE)

        user_id, anonymous_id = _bound_identity(
            principal, request.user_id, request.anonymous_id
        )
        try:
            result = await self._service.recommend(
                RecommendationQuery(
                    user_id=user_id,
                    anonymous_id=anonymous_id,
                    seed_listing_id=request.seed_listing_id,
                    placement_id=_PLACEMENT_BY_CONTEXT.get(request.context, ""),
                    limit=request.limit,
                )
            )
        except ServiceUnavailableError as exc:
            await context.abort(grpc.StatusCode.UNAVAILABLE, exc.message)
            raise AssertionError("unreachable") from exc  # abort raises

        request_id = uuid.uuid4().hex
        # One line per response so impressions and clicks can be joined to what was served.
        # Ids only: no user id, no token.
        logger.bind(
            request_id=request_id,
            placement_id=result.placement_id,
            model_version=result.model_version,
            listing_ids=[item.listing_id for item in result.items],
            fallback=result.fallback,
            principal_type=principal.type,
        ).info("recs.served")
        return recommendation_pb2.RecommendResponse(
            items=[
                recommendation_pb2.RecommendedItem(
                    listing_id=item.listing_id, score=item.score, rank=item.rank
                )
                for item in result.items
            ],
            model_version=result.model_version,
            placement_id=result.placement_id,
            request_id=request_id,
        )


def recommendation_service_registration(
    service: RecommendationService,
) -> GrpcServiceRegistration:
    servicer = RecommendationServicer(service)

    def register(server: grpc.aio.Server) -> None:
        recommendation_pb2_grpc.add_RecommendationServiceServicer_to_server(
            servicer, server
        )

    return GrpcServiceRegistration(_SERVICE_NAME, register)
