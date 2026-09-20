"""Conversion Rate (CVR) and Expected Gross Merchandise Value (eGMV) ranking models."""

from __future__ import annotations

import math
from typing import Any

from recsys.ranker.features import CandidateFeatures, extract_candidate_features


def sigmoid(x: float) -> float:
    """Stable sigmoid function."""
    if x >= 0:
        z = math.exp(-x)
        return 1.0 / (1.0 + z)
    z = math.exp(x)
    return z / (1.0 + z)


class CVRModel:
    """Predicts conversion probability p(Purchase = 1 | Interaction)."""

    def __init__(self, weights: list[float] | None = None, bias: float = -1.5) -> None:
        # Default learned weights for cvr_vector:
        # [item_cvr, cart_to_order_ratio, user_cvr, normalized_price, price_ratio, popularity, category_match]
        self.weights = weights or [2.5, 3.0, 2.0, -0.5, -0.4, 0.8, 0.6]
        self.bias = bias

    def predict_p_cvr(self, cvr_vector: list[float]) -> float:
        """Computes calibrated conversion probability in [0, 1]."""
        logit = self.bias
        for val, w in zip(cvr_vector, self.weights, strict=False):
            logit += val * w
        return sigmoid(logit)


class eGMVRanker:
    """Ranks candidates by expected commercial value (eGMV = pCTR * (pCVR)^beta * Price^gamma)."""

    def __init__(
        self,
        cvr_model: CVRModel | None = None,
        ctr_weights: list[float] | None = None,
        beta: float = 1.0,
        gamma: float = 0.5,
    ) -> None:
        self.cvr_model = cvr_model or CVRModel()
        self.ctr_weights = ctr_weights or [0.35, 0.20, 0.15, 0.05, 0.05, 0.10, 0.10]
        self.beta = beta
        self.gamma = gamma

    def predict_p_ctr(self, feature_vector: list[float]) -> float:
        """Estimate CTR probability from first 7 standard ranker features."""
        score = 0.0
        for val, w in zip(feature_vector[: len(self.ctr_weights)], self.ctr_weights, strict=False):
            score += val * w
        # Transform score into pseudo-probability
        return sigmoid(score - 0.5)

    def score_candidate(self, features: CandidateFeatures, raw_price: float) -> tuple[float, float, float]:
        """Returns (final_score, p_ctr, p_cvr)."""
        p_ctr = self.predict_p_ctr(features.to_vector())
        p_cvr = self.cvr_model.predict_p_cvr(features.to_cvr_vector())
        
        # Guard against 0 or negative prices
        effective_price = max(1.0, raw_price)
        price_factor = math.pow(effective_price / 100000.0, self.gamma)  # Normalized around 100k VND
        
        # eGMV = pCTR * (pCVR)^beta * Price^gamma
        final_score = p_ctr * math.pow(p_cvr, self.beta) * price_factor
        return final_score, p_ctr, p_cvr

    def rank_candidates(
        self,
        candidates: list[dict[str, Any] | tuple[str, float]],
        user_context: dict[str, Any] | None = None,
        item_features_map: dict[str, dict[str, Any]] | None = None,
        top_k: int = 10,
    ) -> list[dict[str, Any]]:
        """Ranks candidates by eGMV score."""
        scored: list[dict[str, Any]] = []
        item_features_map = item_features_map or {}

        for cand in candidates:
            lid = cand[0] if isinstance(cand, tuple) else cand.get("listing_id", "")
            raw_price = float(cand[1] if isinstance(cand, tuple) else cand.get("price", 0.0))
            item_store_feat = item_features_map.get(lid, {})
            if raw_price <= 0:
                raw_price = float(item_store_feat.get("price", 100000.0))

            feat = extract_candidate_features(
                cand,
                user_context=user_context,
                item_store_features=item_store_feat,
            )
            score, p_ctr, p_cvr = self.score_candidate(feat, raw_price)
            scored.append({
                "listing_id": feat.listing_id,
                "score": score,
                "p_ctr": p_ctr,
                "p_cvr": p_cvr,
            })

        scored.sort(key=lambda x: -x["score"])
        return scored[:top_k]
