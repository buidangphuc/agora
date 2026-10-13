from __future__ import annotations

from typing import TYPE_CHECKING

from app.modules.platform.quota.models import QuotaPolicy
from app.modules.platform.quota.service import QuotaService
from app.modules.platform.quota.store import QuotaStore, StaticQuotaPolicyStore

if TYPE_CHECKING:
    from redis.asyncio import Redis


def build_memory_quota_store() -> QuotaStore:
    from app.modules.platform.quota.adapters.memory import MemoryQuotaStore

    return MemoryQuotaStore()


def build_redis_quota_store(redis: Redis, *, prefix: str) -> QuotaStore:
    from app.modules.platform.quota.adapters.redis import RedisQuotaStore

    return RedisQuotaStore(redis, prefix=prefix)


def build_quota_service(
    store: QuotaStore,
    *,
    policies: dict[str, QuotaPolicy] | None = None,
) -> QuotaService:
    return QuotaService(
        store=store,
        policy_store=StaticQuotaPolicyStore(policies),
    )
