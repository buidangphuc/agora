"""Tests for embedding cache layer."""

from __future__ import annotations

import asyncio

from modelserve.cache import EmbeddingCache
from modelserve.config import Settings
from modelserve.model_version import make_embedding_cache_key


class FakeRedis:
    def __init__(self) -> None:
        self.store: dict[str, str] = {}

    async def mget(self, keys: list[str]) -> list[str | None]:
        return [self.store.get(k) for k in keys]

    def pipeline(self) -> FakePipeline:
        return FakePipeline(self)

    async def aclose(self) -> None:
        pass


class FakePipeline:
    def __init__(self, redis: FakeRedis) -> None:
        self.redis = redis
        self.ops: list[tuple[str, str, int | None]] = []

    def set(self, key: str, value: str, ex: int | None = None) -> FakePipeline:
        self.ops.append((key, value, ex))
        return self

    async def execute(self) -> list[bool]:
        for key, val, _ in self.ops:
            self.redis.store[key] = val
        return [True] * len(self.ops)


def test_embedding_cache_key_generation() -> None:
    key1 = make_embedding_cache_key("v1", "hello")
    key2 = make_embedding_cache_key("v1", "hello")
    key3 = make_embedding_cache_key("v2", "hello")
    key4 = make_embedding_cache_key("v1", "world")

    assert key1 == key2
    assert key1 != key3  # model_version isolates cache
    assert key1 != key4  # different text


def test_embedding_cache_get_set() -> None:
    async def _run():
        fake_redis = FakeRedis()
        settings = Settings(embed_cache_enabled=True, model_version="v1")
        cache = EmbeddingCache(settings, redis_client=fake_redis)

        texts = ["apple", "banana"]
        vectors = [[0.1, 0.2, 0.3], [0.4, 0.5, 0.6]]

        # Initial get -> all misses
        results = await cache.get_many(texts)
        assert results == [None, None]

        # Set vectors
        await cache.set_many(texts, vectors)

        # Subsequent get -> all hits
        cached_results = await cache.get_many(texts)
        assert cached_results == vectors

        # Get with mixed texts
        mixed_texts = ["apple", "cherry"]
        mixed_results = await cache.get_many(mixed_texts)
        assert mixed_results == [[0.1, 0.2, 0.3], None]

    asyncio.run(_run())
