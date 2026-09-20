"""RecommendationService — the multi-placement serving pipeline with fallback ladder (ADR-0012).

Resolution order and configuration is driven by PlacementRegistry (WHAT layer) + Execution Strategies (HOW layer).
Integrates GBDT Ranker, FeatureStore, and NearlineSignal ports with graceful degradation.
"""

from __future__ import annotations

import asyncio
import time
from typing import TYPE_CHECKING, Any

from loguru import logger

from app.core.errors import ServiceUnavailableError
from app.modules.business.recommend.placement_config import PlacementRegistry
from app.modules.business.recommend.ranking import (
    CosineRankerAdapter,
    FeatureStorePort,
    GBDTRankerAdapter,
    InMemoryFeatureStore,
    InMemoryNearlineStore,
    NearlineSignalPort,
    RankerPort,
    rank_and_filter,
)
from app.modules.business.recommend.schemas import (
    Candidate,
    RecommendedItem,
    RecommendQuery,
    RecommendResult,
)

if TYPE_CHECKING:
    from app.modules.business.recommend.backends import RetrievalBackend
    from app.modules.business.recommend.cache import PrecomputedCache


class RecommendationService:
    def __init__(
        self,
        *,
        backend: RetrievalBackend,
        cache: PrecomputedCache,
        registry: PlacementRegistry | None = None,
        feature_store: FeatureStorePort | None = None,
        nearline_store: NearlineSignalPort | None = None,
        ranker: RankerPort | None = None,
        candidate_top_k: int = 100,
        result_top_k: int = 10,
        retrieve_timeout_ms: int = 25,
        model_version: str = "serving-fallback",
        collection_ok: bool = True,
    ) -> None:
        self._backend = backend
        self._cache = cache
        self._registry = registry or PlacementRegistry()
        self._feature_store = feature_store or InMemoryFeatureStore()
        self._nearline_store = nearline_store or InMemoryNearlineStore()
        self._gbdt_ranker = ranker or GBDTRankerAdapter()
        self._cosine_ranker = CosineRankerAdapter()
        self._candidate_top_k = candidate_top_k
        self._result_top_k = result_top_k
        self._retrieve_timeout_ms = retrieve_timeout_ms
        self._model_version = model_version
        self._collection_ok = collection_ok

    async def recommend(self, query: RecommendQuery) -> RecommendResult:
        if not self._collection_ok:
            raise ServiceUnavailableError("recommendation collection contract mismatch")

        start_time = time.perf_counter()
        placement_id = query.placement_id or "home_feed"
        config = self._registry.get(placement_id)
        limit = query.limit or config.result_limit or self._result_top_k

        ladder_history: list[dict[str, Any]] = []

        # Ladder traversal driven by placement configuration
        for step in config.candidate_ladder:
            strategy = step.strategy
            tier = step.tier
            min_candidates = step.min_candidates

            candidates: list[Candidate] = []
            source = "unknown"
            status = "real"

            if strategy == "user_precomputed" and not query.is_anonymous:
                cached = await self._cache.get_user_candidates(query.user_id)
                if cached:
                    candidates = cached
                    source = "cache"
                    status = "cached"

            elif strategy == "seed_vector_similarity" and query.seed_listing_id:
                candidates = await self._retrieve_similar(query.seed_listing_id)
                source = "ann"
                status = "real"

            elif strategy == "cart_cross_similarity" and query.cart_listing_ids:
                for cart_item in query.cart_listing_ids[:3]:
                    sim = await self._retrieve_similar(cart_item)
                    candidates.extend(sim)
                source = "ann"
                status = "real"

            elif strategy == "category_popular" and query.category_id:
                candidates = await self._popular()
                source = "popular"
                status = "degraded"

            elif strategy == "global_popular":
                candidates = await self._popular()
                source = "popular"
                status = "fallback"

            ladder_history.append({
                "tier": tier,
                "strategy": strategy,
                "candidates_found": len(candidates),
            })

            if len(candidates) >= min_candidates:
                items, rank_status, hit_count, rank_source = await self._rank_candidates(
                    candidates, query, config, limit
                )
                if rank_status == "degraded":
                    status = "degraded"

                if items:
                    elapsed_ms = (time.perf_counter() - start_time) * 1000.0
                    explain_data: dict[str, Any] = {}
                    if query.include_explain:
                        explain_data = {
                            "placement": placement_id,
                            "tier_chosen": tier,
                            "strategy": strategy,
                            "latency_ms": round(elapsed_ms, 2),
                            "ladder_traversed": ladder_history,
                            "ranking_model": config.ranking_model,
                            "ranking_source": rank_source,
                            "featurestore_hit_count": hit_count,
                            "nearline_enabled": self._nearline_store is not None,
                            "status": status,
                        }
                    return self._result(
                        items=items,
                        source=source,
                        placement_id=placement_id,
                        fallback_tier=tier,
                        status=status,
                        explain=explain_data,
                    )

        # Fallback to absolute floor
        popular = await self._popular()
        items = rank_and_filter(popular, query, limit)
        elapsed_ms = (time.perf_counter() - start_time) * 1000.0
        explain_data = {}
        if query.include_explain:
            explain_data = {
                "placement": placement_id,
                "tier_chosen": "tier4_global_popular",
                "strategy": "floor_popular",
                "latency_ms": round(elapsed_ms, 2),
                "ladder_traversed": ladder_history,
                "ranking_model": "cosine_rank",
                "ranking_source": "cosine",
                "featurestore_hit_count": 0,
                "nearline_enabled": self._nearline_store is not None,
                "status": "fallback",
            }
        return self._result(
            items=items,
            source="popular",
            placement_id=placement_id,
            fallback_tier="tier4_global_popular",
            status="fallback",
            explain=explain_data,
        )

    async def _rank_candidates(
        self,
        candidates: list[Candidate],
        query: RecommendQuery,
        config: Any,
        limit: int,
    ) -> tuple[list[RecommendedItem], str, int, str]:
        """Rank candidates using configured ranking model with FeatureStore & Nearline enrichment."""
        item_features: dict[str, dict[str, Any]] = {}
        hit_count = 0
        ranking_model = getattr(config, "ranking_model", "cosine_rank")
        use_fs = getattr(config, "use_featurestore", False)

        # 1. Feature store enrichment
        if use_fs and self._feature_store is not None:
            try:
                candidate_ids = [c.listing_id for c in candidates if c.listing_id]
                item_features = await self._feature_store.get_item_features_batch(candidate_ids)
                hit_count = len(item_features)
            except Exception as exc:
                logger.warning("Feature store lookup failed: {}, degrading to cosine", exc)
                return rank_and_filter(candidates, query, limit), "degraded", 0, "degraded_cosine"

        # 2. Ranking dispatch
        if ranking_model == "gbdt":
            try:
                ranked = self._gbdt_ranker.rank_candidates(
                    candidates=candidates,
                    query=query,
                    item_features_map=item_features,
                    nearline_store=self._nearline_store,
                    limit=limit,
                )
                return ranked, "ok", hit_count, "gbdt"
            except Exception as exc:
                logger.warning("GBDT ranker failed: {}, degrading to cosine", exc)
                return rank_and_filter(candidates, query, limit), "degraded", hit_count, "degraded_cosine"
        else:
            return rank_and_filter(candidates, query, limit), "ok", hit_count, "cosine"

    async def _retrieve_similar(self, seed_listing_id: str) -> list[Candidate]:
        try:
            return await asyncio.wait_for(
                self._backend.retrieve_similar(
                    seed_listing_id, top_k=self._candidate_top_k
                ),
                timeout=self._retrieve_timeout_ms / 1000,
            )
        except TimeoutError:
            logger.warning(
                "recs.retrieve.timeout seed={} budget_ms={}",
                seed_listing_id,
                self._retrieve_timeout_ms,
            )
            return []
        except Exception as exc:
            logger.warning("recs.retrieve.failed seed={} err={}", seed_listing_id, exc)
            return []

    async def _popular(self) -> list[Candidate]:
        cached = await self._cache.get_popular_candidates()
        if cached:
            return cached
        try:
            return await self._backend.popular(top_k=self._candidate_top_k)
        except Exception as exc:
            logger.warning("recs.popular.failed err={}", exc)
            return []

    def _result(
        self,
        items: list,
        source: str,
        placement_id: str = "home_feed",
        fallback_tier: str = "tier1_personalized",
        status: str = "real",
        explain: dict[str, Any] | None = None,
    ) -> RecommendResult:
        return RecommendResult(
            items=items,
            model_version=self._model_version,
            source=source,
            placement_id=placement_id,
            fallback_tier=fallback_tier,
            status=status,
            explain=explain or {},
        )
