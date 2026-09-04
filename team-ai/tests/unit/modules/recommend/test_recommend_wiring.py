"""Cache parsing, backend seam, config, and addon-gating — all proto-free."""

from __future__ import annotations

import pytest

from app.modules.business.recommend.backends import (
    MemoryRetrievalBackend,
    QdrantRetrievalBackend,
    build_backend,
)
from app.modules.business.recommend.cache import PrecomputedCache
from app.modules.business.recommend.factory import RecommendAddon
from tests.factories import build_test_settings


class _FakeRedis:
    def __init__(self, values=None, *, raise_on_get=False):
        self._values = values or {}
        self._raise = raise_on_get

    async def get(self, key):
        if self._raise:
            raise ConnectionError("down")
        return self._values.get(key)


# --- cache parsing -----------------------------------------------------------


async def test_cache_parses_list_of_ids_in_rank_order():
    redis = _FakeRedis({"recs:v1:user:u1": '["a","b","c"]'})
    cache = PrecomputedCache(redis, prefix="recs", schema_version="v1")

    cands = await cache.get_user_candidates("u1")

    assert [c.listing_id for c in cands] == ["a", "b", "c"]
    # Earlier positions get a higher synthetic score so rank order is preserved.
    assert cands[0].score > cands[1].score > cands[2].score


async def test_cache_parses_list_of_objects():
    redis = _FakeRedis(
        {"recs:v1:user:u1": '[{"listing_id":"a","score":0.9,"in_stock":false}]'}
    )
    cache = PrecomputedCache(redis, prefix="recs", schema_version="v1")

    cands = await cache.get_user_candidates("u1")

    assert cands[0].listing_id == "a"
    assert cands[0].score == pytest.approx(0.9)
    assert cands[0].in_stock is False


async def test_cache_malformed_value_is_miss():
    redis = _FakeRedis({"recs:v1:user:u1": "not-json{"})
    cache = PrecomputedCache(redis, prefix="recs", schema_version="v1")
    assert await cache.get_user_candidates("u1") is None


async def test_cache_missing_key_is_miss():
    cache = PrecomputedCache(_FakeRedis({}), prefix="recs", schema_version="v1")
    assert await cache.get_user_candidates("nobody") is None


async def test_cache_redis_error_is_fail_open():
    cache = PrecomputedCache(
        _FakeRedis(raise_on_get=True), prefix="recs", schema_version="v1"
    )
    assert await cache.get_user_candidates("u1") is None


async def test_cache_none_redis_is_miss():
    cache = PrecomputedCache(None, prefix="recs", schema_version="v1")
    assert await cache.get_user_candidates("u1") is None
    assert await cache.get_popular_candidates() is None


async def test_cache_anonymous_user_id_not_read():
    cache = PrecomputedCache(_FakeRedis({}), prefix="recs", schema_version="v1")
    assert await cache.get_user_candidates("") is None


# --- backend seam ------------------------------------------------------------


def test_build_backend_memory():
    settings = build_test_settings(RECS_BACKEND="memory")
    assert isinstance(build_backend(settings), MemoryRetrievalBackend)


def test_build_backend_qdrant():
    settings = build_test_settings(RECS_BACKEND="qdrant")
    assert isinstance(build_backend(settings), QdrantRetrievalBackend)


def test_build_backend_unknown_raises():
    settings = build_test_settings(RECS_BACKEND="bogus")
    with pytest.raises(RuntimeError):
        build_backend(settings)


async def test_memory_backend_is_deterministic_and_excludes_seed():
    backend = MemoryRetrievalBackend()
    first = await backend.retrieve_similar("listing-1", top_k=5)
    second = await backend.retrieve_similar("listing-1", top_k=5)

    assert [c.listing_id for c in first] == [c.listing_id for c in second]
    assert all(c.listing_id != "listing-1" for c in first)
    assert await backend.collection_ok() is True


async def test_memory_backend_no_seed_returns_empty():
    assert await MemoryRetrievalBackend().retrieve_similar("", top_k=5) == []


# --- config + gating ---------------------------------------------------------


def test_settings_defaults_are_conservative():
    settings = build_test_settings()
    assert settings.RECS_ENABLED is False
    assert settings.RECS_BACKEND == "memory"
    assert settings.RECS_QDRANT_COLLECTION == "item_als_vectors"
    assert settings.RECS_CANDIDATE_TOP_K == 100
    assert settings.RECS_RESULT_TOP_K == 10
    assert settings.RECS_CACHE_PREFIX == "recs"


def test_addon_is_gated_by_flag():
    addon = RecommendAddon()
    assert addon.is_enabled(build_test_settings(RECS_ENABLED=False)) is False
    assert addon.is_enabled(build_test_settings(RECS_ENABLED=True)) is True


async def test_addon_open_builds_service_with_memory_backend():
    from types import SimpleNamespace

    from app.modules.business.recommend.service import RecommendationService

    settings = build_test_settings(RECS_ENABLED=True, RECS_BACKEND="memory")
    resources = SimpleNamespace(redis=None, recommendation_service=None)

    await RecommendAddon().open(app=None, resources=resources, settings=settings)

    assert isinstance(resources.recommendation_service, RecommendationService)
