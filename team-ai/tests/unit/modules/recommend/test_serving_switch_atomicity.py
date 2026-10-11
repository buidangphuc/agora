"""serving-switch-atomicity: one serving-pointer read decides the Redis lists and the Qdrant collection."""

from __future__ import annotations

from types import SimpleNamespace

import pytest

from app.modules.business.recommend.backends import (
    QdrantRetrievalBackend,
    generation_collection,
    point_id,
)
from app.modules.business.recommend.cache import PrecomputedCache
from app.modules.business.recommend.factory import build_recommendation_service
from app.modules.business.recommend.schemas import RecommendQuery
from tests.factories import build_test_settings

pytest.importorskip("qdrant_client")


class _Redis:
    def __init__(self, values=None):
        self.values = values or {}

    async def get(self, key):
        return self.values.get(key)


class _Clock:
    def __init__(self):
        self.now = 0.0

    def __call__(self):
        return self.now


class _Qdrant:
    def __init__(self, fail_collections=()):
        self.collections: list[str] = []
        self.fail = set(fail_collections)

    def query_points(self, **kwargs):
        name = kwargs["collection_name"]
        self.collections.append(name)
        if name in self.fail:
            raise RuntimeError("collection not found")
        hit = SimpleNamespace(
            id=point_id("n"), score=0.5, payload={"listing_id": f"from-{name}"}
        )
        return SimpleNamespace(points=[hit])


def _backend(client, source) -> QdrantRetrievalBackend:
    b = QdrantRetrievalBackend(
        url="http://q:6333",
        collection="item_als_vectors",
        vector_dim=4,
        distance="Cosine",
        generation_source=source,
    )
    b._client = client
    return b


async def test_collection_name_comes_from_the_serving_pointer() -> None:
    redis = _Redis({"recs:v1:serving": "g2"})
    cache = PrecomputedCache(redis, prefix="recs", schema_version="v1")
    client = _Qdrant()
    got = await _backend(client, cache.serving_generation).retrieve_similar(
        "a", top_k=3
    )
    assert client.collections == ["item_als_vectors__g2"]
    assert [c.listing_id for c in got] == ["from-item_als_vectors__g2"]


async def test_no_pointer_falls_back_to_the_alias_name() -> None:
    cache = PrecomputedCache(_Redis(), prefix="recs", schema_version="v1")
    client = _Qdrant()
    await _backend(client, cache.serving_generation).retrieve_similar("a", top_k=3)
    assert client.collections == ["item_als_vectors"]
    assert generation_collection("item_als_vectors", None) == "item_als_vectors"


async def test_a_pointer_to_a_missing_collection_does_not_fall_back_to_the_alias() -> (
    None
):
    redis = _Redis({"recs:v1:serving": "g2"})
    cache = PrecomputedCache(redis, prefix="recs", schema_version="v1")
    client = _Qdrant(fail_collections={"item_als_vectors__g2"})
    with pytest.raises(RuntimeError):
        await _backend(client, cache.serving_generation).retrieve_similar("a", top_k=3)
    assert client.collections == ["item_als_vectors__g2"]  # the alias was never tried


async def test_the_pinned_generation_survives_a_pointer_move_mid_request() -> None:
    redis = _Redis({"recs:v1:serving": "g1"})
    clock = _Clock()
    cache = PrecomputedCache(redis, prefix="recs", schema_version="v1", clock=clock)
    client = _Qdrant()
    backend = _backend(client, cache.serving_generation)

    token = await cache.pin_generation()
    try:
        redis.values["recs:v1:serving"] = "g2"  # publish switches ...
        clock.now = 60.0  # ... and the 5 s memo has expired
        await backend.retrieve_similar("a", top_k=3)
        assert await cache.serving_generation() == "g1"
    finally:
        cache.unpin_generation(token)

    await backend.retrieve_similar("a", top_k=3)
    assert client.collections == ["item_als_vectors__g1", "item_als_vectors__g2"]


async def test_service_pins_one_generation_for_lists_and_vectors() -> None:
    settings = build_test_settings(RECS_ENABLED=True, RECS_BACKEND="qdrant")
    redis = _Redis({"recs:v1:serving": "g1"})
    svc = await build_recommendation_service(settings, redis=redis)
    client = _Qdrant()
    svc._backend._client = client
    svc._collection_ok = True

    real_get = redis.get
    calls = {"n": 0}

    async def moving_get(key):
        # The pointer moves after the first read of the request: a request that reads it
        # again would see g2 (lists from g1, vectors from g2 - the mix this change removes).
        if key == "recs:v1:serving":
            calls["n"] += 1
            if calls["n"] > 1:
                return "g2"
        return await real_get(key)

    redis.get = moving_get
    svc._cache._pointer_ttl_s = 0.0  # no memo: every unpinned read hits Redis
    await svc.recommend(RecommendQuery(seed_listing_id="a"))
    assert client.collections
    assert set(client.collections) == {"item_als_vectors__g1"}


async def test_factory_wires_the_cache_pointer_into_the_backend() -> None:
    settings = build_test_settings(RECS_ENABLED=True, RECS_BACKEND="qdrant")
    svc = await build_recommendation_service(
        settings, redis=_Redis({"recs:v1:serving": "g7"})
    )
    assert await svc._backend.current_collection() == "item_als_vectors__g7"
