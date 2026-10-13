"""Lifecycle addon for tag taxonomy persistence: opens its Redis client and store."""

from __future__ import annotations

from contextlib import AsyncExitStack

from redis.asyncio import Redis

from app.bootstrap.resources import ApplicationResources
from app.core.config import Settings
from app.modules.business.tag_classifier.store import RedisTaxonomyStore


def build_taxonomy_redis_client(settings: Settings) -> Redis:
    """Like ``build_redis_client`` but on the taxonomy database."""
    return Redis(
        host=settings.REDIS_HOST,
        port=settings.REDIS_PORT,
        password=settings.REDIS_PASSWORD or None,
        db=settings.TAXONOMY_REDIS_DATABASE,
        socket_timeout=settings.REDIS_TIMEOUT_SECONDS,
        decode_responses=True,
    )


class TaxonomyAddon:
    name = "taxonomy"

    def is_enabled(self, settings: Settings) -> bool:
        return True

    async def open(
        self,
        resources: ApplicationResources,
        settings: Settings,
        stack: AsyncExitStack,
    ) -> None:
        redis = build_taxonomy_redis_client(settings)
        stack.push_async_callback(redis.aclose)
        resources.taxonomy_store = RedisTaxonomyStore(
            redis, prefix=settings.TAXONOMY_REDIS_PREFIX
        )
