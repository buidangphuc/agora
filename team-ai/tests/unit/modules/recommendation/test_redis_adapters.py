"""The Redis adapters against the producers' key layouts; every read fails open."""

import asyncio
import json
from typing import Any

import pytest
from fakeredis import aioredis

from app.modules.business.recommendation.adapters.featurestore import (
    RedisFeatureStore,
)
from app.modules.business.recommendation.adapters.nearline import RedisNearlineSignals
from app.modules.business.recommendation.adapters.redis_cache import RedisServingCache
from app.modules.business.recommendation.contract.keys import ServingKeys

KEYS = ServingKeys("recs", "v1")


class Clock:
    def __init__(self) -> None:
        self.now = 1000.0

    def __call__(self) -> float:
        return self.now


class DownRedis:
    """Every command raises, as when Redis is unreachable."""

    def __getattr__(self, name: str) -> Any:
        raise ConnectionError("redis down")


def _fake() -> Any:
    return aioredis.FakeRedis(decode_responses=True)


# --- serving cache -------------------------------------------------------------------


def _cache(redis: Any, clock: Clock | None = None) -> RedisServingCache:
    return RedisServingCache(redis, KEYS, clock=clock or Clock())


async def test_generation_scopes_the_user_and_popular_lists() -> None:
    redis = _fake()
    await redis.set("recs:v1:serving", "g2")
    await redis.set("recs:v1:gen:g2:user:u1", '["a"]')
    await redis.set("recs:v1:gen:g2:popular", '["p"]')
    await redis.set("recs:v1:user:u1", '["legacy"]')
    cache = _cache(redis)
    gen = await cache.serving_generation()
    assert gen == "g2"
    assert [c.listing_id for c in await cache.user_candidates("u1", gen)] == ["a"]
    assert [c.listing_id for c in await cache.popular_candidates(gen)] == ["p"]


async def test_without_a_generation_the_legacy_keys_are_read() -> None:
    redis = _fake()
    await redis.set("recs:v1:user:u1", '["legacy"]')
    await redis.set("recs:v1:popular", '["lp"]')
    cache = _cache(redis)
    assert await cache.serving_generation() is None
    assert [c.listing_id for c in await cache.user_candidates("u1", None)] == ["legacy"]
    assert [c.listing_id for c in await cache.popular_candidates(None)] == ["lp"]


async def test_model_version_is_the_generation_else_the_legacy_key() -> None:
    redis = _fake()
    await redis.set("recs:v1:model_version", "v0")
    cache = _cache(redis)
    assert await cache.model_version("g2") == "g2"
    assert await cache.model_version(None) == "v0"
    await redis.delete("recs:v1:model_version")
    assert await cache.model_version(None) is None


async def test_the_pointer_is_memoised_for_five_seconds() -> None:
    redis = _fake()
    await redis.set("recs:v1:serving", "g1")
    clock = Clock()
    cache = _cache(redis, clock)
    assert await cache.serving_generation() == "g1"
    await redis.set("recs:v1:serving", "g2")
    clock.now += 4.9
    assert await cache.serving_generation() == "g1"
    clock.now += 0.2
    assert await cache.serving_generation() == "g2"


async def test_a_pointer_read_error_counts_as_absent() -> None:
    assert await _cache(DownRedis()).serving_generation() is None


async def test_every_list_read_error_is_a_miss() -> None:
    cache = _cache(DownRedis())
    assert await cache.user_candidates("u1", "g1") == []
    assert await cache.popular_candidates("g1") == []
    assert await cache.model_version("g1") == "g1"
    assert await cache.model_version(None) is None
    assert await cache.ranker_artifact("g1") is None


async def test_the_ranker_artifact_is_the_raw_string() -> None:
    redis = _fake()
    await redis.set("recs:v1:gen:g1:ranker", '{"format":"agora-gbdt/1"}')
    assert await _cache(redis).ranker_artifact("g1") == '{"format":"agora-gbdt/1"}'
    assert await _cache(redis).ranker_artifact("g2") is None


async def test_lists_accept_objects_and_plain_ids_scoring_plain_ids_by_position() -> (
    None
):
    redis = _fake()
    await redis.set(
        "recs:v1:user:u1",
        json.dumps(["a", {"listing_id": "b", "score": 0.5}, {"listing_id": "c"}, ""]),
    )
    got = await _cache(redis).user_candidates("u1", None)
    assert [(c.listing_id, c.score) for c in got] == [
        ("a", 4.0),
        ("b", 0.5),
        ("c", 2.0),
    ]


async def test_a_list_entry_may_flag_out_of_stock() -> None:
    redis = _fake()
    await redis.set(
        "recs:v1:user:u1", json.dumps([{"listing_id": "a", "in_stock": False}])
    )
    [cand] = await _cache(redis).user_candidates("u1", None)
    assert not cand.in_stock


@pytest.mark.parametrize(
    "raw",
    [
        "not json",
        '{"listing_id": "a"}',
        "[]",
        '[{"score": 1}]',
        '[{"listing_id": "a", "score": "x"}]',
    ],
)
async def test_malformed_lists_are_a_miss(raw: str) -> None:
    redis = _fake()
    await redis.set("recs:v1:user:u1", raw)
    assert await _cache(redis).user_candidates("u1", None) == []


async def test_an_empty_user_id_reads_nothing() -> None:
    assert await _cache(DownRedis()).user_candidates("", None) == []


# --- feature store -------------------------------------------------------------------


async def _fs(rows: dict[str, str]) -> Any:
    redis = _fake()
    for key, value in rows.items():
        await redis.set(key, value)
    return redis


async def test_feature_store_reads_the_current_versions_rows() -> None:
    redis = await _fs(
        {
            "fs:item_popularity:current": "3",
            "fs:item_popularity:v3:a": '{"ctr_7d": 0.5}',
            "fs:item_popularity:v2:b": '{"ctr_7d": 0.9}',
        }
    )
    store = RedisFeatureStore(redis, "item_popularity")
    assert await store.item_features(["a", "b"]) == {"a": {"ctr_7d": 0.5}}


async def test_feature_store_view_selects_the_key_family() -> None:
    redis = await _fs(
        {"fs:item_attributes:current": "1", "fs:item_attributes:v1:a": '{"price": 7}'}
    )
    assert await RedisFeatureStore(redis, "item_attributes").item_features(["a"]) == {
        "a": {"price": 7}
    }


async def test_feature_store_skips_malformed_and_non_object_rows() -> None:
    redis = await _fs(
        {
            "fs:item_popularity:current": "1",
            "fs:item_popularity:v1:a": "not json",
            "fs:item_popularity:v1:b": "[1]",
            "fs:item_popularity:v1:c": '{"ctr_7d": 1}',
        }
    )
    store = RedisFeatureStore(redis, "item_popularity")
    assert await store.item_features(["a", "b", "c"]) == {"c": {"ctr_7d": 1}}


async def test_feature_store_memoises_the_current_pointer_for_five_seconds() -> None:
    redis = await _fs(
        {"fs:item_popularity:current": "1", "fs:item_popularity:v1:a": '{"x": 1}'}
    )
    clock = Clock()
    store = RedisFeatureStore(redis, "item_popularity", clock=clock)
    await store.item_features(["a"])
    await redis.set("fs:item_popularity:current", "2")
    await redis.set("fs:item_popularity:v2:a", '{"x": 2}')
    clock.now += 2
    assert await store.item_features(["a"]) == {"a": {"x": 1}}
    clock.now += 4
    assert await store.item_features(["a"]) == {"a": {"x": 2}}


async def test_feature_store_error_degrades_to_empty() -> None:
    assert (
        await RedisFeatureStore(DownRedis(), "item_popularity").item_features(
            ["a", "b"]
        )
        == {}
    )


async def test_feature_store_without_a_current_version_is_empty() -> None:
    assert (
        await RedisFeatureStore(_fake(), "item_popularity").item_features(["a"]) == {}
    )


async def test_feature_store_with_no_ids_reads_nothing() -> None:
    assert (
        await RedisFeatureStore(DownRedis(), "item_popularity").item_features([]) == {}
    )


# --- nearline ------------------------------------------------------------------------


async def _nearline(rows: dict[str, dict[str, str]], **kw: Any) -> RedisNearlineSignals:
    redis = _fake()
    for key, fields in rows.items():
        await redis.hset(key, mapping=fields)
    kw.setdefault("prefix", "recs:nearline")
    kw.setdefault("min_impressions", 1.0)
    kw.setdefault("timeout_s", 1.0)
    return RedisNearlineSignals(redis, **kw)


async def test_nearline_reads_clicks_over_impressions_per_listing() -> None:
    signals = await _nearline(
        {
            "recs:nearline:ctr:a": {"clicks_ips": "3", "imprs_ips": "30"},
            "recs:nearline:ctr:b": {"clicks_ips": "9", "imprs_ips": "10.0"},
        }
    )
    got = await signals.debiased_ctrs(["a", "b", "missing"])
    assert got == {"a": pytest.approx(0.1), "b": pytest.approx(0.9)}


async def test_nearline_ctr_is_capped_at_one() -> None:
    signals = await _nearline(
        {"recs:nearline:ctr:a": {"clicks_ips": "50", "imprs_ips": "10"}}
    )
    assert (await signals.debiased_ctrs(["a"]))["a"] == 1.0


@pytest.mark.parametrize(
    "fields",
    [
        {"clicks_ips": "1"},
        {"imprs_ips": "5"},
        {"clicks_ips": "x", "imprs_ips": "5"},
        {"clicks_ips": "1", "imprs_ips": "0"},
        {"clicks_ips": "1", "imprs_ips": "0.4"},
        {"clicks_ips": "-1", "imprs_ips": "5"},
    ],
)
async def test_nearline_unusable_rows_are_left_out(fields: dict[str, str]) -> None:
    signals = await _nearline({"recs:nearline:ctr:a": fields})
    assert await signals.debiased_ctrs(["a"]) == {}


async def test_nearline_prefix_and_floor_are_configurable() -> None:
    signals = await _nearline(
        {"n:ctr:a": {"clicks_ips": "1", "imprs_ips": "4"}},
        prefix="n",
        min_impressions=4.0,
    )
    assert await signals.debiased_ctrs(["a"]) == {"a": 0.25}


async def test_nearline_error_gives_no_entries() -> None:
    signals = RedisNearlineSignals(
        DownRedis(), prefix="recs:nearline", min_impressions=1.0, timeout_s=1.0
    )
    assert await signals.debiased_ctrs(["a"]) == {}
    assert await signals.debiased_ctrs([]) == {}


async def test_a_slow_nearline_read_is_cut_off_by_its_budget() -> None:
    class SlowPipeline:
        def hmget(self, *args: Any) -> None:
            pass

        async def execute(self) -> list[Any]:
            await asyncio.sleep(5)
            return []

    class SlowRedis:
        def pipeline(self) -> SlowPipeline:
            return SlowPipeline()

    signals = RedisNearlineSignals(
        SlowRedis(), prefix="recs:nearline", min_impressions=1.0, timeout_s=0.01
    )
    assert await signals.debiased_ctrs(["a"]) == {}
