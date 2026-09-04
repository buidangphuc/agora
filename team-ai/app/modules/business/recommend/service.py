"""RecommendationService — the two-stage serving pipeline (pure orchestration).

Resolution order (design.md):
  1. logged-in with a Redis pre-computed cache hit  -> serve the cached list
  2. cache miss/anonymous WITH a seed_listing_id     -> Qdrant ANN "similar items"
  3. no cache and no seed (or any datastore error)   -> popularity fallback

Every stage feeds the same stage-2 ``rank_and_filter`` (drop seed, drop
out-of-stock, dedupe, Top-K). The pipeline is fail-open: a cache error, a
retrieval error, or a retrieval timeout degrades to the next stage rather than
failing the RPC. A startup collection-contract mismatch is the one hard failure
— it raises ``ServiceUnavailableError`` so the servicer aborts UNAVAILABLE
(loud, not a silent empty result).
"""

from __future__ import annotations

import asyncio
from typing import TYPE_CHECKING

from loguru import logger

from app.core.errors import ServiceUnavailableError
from app.modules.business.recommend.ranking import rank_and_filter
from app.modules.business.recommend.schemas import (
    Candidate,
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
        candidate_top_k: int = 100,
        result_top_k: int = 10,
        retrieve_timeout_ms: int = 15,
        model_version: str = "serving-fallback",
        collection_ok: bool = True,
    ) -> None:
        self._backend = backend
        self._cache = cache
        self._candidate_top_k = candidate_top_k
        self._result_top_k = result_top_k
        self._retrieve_timeout_ms = retrieve_timeout_ms
        self._model_version = model_version
        self._collection_ok = collection_ok

    async def recommend(self, query: RecommendQuery) -> RecommendResult:
        if not self._collection_ok:
            # Contract break with the training job — fail loud, don't serve empty.
            raise ServiceUnavailableError("recommendation collection contract mismatch")

        limit = query.limit or self._result_top_k

        # Stage: Redis pre-computed fast path (logged-in only).
        if not query.is_anonymous:
            cached = await self._cache.get_user_candidates(query.user_id)
            if cached:
                items = rank_and_filter(cached, query, limit)
                if items:
                    return self._result(items, "cache")

        # Stage: Qdrant ANN around the seed listing (PDP "similar items").
        if query.seed_listing_id:
            candidates = await self._retrieve_similar(query.seed_listing_id)
            if candidates:
                items = rank_and_filter(candidates, query, limit)
                if items:
                    return self._result(items, "ann")

        # Stage: popularity fallback so the row is never empty.
        popular = await self._popular()
        items = rank_and_filter(popular, query, limit)
        return self._result(items, "popular")

    async def _retrieve_similar(self, seed_listing_id: str) -> list[Candidate]:
        """Bounded ANN retrieval; timeout or datastore error -> [] (fall through)."""
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
        except Exception as exc:  # fail-open on any datastore error
            logger.warning("recs.retrieve.failed seed={} err={}", seed_listing_id, exc)
            return []

    async def _popular(self) -> list[Candidate]:
        # Prefer the training job's maintained popularity list in Redis; fall
        # back to the backend's own popular query. Both are fail-open.
        cached = await self._cache.get_popular_candidates()
        if cached:
            return cached
        try:
            return await self._backend.popular(top_k=self._candidate_top_k)
        except Exception as exc:  # fail-open — return empty rather than raise
            logger.warning("recs.popular.failed err={}", exc)
            return []

    def _result(self, items: list, source: str) -> RecommendResult:
        return RecommendResult(
            items=items, model_version=self._model_version, source=source
        )
