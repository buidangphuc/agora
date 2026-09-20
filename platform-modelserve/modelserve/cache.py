"""Redis embedding cache layer."""

from __future__ import annotations

import json
import logging
from typing import Any

import redis.asyncio as aioredis

from modelserve.config import Settings
from modelserve.model_version import make_embedding_cache_key

logger = logging.getLogger(__name__)


class EmbeddingCache:
    """Async Redis cache for text embeddings."""

    def __init__(self, settings: Settings, redis_client: Any | None = None) -> None:
        self._settings = settings
        self._client: Any = redis_client
        self._enabled = settings.embed_cache_enabled

    async def connect(self) -> None:
        if not self._enabled:
            return
        if self._client is None:
            try:
                self._client = aioredis.from_url(
                    self._settings.redis_url,
                    decode_responses=True,
                )
            except Exception as exc:
                logger.warning("Failed to initialize Redis client: %s", exc)
                self._client = None

    async def close(self) -> None:
        if self._client is not None:
            try:
                await self._client.aclose()
            except Exception as exc:
                logger.warning("Error closing Redis client: %s", exc)
            self._client = None

    async def get_many(
        self,
        texts: list[str],
        *,
        model_version: str | None = None,
    ) -> list[list[float] | None]:
        """Fetch cached vectors for a list of texts. Returns None for cache misses."""
        if not self._enabled or self._client is None or not texts:
            return [None] * len(texts)

        version = model_version or self._settings.model_version
        keys = [make_embedding_cache_key(version, t) for t in texts]

        try:
            raw_results = await self._client.mget(keys)
            results: list[list[float] | None] = []
            for item in raw_results:
                if item is not None:
                    try:
                        results.append(json.loads(item))
                    except Exception:
                        results.append(None)
                else:
                    results.append(None)
            return results
        except Exception as exc:
            logger.warning("Redis mget error: %s", exc)
            return [None] * len(texts)

    async def set_many(
        self,
        texts: list[str],
        vectors: list[list[float]],
        *,
        model_version: str | None = None,
    ) -> None:
        """Store newly calculated vectors into Redis."""
        if not self._enabled or self._client is None or not texts:
            return

        version = model_version or self._settings.model_version
        ttl = self._settings.embed_cache_ttl_seconds

        try:
            pipeline = self._client.pipeline()
            for text, vector in zip(texts, vectors, strict=False):
                key = make_embedding_cache_key(version, text)
                pipeline.set(key, json.dumps(vector), ex=ttl)
            await pipeline.execute()
        except Exception as exc:
            logger.warning("Redis set_many error: %s", exc)
