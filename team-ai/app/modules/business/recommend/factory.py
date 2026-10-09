"""Wiring for the recommend module: build the service and the bootstrap addon.

Mirrors ``RagAddon`` — registered on every boot but only *opened* when
``RECS_ENABLED=true``, at which point it builds the ``RecommendationService``
(wired to the Qdrant backend + the Redis pre-computed cache) and publishes it on
``resources.recommendation_service`` for the gRPC servicer to read. When the flag
is off the addon stays closed and the provider returns ``None``, so the servicer
aborts UNAVAILABLE — identical to the RAG gating seam.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from loguru import logger

from app.modules.business.recommend.backends import build_backend
from app.modules.business.recommend.cache import PrecomputedCache
from app.modules.business.recommend.placement_config import PlacementRegistry
from app.modules.business.recommend.ranking import (
    FeatureStorePort,
    GBDTRankerAdapter,
    InMemoryFeatureStore,
    NearlineSourcePort,
    RedisFeatureStore,
    RedisNearlineStore,
)
from app.modules.business.recommend.service import RecommendationService

if TYPE_CHECKING:
    from fastapi import FastAPI

    from app.bootstrap.resources import ApplicationResources
    from app.core.config import Settings


def _build_feature_store(settings: Settings) -> FeatureStorePort:
    """Online features from the feature store's Redis when configured, else none."""
    url = settings.RECS_FEATURESTORE_REDIS_URL
    if not url:
        return InMemoryFeatureStore()
    from redis.asyncio import Redis

    return RedisFeatureStore(Redis.from_url(url, decode_responses=True))


def _build_nearline_store(settings: Settings) -> NearlineSourcePort | None:
    """Nearline signals from the recsys consumer's Redis when configured, else none."""
    url = settings.RECS_NEARLINE_REDIS_URL
    if not url:
        return None
    from redis.asyncio import Redis

    return RedisNearlineStore(
        Redis.from_url(url, decode_responses=True),
        prefix=settings.RECS_NEARLINE_PREFIX,
        min_impressions=settings.RECS_NEARLINE_MIN_IMPRESSIONS,
    )


async def build_recommendation_service(
    settings: Settings,
    *,
    redis: object | None,
) -> RecommendationService:
    cache = PrecomputedCache(
        redis,  # type: ignore[arg-type]
        prefix=settings.RECS_CACHE_PREFIX,
        schema_version=settings.RECS_CACHE_SCHEMA_VERSION,
    )
    # One pointer decides both stores: the backend names its Qdrant collection from the same
    # serving generation the cache scopes its Redis keys with.
    backend = build_backend(settings, generation_source=cache.serving_generation)
    # Startup collection-contract check (name/dim/metric vs the training job).
    # A mismatch makes Recommend return UNAVAILABLE rather than serve empty.
    collection_ok = await backend.collection_ok()
    if not collection_ok:
        logger.error(
            "recs.collection.mismatch collection={} dim={} distance={} — "
            "Recommend will return UNAVAILABLE",
            settings.RECS_QDRANT_COLLECTION,
            settings.RECS_VECTOR_DIM,
            settings.RECS_QDRANT_DISTANCE,
        )
    registry = PlacementRegistry()
    feature_store: FeatureStorePort = _build_feature_store(settings)
    nearline_store = _build_nearline_store(settings)
    ranker = GBDTRankerAdapter()

    return RecommendationService(
        backend=backend,
        cache=cache,
        registry=registry,
        feature_store=feature_store,
        nearline_store=nearline_store,
        nearline_timeout_ms=settings.RECS_NEARLINE_TIMEOUT_MS,
        ranker=ranker,
        candidate_top_k=settings.RECS_CANDIDATE_TOP_K,
        result_top_k=settings.RECS_RESULT_TOP_K,
        retrieve_timeout_ms=settings.RECS_RETRIEVE_TIMEOUT_MS,
        model_version=settings.RECS_MODEL_VERSION,
        collection_ok=collection_ok,
    )


class RecommendAddon:
    name = "recommend"

    def is_enabled(self, settings: Settings) -> bool:
        return settings.RECS_ENABLED

    async def open(
        self,
        app: FastAPI,
        resources: ApplicationResources,
        settings: Settings,
    ) -> None:
        resources.recommendation_service = await build_recommendation_service(
            settings, redis=resources.redis
        )

    async def close(self, app: FastAPI, resources: ApplicationResources) -> None:
        resources.recommendation_service = None
