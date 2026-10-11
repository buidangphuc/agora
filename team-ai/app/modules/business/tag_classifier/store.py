"""Durable storage for the tag taxonomy registry (change tag-taxonomy-persistence).

Two hashes, ``<prefix>:v1:canonical`` and ``<prefix>:v1:candidates``, each ``slug -> TagItem JSON``.
A write touches only the tags it changed, in one MULTI, so replicas promoting different tags do not
overwrite each other. Only changed tags are stored: the code seed stays the base layer.
"""

from __future__ import annotations

from collections.abc import Awaitable, Sequence
from typing import TYPE_CHECKING, Protocol, cast

from loguru import logger

from app.modules.business.tag_classifier.schemas import TagItem

if TYPE_CHECKING:
    from redis.asyncio import Redis

FORMAT_VERSION = "v1"


class TaxonomyStore(Protocol):
    async def load(self) -> tuple[list[TagItem], list[TagItem]]:
        """``(canonical, candidates)`` as last saved."""
        ...

    async def save(
        self,
        *,
        canonical: Sequence[TagItem] = (),
        candidates: Sequence[TagItem] = (),
        remove_candidates: Sequence[str] = (),
    ) -> None:
        """Upsert the given tags and delete the given candidate slugs atomically."""
        ...


class RedisTaxonomyStore:
    def __init__(self, redis: Redis, *, prefix: str = "tagtax") -> None:
        self._redis = redis
        self._canonical_key = f"{prefix}:{FORMAT_VERSION}:canonical"
        self._candidates_key = f"{prefix}:{FORMAT_VERSION}:candidates"

    async def load(self) -> tuple[list[TagItem], list[TagItem]]:
        canonical = await self._read(self._canonical_key)
        candidates = await self._read(self._candidates_key)
        return canonical, candidates

    async def _read(self, key: str) -> list[TagItem]:
        raw = await cast("Awaitable[dict[str, str]]", self._redis.hgetall(key))
        tags: list[TagItem] = []
        for slug, payload in raw.items():
            try:
                tags.append(TagItem.model_validate_json(payload))
            except ValueError:
                # One unreadable entry must not hide the rest of the taxonomy.
                logger.error(
                    "taxonomy.store.unreadable_entry key={} slug={}", key, slug
                )
        return tags

    async def save(
        self,
        *,
        canonical: Sequence[TagItem] = (),
        candidates: Sequence[TagItem] = (),
        remove_candidates: Sequence[str] = (),
    ) -> None:
        if not (canonical or candidates or remove_candidates):
            return
        async with self._redis.pipeline(transaction=True) as pipe:
            if canonical:
                pipe.hset(
                    self._canonical_key,
                    mapping={t.slug: t.model_dump_json() for t in canonical},
                )
            if candidates:
                pipe.hset(
                    self._candidates_key,
                    mapping={t.slug: t.model_dump_json() for t in candidates},
                )
            if remove_candidates:
                pipe.hdel(self._candidates_key, *remove_candidates)
            await pipe.execute()
