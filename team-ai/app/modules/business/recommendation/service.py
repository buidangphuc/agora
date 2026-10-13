"""RecommendationService: the multi-placement serving pipeline with a fallback ladder (ADR-0012).

A placement (``placements.yaml``) names its candidate ladder and ranking. The ladder is walked
step by step until a step yields candidates that rank to at least one item; the popular list is
the floor. Dependencies fail open inside their adapters; any other failure takes the one fallback
path, the serving generation's popular list.
"""

import time
from collections.abc import Callable

from loguru import logger

from app.core.errors import ServiceUnavailableError
from app.modules.business.recommendation.placements import (
    Placement,
    Placements,
    Strategy,
)
from app.modules.business.recommendation.ports import (
    CandidateSource,
    FeatureStore,
    NearlineSignals,
    ServingCache,
)
from app.modules.business.recommendation.ranking.artifact import TrainedRankerLoader
from app.modules.business.recommendation.ranking.boost import apply_online_features
from app.modules.business.recommendation.ranking.linear import (
    rank_by_fixed_weights,
    rank_by_similarity,
)
from app.modules.business.recommendation.schemas import (
    SERVING_FALLBACK,
    Candidate,
    RecommendationQuery,
    RecommendationResult,
    RecommendedItem,
)

_RECHECK_S = 30.0


class RecommendationService:
    def __init__(
        self,
        *,
        source: CandidateSource,
        cache: ServingCache,
        placements: Placements,
        popularity_store: FeatureStore | None,
        attribute_store: FeatureStore | None,
        nearline: NearlineSignals | None,
        ranker_loader: TrainedRankerLoader,
        candidate_top_k: int,
        result_top_k: int,
        model_version: str,
        clock: Callable[[], float] = time.monotonic,
        recheck_s: float = _RECHECK_S,
    ) -> None:
        self._source = source
        self._cache = cache
        self._placements = placements
        self._popularity_store = popularity_store
        self._attribute_store = attribute_store
        self._nearline = nearline
        self._ranker_loader = ranker_loader
        self._candidate_top_k = candidate_top_k
        self._result_top_k = result_top_k
        self._model_version = model_version
        self._clock = clock
        self._recheck_s = recheck_s
        self._ready = False
        self._checked_at: float | None = None

    async def check_ready(self) -> bool:
        """Whether the candidate source matches the producers' contract.

        A pass is final. A failure is retried, at most every ``recheck_s``: the training job may
        create the collection, or the backend come back, after this service started.
        """
        if self._ready:
            return True
        now = self._clock()
        if self._checked_at is not None and now - self._checked_at < self._recheck_s:
            return False
        self._checked_at = now
        self._ready = await self._source.ready(await self._cache.serving_generation())
        return self._ready

    async def recommend(self, query: RecommendationQuery) -> RecommendationResult:
        """Serve a request. Only a failed contract check (a deployment error, not a runtime
        hiccup) raises ``ServiceUnavailableError``."""
        if not await self.check_ready():
            raise ServiceUnavailableError("recommendation collection contract mismatch")
        # Fix the serving generation once: the Redis lists and the Qdrant collection of this
        # request must come from the same generation even if the pointer moves mid-request.
        generation = await self._cache.serving_generation()
        placement_id = self._placement_id(query)
        placement = self._placements.get(placement_id)
        limit = query.limit or placement.result_limit or self._result_top_k
        try:
            return await self._serve(query, placement_id, placement, limit, generation)
        except Exception as exc:
            logger.warning("recs.recommend.failed err={}", exc)
            return await self._popular_result(
                query, placement_id, limit, generation, SERVING_FALLBACK
            )

    @staticmethod
    def _placement_id(query: RecommendationQuery) -> str:
        if query.placement_id:
            return query.placement_id
        return (
            "similar_items"
            if query.seed_listing_id and not query.user_id
            else "home_feed"
        )

    async def _serve(
        self,
        query: RecommendationQuery,
        placement_id: str,
        placement: Placement,
        limit: int,
        generation: str | None,
    ) -> RecommendationResult:
        model_version = self._model_version
        if model_version == SERVING_FALLBACK:
            model_version = await self._cache.model_version(generation) or model_version

        for step in placement.candidate_ladder:
            candidates = await self._candidates(step.strategy, query, generation)
            if len(candidates) < step.min_candidates:
                continue
            items = await self._rank(candidates, query, placement, limit, generation)
            if items:
                return RecommendationResult(
                    items=items, model_version=model_version, placement_id=placement_id
                )
        return await self._popular_result(
            query, placement_id, limit, generation, model_version
        )

    async def _candidates(
        self, strategy: Strategy, query: RecommendationQuery, generation: str | None
    ) -> list[Candidate]:
        match strategy:
            case "user_precomputed":
                if not query.user_id:
                    return []
                return await self._cache.user_candidates(query.user_id, generation)
            case "seed_vector_similarity":
                if not query.seed_listing_id:
                    return []
                return await self._source.similar(
                    query.seed_listing_id,
                    top_k=self._candidate_top_k,
                    generation=generation,
                )
            case "global_popular":
                return await self._cache.popular_candidates(generation)
            case "category_popular" | "cart_cross_similarity":
                # A query carries no category or cart (team-ai's servicer never set them),
                # so these steps never had input to act on.
                return []

    async def _rank(
        self,
        candidates: list[Candidate],
        query: RecommendationQuery,
        placement: Placement,
        limit: int,
        generation: str | None,
    ) -> list[RecommendedItem]:
        seed = query.seed_listing_id
        ids = [c.listing_id for c in candidates if c.listing_id]
        features = {}
        if placement.ranking.use_featurestore and self._popularity_store is not None:
            features = await self._popularity_store.item_features(ids)
        # With online features the ranker returns every candidate and the boost cuts to
        # ``limit`` afterwards.
        rank_limit = 0 if features else limit

        if placement.ranking.model == "gbdt":
            nearline = await self._nearline_ctrs(ids)
            trained = await self._ranker_loader.get(generation)
            if trained is not None and features:
                attributes = (
                    await self._attribute_store.item_features(list(set(ids)))
                    if self._attribute_store is not None
                    else {}
                )
                return trained.rank_candidates(
                    candidates, seed, features, attributes, nearline, limit
                )
            ranked = rank_by_fixed_weights(
                candidates, seed, features, nearline, rank_limit
            )
        else:
            ranked = rank_by_similarity(candidates, seed, rank_limit)
        return apply_online_features(ranked, features, limit)

    async def _nearline_ctrs(self, ids: list[str]) -> dict[str, float]:
        if self._nearline is None:
            return {}
        return await self._nearline.debiased_ctrs(list(set(ids)))

    async def _popular_result(
        self,
        query: RecommendationQuery,
        placement_id: str,
        limit: int,
        generation: str | None,
        model_version: str,
    ) -> RecommendationResult:
        """The popular list in retrieval order: the floor of the ladder and the fallback."""
        popular = await self._cache.popular_candidates(generation)
        return RecommendationResult(
            items=rank_by_similarity(popular, query.seed_listing_id, limit),
            model_version=model_version,
            placement_id=placement_id,
            fallback=True,
        )
