"""Recommendation service client (surface-recommendations).

Wraps the gateway Connect/JSON RPC
`POST /platform.recommendation.v1.RecommendationService/Recommend`, which the
gateway forwards read-only to team-ai over gRPC (no gateway-side business logic).
The frontend reaches recommendations only through this gateway path — never
team-ai directly — so this client exercises the SAME path the browser's Server
Action uses, letting a scenario prove the row is sourced from the real gateway →
team-ai chain (not a client-side mock).

The RPC returns listing ids + ranks only (recommendation never owns listing
content, Rule 3); card hydration happens in the frontend via the listing client.
"""

from __future__ import annotations

from typing import Any

from src.constants import gateway_endpoints as ep

from .base_service import BaseService

# Proto RecommendationContext enum names (JSON transport uses the enum string).
CONTEXT_UNSPECIFIED = "RECOMMENDATION_CONTEXT_UNSPECIFIED"
CONTEXT_HOMEPAGE = "RECOMMENDATION_CONTEXT_HOMEPAGE"
CONTEXT_SIMILAR_ITEMS = "RECOMMENDATION_CONTEXT_SIMILAR_ITEMS"


class RecommendationService(BaseService):
    def recommend(
        self,
        *,
        user_id: str = "",
        anonymous_id: str = "",
        seed_listing_id: str = "",
        context: str = CONTEXT_UNSPECIFIED,
        limit: int = 10,
    ) -> dict[str, Any]:
        """Call Recommend through the gateway. Raises GatewayError on non-2xx.

        Returns the raw Connect/JSON response: `{"items": [{"listingId", "rank",
        "score"}...], "modelVersion": "..."}`. Callers assert on `items`.
        """
        payload: dict[str, Any] = {
            "userId": user_id,
            "anonymousId": anonymous_id,
            "seedListingId": seed_listing_id,
            "context": context,
            "limit": limit,
        }
        return self.post(ep.RECOMMENDATION_RECOMMEND, payload)

    def recommended_ids(
        self,
        *,
        seed_listing_id: str = "",
        context: str = CONTEXT_UNSPECIFIED,
        limit: int = 10,
    ) -> list[str]:
        """Best-first listing ids from Recommend (empty list when none)."""
        data = self.recommend(seed_listing_id=seed_listing_id, context=context, limit=limit)
        items = data.get("items") or []
        ranked = sorted(items, key=lambda it: it.get("rank", 0))
        return [it.get("listingId", "") for it in ranked if it.get("listingId")]
