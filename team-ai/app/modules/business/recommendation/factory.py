"""Composition root of the recommendation module.

The caller (the bootstrap addon) owns the clients: it opens them and closes them on shutdown.
``build_recommendation_service`` only wires adapters around them. Every Redis client must
decode responses (``decode_responses=True``), as the shared ``resources.redis`` does.
"""

from dataclasses import dataclass
from typing import Protocol

from qdrant_client import AsyncQdrantClient
from redis.asyncio import Redis

from app.modules.business.recommendation.adapters.featurestore import (
    RedisFeatureStore,
)
from app.modules.business.recommendation.adapters.nearline import RedisNearlineSignals
from app.modules.business.recommendation.adapters.qdrant import QdrantCandidateSource
from app.modules.business.recommendation.adapters.redis_cache import RedisServingCache
from app.modules.business.recommendation.contract.features import (
    ITEM_ATTRIBUTES_VIEW,
    ITEM_POPULARITY_VIEW,
)
from app.modules.business.recommendation.contract.keys import ServingKeys
from app.modules.business.recommendation.placements import load_placements
from app.modules.business.recommendation.ports import CandidateSource
from app.modules.business.recommendation.ranking.artifact import TrainedRankerLoader
from app.modules.business.recommendation.service import RecommendationService


class RecommendationSettings(Protocol):
    """The ``RECS_*`` settings this module reads (read-only: ``Settings`` satisfies it)."""

    @property
    def RECS_QDRANT_COLLECTION(self) -> str: ...
    @property
    def RECS_VECTOR_DIM(self) -> int: ...
    @property
    def RECS_QDRANT_DISTANCE(self) -> str: ...
    @property
    def RECS_CANDIDATE_TOP_K(self) -> int: ...
    @property
    def RECS_RESULT_TOP_K(self) -> int: ...
    @property
    def RECS_CACHE_PREFIX(self) -> str: ...
    @property
    def RECS_CACHE_SCHEMA_VERSION(self) -> str: ...
    @property
    def RECS_RETRIEVE_TIMEOUT_MS(self) -> int: ...
    @property
    def RECS_MODEL_VERSION(self) -> str: ...
    @property
    def RECS_NEARLINE_PREFIX(self) -> str: ...
    @property
    def RECS_NEARLINE_MIN_IMPRESSIONS(self) -> float: ...
    @property
    def RECS_NEARLINE_TIMEOUT_MS(self) -> int: ...


@dataclass(frozen=True, slots=True)
class RecommendationClients:
    """The app-lifetime clients of this module; the optional ones are ``None`` where their URL is not configured."""

    qdrant: "AsyncQdrantClient"
    featurestore: Redis | None
    nearline: Redis | None


def build_candidate_source(
    settings: RecommendationSettings, clients: RecommendationClients
) -> CandidateSource:
    """The Qdrant candidate source; tests swap it for ``MemoryCandidateSource``."""
    return QdrantCandidateSource(
        clients.qdrant,
        collection=settings.RECS_QDRANT_COLLECTION,
        vector_dim=settings.RECS_VECTOR_DIM,
        distance=settings.RECS_QDRANT_DISTANCE,
        timeout_s=settings.RECS_RETRIEVE_TIMEOUT_MS / 1000,
    )


def build_recommendation_service(
    settings: RecommendationSettings,
    *,
    source: CandidateSource,
    serving_redis: Redis,
    clients: RecommendationClients,
) -> RecommendationService:
    cache = RedisServingCache(
        serving_redis,
        ServingKeys(settings.RECS_CACHE_PREFIX, settings.RECS_CACHE_SCHEMA_VERSION),
    )
    featurestore = clients.featurestore
    return RecommendationService(
        source=source,
        cache=cache,
        placements=load_placements(),
        popularity_store=(
            None
            if featurestore is None
            else RedisFeatureStore(featurestore, ITEM_POPULARITY_VIEW)
        ),
        attribute_store=(
            None
            if featurestore is None
            else RedisFeatureStore(featurestore, ITEM_ATTRIBUTES_VIEW)
        ),
        nearline=(
            None
            if clients.nearline is None
            else RedisNearlineSignals(
                clients.nearline,
                prefix=settings.RECS_NEARLINE_PREFIX,
                min_impressions=settings.RECS_NEARLINE_MIN_IMPRESSIONS,
                timeout_s=settings.RECS_NEARLINE_TIMEOUT_MS / 1000,
            )
        ),
        ranker_loader=TrainedRankerLoader(cache),
        candidate_top_k=settings.RECS_CANDIDATE_TOP_K,
        result_top_k=settings.RECS_RESULT_TOP_K,
        model_version=settings.RECS_MODEL_VERSION,
    )
