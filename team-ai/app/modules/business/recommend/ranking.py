"""Stage-2 ranking + business-rule filtering (pure, no I/O).

Provides Cosine and GBDT ranking adapters with FeatureStore and NearlineSignal port integration.
"""

from __future__ import annotations

from collections.abc import Iterable
from typing import Any, Protocol

from app.modules.business.recommend.schemas import (
    Candidate,
    RecommendedItem,
    RecommendQuery,
)


class FeatureStorePort(Protocol):
    async def get_item_features_batch(self, listing_ids: list[str]) -> dict[str, dict[str, Any]]:
        ...


class NearlineSignalPort(Protocol):
    def get_debiased_ctr(self, listing_id: str) -> float:
        ...


class InMemoryFeatureStore:
    """In-memory feature store adapter for offline testing and local execution."""

    def __init__(self, initial_data: dict[str, dict[str, Any]] | None = None) -> None:
        self._items: dict[str, dict[str, Any]] = initial_data or {}

    def set_item_features(self, listing_id: str, features: dict[str, Any]) -> None:
        self._items[listing_id] = features

    async def get_item_features_batch(self, listing_ids: list[str]) -> dict[str, dict[str, Any]]:
        return {lid: self._items[lid] for lid in listing_ids if lid in self._items}


class InMemoryNearlineStore:
    """In-memory nearline signal store adapter for position-debiased CTR."""

    def __init__(self, ctr_map: dict[str, float] | None = None) -> None:
        self._ctr_map: dict[str, float] = ctr_map or {}

    def set_debiased_ctr(self, listing_id: str, ctr: float) -> None:
        self._ctr_map[listing_id] = ctr

    def get_debiased_ctr(self, listing_id: str) -> float:
        return self._ctr_map.get(listing_id, 0.0)


class RankerPort(Protocol):
    def rank_candidates(
        self,
        candidates: Iterable[Candidate],
        query: RecommendQuery,
        item_features_map: dict[str, dict[str, Any]] | None = None,
        nearline_store: NearlineSignalPort | None = None,
        limit: int = 10,
    ) -> list[RecommendedItem]:
        ...


class CosineRankerAdapter:
    """Default baseline ranker sorting purely by cosine retrieval score."""

    def rank_candidates(
        self,
        candidates: Iterable[Candidate],
        query: RecommendQuery,
        item_features_map: dict[str, dict[str, Any]] | None = None,
        nearline_store: NearlineSignalPort | None = None,
        limit: int = 10,
    ) -> list[RecommendedItem]:
        return rank_and_filter(candidates, query, limit)


class GBDTRankerAdapter:
    """Gradient-Boosted Decision Tree ranker with nearline position-debiased CTR integration."""

    def __init__(self, weights: list[float] | None = None) -> None:
        # Default weights: [similarity, category_match, popularity, price_fit, freshness, ctr, cvr]
        self.weights = weights or [0.30, 0.25, 0.15, 0.05, 0.05, 0.10, 0.10]

    def _extract_vector(
        self,
        cand: Candidate,
        query: RecommendQuery,
        item_feat: dict[str, Any],
        nearline_store: NearlineSignalPort | None = None,
    ) -> tuple[list[float], str]:
        cat_match = 1.0 if (cand.category_id and cand.category_id == query.category_id) else float(item_feat.get("category_match", 0.0))
        pop = float(item_feat.get("popularity_score", 50.0)) / 100.0
        price = float(item_feat.get("price", 50.0)) / 1000.0
        freshness = float(item_feat.get("freshness_score", 0.8))

        # Check nearline position-debiased CTR
        ctr_source = "fallback"
        ctr = float(item_feat.get("historical_ctr", 0.0))
        if nearline_store is not None:
            nearline_ctr = nearline_store.get_debiased_ctr(cand.listing_id)
            if nearline_ctr > 0:
                ctr = nearline_ctr
                ctr_source = "nearline"

        cvr = float(item_feat.get("conversion_rate", 0.0))

        vec = [
            cand.score,
            cat_match,
            min(1.0, max(0.0, pop)),
            min(1.0, max(0.0, price)),
            min(1.0, max(0.0, freshness)),
            min(1.0, max(0.0, ctr)),
            min(1.0, max(0.0, cvr)),
        ]
        return vec, ctr_source

    def _predict(self, vec: list[float]) -> float:
        score = 0.0
        for v, w in zip(vec, self.weights, strict=False):
            score += v * w
        # Synergy boost: strong category match + strong CTR
        if len(vec) >= 6 and vec[1] > 0.5 and vec[5] > 0.05:
            score += 0.20
        return score

    def rank_candidates(
        self,
        candidates: Iterable[Candidate],
        query: RecommendQuery,
        item_features_map: dict[str, dict[str, Any]] | None = None,
        nearline_store: NearlineSignalPort | None = None,
        limit: int = 10,
    ) -> list[RecommendedItem]:
        item_features_map = item_features_map or {}
        seed = query.seed_listing_id
        filtered_cands: list[Candidate] = []
        for cand in candidates:
            if not cand.listing_id or cand.listing_id == seed or not cand.in_stock:
                continue
            filtered_cands.append(cand)

        if not filtered_cands:
            return []

        scored: list[tuple[Candidate, float]] = []
        for cand in filtered_cands:
            feat = item_features_map.get(cand.listing_id, {})
            vec, _source = self._extract_vector(cand, query, feat, nearline_store=nearline_store)
            score = self._predict(vec)
            scored.append((cand, score))

        scored.sort(key=lambda x: -x[1])
        res_limit = limit if limit > 0 else len(scored)
        return [
            RecommendedItem(listing_id=cand.listing_id, score=score, rank=rank)
            for rank, (cand, score) in enumerate(scored[:res_limit], start=1)
        ]


def rank_and_filter(
    candidates: Iterable[Candidate],
    query: RecommendQuery,
    result_top_k: int,
) -> list[RecommendedItem]:
    """Pure cosine retrieval ranking and business-rule filtering."""
    seed = query.seed_listing_id
    best: dict[str, Candidate] = {}
    for cand in candidates:
        if not cand.listing_id:
            continue
        if cand.listing_id == seed:
            continue
        if not cand.in_stock:
            continue
        existing = best.get(cand.listing_id)
        if existing is None or cand.score > existing.score:
            best[cand.listing_id] = cand

    ordered = sorted(best.values(), key=lambda c: c.score, reverse=True)
    limit = result_top_k if result_top_k > 0 else len(ordered)
    return [
        RecommendedItem(listing_id=c.listing_id, score=c.score, rank=rank)
        for rank, c in enumerate(ordered[:limit], start=1)
    ]
