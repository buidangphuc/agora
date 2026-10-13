"""Durable storage for the tag taxonomy overlay.

Two hashes, ``<prefix>:v1:canonical`` and ``<prefix>:v1:candidates``, each ``slug -> TagItem JSON``.
A write touches only the tags it changed, in one MULTI, so replicas promoting different tags do not
overwrite each other. Only changed tags are stored: the code seed stays the base layer.
"""

from collections.abc import Awaitable, Sequence
from typing import Protocol, cast

from loguru import logger
from redis.asyncio import Redis

from app.modules.business.tag_classifier.schemas import TagItem

FORMAT_VERSION = "v1"


class TaxonomyStore(Protocol):
    async def load(self) -> tuple[dict[str, TagItem], dict[str, TagItem]]:
        """``(canonical overlay, candidates)`` as last saved, each keyed by slug."""
        ...

    async def save(
        self,
        *,
        canonical: Sequence[TagItem] = (),
        candidates: Sequence[TagItem] = (),
        drop_candidates: Sequence[str] = (),
    ) -> None:
        """Upsert the given tags and delete the given candidate slugs atomically."""
        ...


class RedisTaxonomyStore:
    def __init__(self, redis: Redis, *, prefix: str) -> None:
        self._redis = redis
        self._canonical_key = f"{prefix}:{FORMAT_VERSION}:canonical"
        self._candidates_key = f"{prefix}:{FORMAT_VERSION}:candidates"

    async def load(self) -> tuple[dict[str, TagItem], dict[str, TagItem]]:
        canonical = await self._read(self._canonical_key)
        candidates = await self._read(self._candidates_key)
        return canonical, candidates

    async def _read(self, key: str) -> dict[str, TagItem]:
        raw = await cast("Awaitable[dict[str, str]]", self._redis.hgetall(key))
        tags: dict[str, TagItem] = {}
        for slug, payload in raw.items():
            try:
                tags[slug] = TagItem.model_validate_json(payload)
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
        drop_candidates: Sequence[str] = (),
    ) -> None:
        if not (canonical or candidates or drop_candidates):
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
            if drop_candidates:
                pipe.hdel(self._candidates_key, *drop_candidates)
            await pipe.execute()
