"""Lifecycle addon for quota: opens it and registers its close on the stack."""

from __future__ import annotations

from contextlib import AsyncExitStack

from app.bootstrap.resources import ApplicationResources, opened
from app.core.config import Settings
from app.modules.platform.quota.factory import (
    build_quota_service,
    build_redis_quota_store,
)


class QuotaAddon:
    name = "quota"

    def is_enabled(self, settings: Settings) -> bool:
        return True

    async def open(
        self,
        resources: ApplicationResources,
        settings: Settings,
        stack: AsyncExitStack,
    ) -> None:
        quota = build_quota_service(
            build_redis_quota_store(
                opened(resources.redis, "redis"), prefix=settings.QUOTA_REDIS_PREFIX
            )
        )
        stack.push_async_callback(quota.close)
        resources.quota = quota
