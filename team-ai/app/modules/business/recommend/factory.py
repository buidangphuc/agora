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
from app.modules.business.recommend.service import RecommendationService

if TYPE_CHECKING:
    from fastapi import FastAPI

    from app.bootstrap.resources import ApplicationResources
    from app.core.config import Settings


async def build_recommendation_service(
    settings: Settings,
    *,
    redis: object | None,
) -> RecommendationService:
    backend = build_backend(settings)
    cache = PrecomputedCache(
        redis,  # type: ignore[arg-type]
        prefix=settings.RECS_CACHE_PREFIX,
        schema_version=settings.RECS_CACHE_SCHEMA_VERSION,
    )
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
    return RecommendationService(
        backend=backend,
        cache=cache,
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
