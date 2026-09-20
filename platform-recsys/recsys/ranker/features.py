"""Feature extraction for candidate re-ranking with feature store enrichment and IPS position debiasing."""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Any


def compute_ips_weight(position: int, gamma: float = 0.5) -> float:
    """Computes Inverse Propensity Score (IPS) weight for training sample."""
    pos = max(1, position)
    return float(math.pow(pos, gamma))


@dataclass
class CandidateFeatures:
    listing_id: str
    similarity_score: float
    category_match: float
    popularity_score: float
    price: float
    freshness_days: float
    historical_ctr: float = 0.0
    conversion_rate: float = 0.0
    cart_to_order_ratio: float = 0.0
    user_cvr: float = 0.0
    price_ratio: float = 1.0
    position_weight: float = 1.0

    def to_vector(self) -> list[float]:
        return [
            self.similarity_score,
            self.category_match,
            self.popularity_score,
            self.price,
            self.freshness_days,
            self.historical_ctr,
            self.conversion_rate,
            self.cart_to_order_ratio,
            self.user_cvr,
            self.price_ratio,
        ]

    def to_cvr_vector(self) -> list[float]:
        return [
            self.conversion_rate,
            self.cart_to_order_ratio,
            self.user_cvr,
            self.price,
            self.price_ratio,
            self.popularity_score,
            self.category_match,
        ]



def extract_candidate_features(
    candidate: dict[str, Any] | tuple[str, float],
    user_context: dict[str, Any] | None = None,
    item_store_features: dict[str, Any] | None = None,
    position: int = 1,
    nearline_store: Any | None = None,
) -> CandidateFeatures:
    """Extract normalized feature vector for a candidate item given user context and item store attributes."""
    user_context = user_context or {}
    item_store_features = item_store_features or {}

    if isinstance(candidate, tuple):
        listing_id, sim_score = candidate
        cat_id = item_store_features.get("category_id", "")
        pop_score = float(item_store_features.get("popularity_score", 0.0))
        price = float(item_store_features.get("price", 0.0))
        freshness = float(item_store_features.get("freshness_days", 1.0))
        ctr = float(item_store_features.get("historical_ctr", 0.0))
        cvr = float(item_store_features.get("conversion_rate", 0.0))
        cart_to_order = float(item_store_features.get("cart_to_order_ratio", 0.0))
        cat_avg_price = float(item_store_features.get("category_avg_price", price if price > 0 else 1.0))
    else:
        listing_id = candidate.get("listing_id", "")
        sim_score = float(candidate.get("score", 0.0))
        cat_id = candidate.get("category_id") or item_store_features.get("category_id", "")
        pop_val = candidate.get("popularity_score") or item_store_features.get("popularity_score", 0.0)
        pop_score = float(pop_val)
        price = float(candidate.get("price") or item_store_features.get("price", 0.0))
        freshness = float(candidate.get("freshness_days") or item_store_features.get("freshness_days", 1.0))
        ctr = float(candidate.get("historical_ctr") or item_store_features.get("historical_ctr", 0.0))
        cvr = float(candidate.get("conversion_rate") or item_store_features.get("conversion_rate", 0.0))
        cart_to_order = float(candidate.get("cart_to_order_ratio") or item_store_features.get("cart_to_order_ratio", 0.0))
        cat_avg_price = float(candidate.get("category_avg_price") or item_store_features.get("category_avg_price", price if price > 0 else 1.0))

    user_cvr = float(user_context.get("user_cvr", 0.0))
    price_ratio = (price / cat_avg_price) if cat_avg_price > 0 else 1.0

    if nearline_store is not None and hasattr(nearline_store, "get_debiased_ctr"):
        n_ctr = nearline_store.get_debiased_ctr(listing_id)
        if n_ctr > 0:
            ctr = n_ctr

    # Compute category match against user preferred categories or category affinities
    user_cats = user_context.get("category_affinities") or user_context.get("preferred_categories", {})
    if isinstance(user_cats, list):
        cat_match = 1.0 if cat_id in user_cats else 0.0
    elif isinstance(user_cats, dict):
        cat_match = float(user_cats.get(cat_id, 0.0)) / 10.0 if cat_id else 0.0
    else:
        cat_match = 0.0

    ips = compute_ips_weight(position)

    return CandidateFeatures(
        listing_id=listing_id,
        similarity_score=sim_score,
        category_match=min(1.0, max(0.0, cat_match)),
        popularity_score=min(1.0, max(0.0, pop_score / 100.0)),
        price=min(1.0, max(0.0, price / 1000.0)),
        freshness_days=max(0.0, 1.0 - (freshness / 30.0)),
        historical_ctr=min(1.0, max(0.0, ctr)),
        conversion_rate=min(1.0, max(0.0, cvr)),
        cart_to_order_ratio=min(1.0, max(0.0, cart_to_order)),
        user_cvr=min(1.0, max(0.0, user_cvr)),
        price_ratio=min(5.0, max(0.0, price_ratio)),
        position_weight=ips,
    )

