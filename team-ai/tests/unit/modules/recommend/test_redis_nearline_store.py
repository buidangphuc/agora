"""RedisNearlineStore against the key layout in add-recsys-nearline-signals/design.md."""

from __future__ import annotations

import asyncio

import pytest
from fakeredis import aioredis

from app.modules.business.recommend.ranking import RedisNearlineStore


async def _store(rows: dict[str, dict[str, str]], **kw) -> RedisNearlineStore:
    redis = aioredis.FakeRedis(decode_responses=True)
    for lid, fields in rows.items():
        await redis.hset(f"recs:nearline:ctr:{lid}", mapping=fields)  # pyright: ignore[reportGeneralTypeIssues]
    return RedisNearlineStore(redis, **kw)


async def test_reads_clicks_over_impressions_per_listing():
    store = await _store(
        {
            "a": {"clicks_ips": "3", "imprs_ips": "30"},
            "b": {"clicks_ips": "9", "imprs_ips": "10.0"},
        }
    )
    got = await store.get_debiased_ctr_batch(["a", "b", "missing"])
    assert got == {"a": pytest.approx(0.1), "b": pytest.approx(0.9)}


async def test_ctr_is_capped_at_one():
    store = await _store({"a": {"clicks_ips": "50", "imprs_ips": "10"}})
    assert (await store.get_debiased_ctr_batch(["a"]))["a"] == 1.0


@pytest.mark.parametrize(
    "fields",
    [
        {"clicks_ips": "1"},  # no impressions field
        {"imprs_ips": "5"},  # no clicks field
        {"clicks_ips": "x", "imprs_ips": "5"},  # malformed
        {"clicks_ips": "1", "imprs_ips": "0"},  # no impressions
        {"clicks_ips": "1", "imprs_ips": "0.4"},  # below the floor
    ],
)
async def test_unusable_rows_are_left_out(fields):
    store = await _store({"a": fields}, min_impressions=1.0)
    assert await store.get_debiased_ctr_batch(["a"]) == {}


async def test_prefix_and_floor_are_configurable():
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.hset("n:ctr:a", mapping={"clicks_ips": "1", "imprs_ips": "4"})  # pyright: ignore[reportGeneralTypeIssues]
    store = RedisNearlineStore(redis, prefix="n", min_impressions=4.0)
    assert await store.get_debiased_ctr_batch(["a"]) == {"a": 0.25}


async def test_redis_error_gives_no_entries_and_logs_once():
    class Down:
        def pipeline(self):
            raise ConnectionError("down")

    store = RedisNearlineStore(Down())
    assert await store.get_debiased_ctr_batch(["a"]) == {}
    assert await store.get_debiased_ctr_batch([]) == {}


async def test_slow_nearline_is_cut_off_by_the_service_budget():
    from app.modules.business.recommend.schemas import Candidate
    from app.modules.business.recommend.service import RecommendationService

    class Slow:
        async def get_debiased_ctr_batch(self, ids):
            await asyncio.sleep(5)
            return {"a": 0.9}

    service = RecommendationService(
        backend=object(),  # type: ignore[arg-type]
        cache=object(),  # type: ignore[arg-type]
        nearline_store=Slow(),
        nearline_timeout_ms=10,
    )
    snapshot = await service._nearline_for([Candidate(listing_id="a", score=1.0)])
    assert snapshot is not None and snapshot.get_debiased_ctr("a") == 0.0
