"""Lifecycle addon for rate_limit: builds the per-principal limiter on Redis."""

from __future__ import annotations

from contextlib import AsyncExitStack

from app.bootstrap.resources import ApplicationResources, opened
from app.core.config import Settings
from app.modules.platform.rate_limit.factory import build_redis_rate_limiter


class RateLimitAddon:
    name = "rate_limit"

    def is_enabled(self, settings: Settings) -> bool:
        return True

    async def open(
        self,
        resources: ApplicationResources,
        settings: Settings,
        stack: AsyncExitStack,
    ) -> None:
        # The limiter holds no connection of its own; Redis is closed by its owner.
        resources.principal_rate_limiter = build_redis_rate_limiter(
            opened(resources.redis, "redis"),
            limit=settings.RATE_LIMIT_PRINCIPAL_PER_MINUTE,
            window_seconds=settings.RATE_LIMIT_WINDOW_SECONDS,
            prefix=f"{settings.RATE_LIMIT_REDIS_PREFIX}:principal",
        )
