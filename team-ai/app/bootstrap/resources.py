from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import AsyncExitStack, asynccontextmanager
from dataclasses import dataclass, field
from functools import partial
from typing import TYPE_CHECKING, TypeVar

from app.bootstrap.addons.base import BootstrapAddon
from app.core.config import Settings
from app.core.health import HealthChecks, HealthService
from app.core.redis import build_redis_client, check_redis_connection

if TYPE_CHECKING:
    from redis.asyncio import Redis

    from app.core.telemetry_otel import Telemetry
    from app.modules.ai.rag.service import KnowledgeRetrievalService
    from app.modules.business.ai_assistant.factory import AIAssistantServices
    from app.modules.business.chat.ports import ChatService
    from app.modules.business.completions.service import CompletionService
    from app.modules.business.recommendation.factory import RecommendationClients
    from app.modules.business.recommendation.service import RecommendationService
    from app.modules.business.tag_classifier.service import TagClassifierService
    from app.modules.business.tag_classifier.store import RedisTaxonomyStore
    from app.modules.platform.quota.service import QuotaService
    from app.modules.platform.rate_limit.service import RedisRateLimiter


_T = TypeVar("_T")


def opened(value: _T | None, name: str) -> _T:
    """Narrow a resource an earlier opener guarantees is open."""
    if value is None:
        raise RuntimeError(f"{name} is not open")
    return value


@dataclass
class ApplicationResources:
    # Business services: one instance per app, composed in lifespan after the
    # platform resources below are open (see app/bootstrap/services.py).
    completion_service: CompletionService = field(init=False)
    ai_assistant: AIAssistantServices = field(init=False)
    chat: ChatService = field(init=False)
    tag_classifier: TagClassifierService = field(init=False)
    recommendation: RecommendationService = field(init=False)
    # Core infrastructure
    redis: Redis | None = None
    # Addon-provided components
    quota: QuotaService | None = None
    principal_rate_limiter: RedisRateLimiter | None = None
    rag_service: KnowledgeRetrievalService | None = None
    taxonomy_store: RedisTaxonomyStore | None = None
    recommendation_clients: RecommendationClients | None = None
    telemetry: Telemetry | None = None
    # Addons opened for this runtime (closed in reverse by the exit stack)
    addons: list[BootstrapAddon] = field(default_factory=list)
    # Readiness checks, registered by each opener as its resource comes up
    health_checks: HealthChecks = field(default_factory=HealthChecks)


@asynccontextmanager
async def open_application_resources(
    settings: Settings,
    *,
    init_resources: bool,
    addons: tuple[BootstrapAddon, ...] = (),
) -> AsyncIterator[ApplicationResources]:
    """Open every lifecycle resource once; close them in reverse on exit.

    Each opener registers its own close on the exit stack.
    """
    resources = ApplicationResources(
        health_checks=HealthChecks(
            timeout_seconds=settings.HEALTH_CHECK_TIMEOUT_SECONDS
        )
    )
    async with AsyncExitStack() as stack:
        # Redis and the recommendation clients open even without
        # ``init_resources`` (clients connect lazily): the services composed on
        # top of them are mandatory, and readiness only probes them when it is set.
        _open_redis(resources, settings, stack)
        for addon in addons:
            if addon.is_enabled(settings):
                await addon.open(resources, settings, stack)
                resources.addons.append(addon)

        yield resources


def _open_redis(
    resources: ApplicationResources,
    settings: Settings,
    stack: AsyncExitStack,
) -> None:
    redis = build_redis_client(settings)
    stack.push_async_callback(redis.aclose)
    resources.redis = redis
    resources.health_checks.register("redis", partial(check_redis_connection, redis))


def build_health_service(
    resources: ApplicationResources,
    *,
    init_resources: bool,
) -> HealthService:
    return HealthService(
        check_external_dependencies=init_resources,
        checks=resources.health_checks.checks,
        check_timeout_seconds=resources.health_checks.timeout_seconds,
    )
