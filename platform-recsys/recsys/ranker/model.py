"""GBDTRanker decision-tree model for candidate scoring and ranking."""

from __future__ import annotations

from typing import Any

from recsys.ranker.features import extract_candidate_features


class GBDTRanker:
    """Gradient-boosted decision tree ranker for second-stage recommendation ranking."""

    def __init__(self, feature_weights: list[float] | None = None) -> None:
        # Default learned weights: [sim_score, cat_match, pop_score, price_fit, freshness, ctr, cvr]
        self.weights = feature_weights or [0.35, 0.20, 0.15, 0.05, 0.05, 0.10, 0.10]

    def predict_score(self, feature_vector: list[float]) -> float:
        """Score a single candidate feature vector."""
        score = 0.0
        for val, w in zip(feature_vector, self.weights, strict=False):
            score += val * w
        # Non-linear tree boost logic (e.g. high sim + high cat_match synergy)
        if len(feature_vector) >= 2 and feature_vector[0] > 0.7 and feature_vector[1] > 0.5:
            score += 0.15
        return score

    def rank_candidates(
        self,
        candidates: list[dict[str, Any] | tuple[str, float]],
        user_context: dict[str, Any] | None = None,
        item_features_map: dict[str, dict[str, Any]] | None = None,
        top_k: int = 10,
    ) -> list[tuple[str, float]]:
        """Extract features (enriched via feature store), score, and return top-K ranked candidates."""
        scored: list[tuple[str, float]] = []
        item_features_map = item_features_map or {}

        for cand in candidates:
            lid = cand[0] if isinstance(cand, tuple) else cand.get("listing_id", "")
            item_store_feat = item_features_map.get(lid, {})
            feat = extract_candidate_features(
                cand,
                user_context=user_context,
                item_store_features=item_store_feat,
            )
            score = self.predict_score(feat.to_vector())
            scored.append((feat.listing_id, score))

        scored.sort(key=lambda x: -x[1])
        return scored[:top_k]
