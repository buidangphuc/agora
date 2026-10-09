"""recs-serving-safeguards: degrade instead of fail, cold start, online features."""

from __future__ import annotations

import json
from typing import Any

import pytest

from app.modules.business.recommend.backends import QdrantRetrievalBackend
from app.modules.business.recommend.cache import PrecomputedCache
from app.modules.business.recommend.factory import build_recommendation_service
from app.modules.business.recommend.ranking import (
    InMemoryFeatureStore,
    RedisFeatureStore,
)
from app.modules.business.recommend.schemas import Candidate, RecommendQuery
from app.modules.business.recommend.service import RecommendationService
from tests.factories import build_test_settings


class _Backend:
    def __init__(self, *, popular=None, raise_on_popular=False) -> None:
        self._popular = popular or []
        self._raise = raise_on_popular

    async def retrieve_similar(self, seed_listing_id, *, top_k):
        raise RuntimeError("qdrant down")

    async def popular(self, *, top_k):
        if self._raise:
            raise RuntimeError("qdrant down")
        return list(self._popular[:top_k])

    async def collection_ok(self):
        return True


class _BrokenUserCache:
    """A cache whose user read raises (not the fail-open Redis path)."""

    def __init__(self, popular=None, *, popular_raises=False) -> None:
        self._popular = popular
        self._popular_raises = popular_raises

    async def get_user_candidates(self, user_id):
        raise ConnectionError("redis down")

    async def get_popular_candidates(self):
        if self._popular_raises:
            raise ConnectionError("redis down")
        return self._popular

    async def get_model_version(self):
        raise ConnectionError("redis down")


def _svc(backend, cache, **kw) -> RecommendationService:
    return RecommendationService(
        backend=backend, cache=cache, model_version="als-1", **kw
    )


async def test_cache_error_serves_the_popular_list_with_fallback_version():
    cache = _BrokenUserCache(popular=[Candidate("p1", 2.0), Candidate("p2", 1.0)])
    result = await _svc(_Backend(), cache).recommend(RecommendQuery(user_id="u1"))

    assert [i.listing_id for i in result.items] == ["p1", "p2"]
    assert result.model_version == "serving-fallback"
    assert result.fallback is True


async def test_cache_popular_down_uses_the_backend_popular_list():
    cache = _BrokenUserCache(popular_raises=True)
    backend = _Backend(popular=[Candidate("b1", 1.0)])
    # popular read raises in the cache too, so the fallback ends at the backend.
    result = await _svc(backend, cache)._fallback(RecommendQuery(user_id="u1"))
    assert [i.listing_id for i in result.items] == ["b1"]
    assert result.fallback is True


async def test_everything_down_is_an_empty_ok_result():
    cache = _BrokenUserCache(popular_raises=True)
    backend = _Backend(raise_on_popular=True)
    result = await _svc(backend, cache).recommend(RecommendQuery(user_id="u1"))

    assert result.items == []
    assert result.model_version == "serving-fallback"
    assert result.fallback is True


class _FakeRedis:
    def __init__(self, values=None, *, fail=False) -> None:
        self.values = values or {}
        self.fail = fail
        self.gets: list[str] = []

    async def get(self, key):
        self.gets.append(key)
        if self.fail:
            raise ConnectionError("down")
        return self.values.get(key)

    async def mget(self, keys):
        if self.fail:
            raise ConnectionError("down")
        return [self.values.get(k) for k in keys]


async def test_cold_start_serves_the_generation_popular_list_in_order():
    redis = _FakeRedis(
        {
            "recs:v1:serving": "g2",
            "recs:v1:gen:g2:popular": '["hot-1","hot-2","hot-3"]',
        }
    )
    backend = _Backend(popular=[Candidate("scroll-x", 9.0)])
    cache = PrecomputedCache(redis, prefix="recs", schema_version="v1")
    result = await _svc(backend, cache).recommend(RecommendQuery(user_id="new-user"))

    assert [i.listing_id for i in result.items] == ["hot-1", "hot-2", "hot-3"]
    assert result.fallback is False


async def test_cold_start_without_pointer_uses_the_unscoped_popular_list():
    redis = _FakeRedis({"recs:v1:popular": '["u-1","u-2"]'})
    cache = PrecomputedCache(redis, prefix="recs", schema_version="v1")
    result = await _svc(_Backend(), cache).recommend(RecommendQuery(user_id="x"))
    assert [i.listing_id for i in result.items] == ["u-1", "u-2"]


async def test_qdrant_backend_no_longer_scrolls_for_popular():
    backend = QdrantRetrievalBackend(
        url="http://unused", collection="c", vector_dim=4, distance="Cosine"
    )
    # No client is ever built: popular is the cache's job now.
    assert await backend.popular(top_k=10) == []
    assert backend._client is None


# --- online features ---------------------------------------------------------


def _fs_redis(version="3", rows=None, fail=False) -> _FakeRedis:
    values: dict[str, Any] = {}
    if version:
        values["fs:item_popularity:current"] = version
    for lid, feats in (rows or {}).items():
        values[f"fs:item_popularity:v{version}:{lid}"] = json.dumps(feats)
    return _FakeRedis(values, fail=fail)


async def test_redis_feature_store_reads_the_current_version_rows():
    store = RedisFeatureStore(_fs_redis(rows={"a": {"ctr_7d": 0.5}}))
    got = await store.get_item_features_batch(["a", "b"])
    assert got == {"a": {"ctr_7d": 0.5}}  # missing key -> absent


async def test_redis_feature_store_memoises_the_current_pointer():
    redis = _fs_redis(rows={"a": {"ctr_7d": 0.5}})
    now = [0.0]
    store = RedisFeatureStore(redis, clock=lambda: now[0])
    await store.get_item_features_batch(["a"])
    now[0] = 2.0
    await store.get_item_features_batch(["a"])
    assert redis.gets.count("fs:item_popularity:current") == 1
    now[0] = 6.0
    await store.get_item_features_batch(["a"])
    assert redis.gets.count("fs:item_popularity:current") == 2


async def test_redis_feature_store_error_degrades_to_empty():
    store = RedisFeatureStore(_fs_redis(fail=True))
    assert await store.get_item_features_batch(["a", "b"]) == {}


async def test_redis_feature_store_without_a_current_version_is_empty():
    assert (
        await RedisFeatureStore(_fs_redis(version=None)).get_item_features_batch(["a"])
        == {}
    )


_TIED = [Candidate("first", 0.5), Candidate("second", 0.5)]


async def _rank(feature_store, cache=None):
    cache = cache or _BrokenUserCache(popular=list(_TIED))
    svc = _svc(_Backend(), cache, feature_store=feature_store)
    return await svc.recommend(RecommendQuery(anonymous_id="a"))


async def test_online_features_break_a_tie_by_ctr():
    store = RedisFeatureStore(_fs_redis(rows={"second": {"ctr_7d": 0.5}}))
    result = await _rank(store)
    assert [i.listing_id for i in result.items] == ["second", "first"]


async def test_favorites_break_a_ctr_tie():
    store = RedisFeatureStore(
        _fs_redis(
            rows={
                "first": {"ctr_7d": 0.2, "favorites_current": 1},
                "second": {"ctr_7d": 0.2, "favorites_current": 9},
            }
        )
    )
    result = await _rank(store)
    assert [i.listing_id for i in result.items] == ["second", "first"]


async def test_boost_is_bounded_so_the_model_score_stays_dominant():
    cands = [Candidate("strong", 0.9), Candidate("weak", 0.3)]
    store = RedisFeatureStore(_fs_redis(rows={"weak": {"ctr_7d": 1e9}}))
    result = await _rank(store, _BrokenUserCache(popular=cands))
    assert [i.listing_id for i in result.items] == ["strong", "weak"]


async def test_missing_features_leave_ranking_unchanged():
    store = RedisFeatureStore(_fs_redis(rows={}))
    result = await _rank(store)
    assert [i.listing_id for i in result.items] == ["first", "second"]


async def test_feature_store_error_leaves_ranking_unchanged():
    result = await _rank(RedisFeatureStore(_fs_redis(fail=True)))
    assert [i.listing_id for i in result.items] == ["first", "second"]


async def test_factory_wires_the_redis_feature_store_only_when_configured():
    off = await build_recommendation_service(
        build_test_settings(RECS_ENABLED=True), redis=None
    )
    assert isinstance(off._feature_store, InMemoryFeatureStore)

    on = await build_recommendation_service(
        build_test_settings(
            RECS_ENABLED=True, RECS_FEATURESTORE_REDIS_URL="redis://localhost:6379/2"
        ),
        redis=None,
    )
    assert isinstance(on._feature_store, RedisFeatureStore)


@pytest.mark.parametrize("limit", [1, 2])
async def test_limit_is_applied_after_the_feature_boost(limit):
    store = RedisFeatureStore(_fs_redis(rows={"second": {"ctr_7d": 0.5}}))
    svc = _svc(_Backend(), _BrokenUserCache(popular=list(_TIED)), feature_store=store)
    result = await svc.recommend(RecommendQuery(anonymous_id="a", limit=limit))
    assert len(result.items) == limit
    assert result.items[0].listing_id == "second"
