"""Bootstrap addon: persist the tag taxonomy registry in Redis (change tag-taxonomy-persistence).

Registered on every boot, opened only when ``TAXONOMY_PERSISTENCE_ENABLED=true`` (which requires
``REDIS_ENABLED``). It attaches the store to the process-wide classifier the REST routes and the
gRPC servicer both use, so both see the loaded and promoted state. A store that cannot be read at
startup does not stop the service: it serves the seed taxonomy and retries before the next mutation.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from loguru import logger

from app.core.redis import build_redis_client
from app.modules.business.tag_classifier.service import shared_tag_classifier
from app.modules.business.tag_classifier.store import RedisTaxonomyStore

if TYPE_CHECKING:
    from fastapi import FastAPI
    from redis.asyncio import Redis

    from app.bootstrap.resources import ApplicationResources
    from app.core.config import Settings


class TagTaxonomyAddon:
    name = "tag_taxonomy"

    def __init__(self) -> None:
        self._redis: Redis | None = None

    def is_enabled(self, settings: Settings) -> bool:
        return settings.TAXONOMY_PERSISTENCE_ENABLED

    async def open(
        self,
        app: FastAPI,
        resources: ApplicationResources,
        settings: Settings,
    ) -> None:
        own_db = settings.model_copy(
            update={"REDIS_DATABASE": settings.TAXONOMY_REDIS_DATABASE}
        )
        self._redis = build_redis_client(own_db)
        store = RedisTaxonomyStore(self._redis, prefix=settings.TAXONOMY_REDIS_PREFIX)
        service = shared_tag_classifier()
        loaded = await service.attach_store(store)
        resources.tag_classifier_service = service
        logger.info(
            "taxonomy.persistence_enabled db={} prefix={} loaded={}",
            settings.TAXONOMY_REDIS_DATABASE,
            settings.TAXONOMY_REDIS_PREFIX,
            loaded,
        )

    async def close(self, app: FastAPI, resources: ApplicationResources) -> None:
        shared_tag_classifier().detach_store()
        redis, self._redis = self._redis, None
        if redis is not None:
            await redis.aclose()
