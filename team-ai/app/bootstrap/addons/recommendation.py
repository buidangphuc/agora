"""Lifecycle addon for recommendations: opens the module's clients and owns their close."""

from __future__ import annotations

from contextlib import AsyncExitStack

from qdrant_client import AsyncQdrantClient
from redis.asyncio import Redis

from app.bootstrap.resources import ApplicationResources, opened
from app.core.config import Settings
from app.modules.business.recommendation.factory import RecommendationClients


def _build_qdrant_client(settings: Settings) -> AsyncQdrantClient:
    return AsyncQdrantClient(url=settings.RECS_QDRANT_URL)


def _build_redis_client(url: str, settings: Settings) -> Redis:
    return Redis.from_url(
        url, socket_timeout=settings.REDIS_TIMEOUT_SECONDS, decode_responses=True
    )


class RecommendationAddon:
    name = "recommendation"

    def is_enabled(self, settings: Settings) -> bool:
        return True

    async def open(
        self,
        resources: ApplicationResources,
        settings: Settings,
        stack: AsyncExitStack,
    ) -> None:
        # The serving cache is the shared Redis; this addon owns only its own clients.
        opened(resources.redis, "redis")

        qdrant = _build_qdrant_client(settings)
        stack.push_async_callback(qdrant.close)

        featurestore_url = settings.RECS_FEATURESTORE_REDIS_URL
        nearline_url = settings.RECS_NEARLINE_REDIS_URL
        # One client per distinct URL: equal URLs share it, and it is closed once.
        by_url: dict[str, Redis] = {}
        for url in (featurestore_url, nearline_url):
            if url and url not in by_url:
                by_url[url] = _build_redis_client(url, settings)
                stack.push_async_callback(by_url[url].aclose)

        resources.recommendation_clients = RecommendationClients(
            qdrant=qdrant,
            featurestore=by_url.get(featurestore_url),
            nearline=by_url.get(nearline_url),
        )
