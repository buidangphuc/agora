"""The ladder, the fallback path and readiness, over ports faked in memory."""

import asyncio
import json
from pathlib import Path
from typing import Any

import pytest
from fakeredis import aioredis

from app.core.errors import ServiceUnavailableError
from app.modules.business.recommendation.adapters.memory import (
    MemoryCandidateSource,
    MemoryFeatureStore,
    MemoryNearlineSignals,
)
from app.modules.business.recommendation.adapters.redis_cache import RedisServingCache
from app.modules.business.recommendation.contract.keys import ServingKeys
from app.modules.business.recommendation.placements import load_placements
from app.modules.business.recommendation.ranking.artifact import TrainedRankerLoader
from app.modules.business.recommendation.schemas import (
    Candidate,
    RecommendationQuery,
)
from app.modules.business.recommendation.service import RecommendationService


class Clock:
    def __init__(self) -> None:
        self.now = 1000.0

    def __call__(self) -> float:
        return self.now


class Source(MemoryCandidateSource):
    def __init__(self, similar: dict[str, list[Candidate]] | None = None) -> None:
        super().__init__(similar or {})
        self.ok = True
        self.checks = 0
        self.calls: list[tuple[str, str | None]] = []

    async def ready(self, generation: str | None) -> bool:
        self.checks += 1
        return self.ok

    async def similar(
        self, seed_listing_id: str, *, top_k: int, generation: str | None
    ) -> list[Candidate]:
        self.calls.append((seed_listing_id, generation))
        return await super().similar(
            seed_listing_id, top_k=top_k, generation=generation
        )


def _service(
    source: Any = None,
    redis: Any = None,
    *,
    clock: Clock | None = None,
    cache: Any = None,
    **kw: Any,
) -> RecommendationService:
    clock = clock or Clock()
    redis = aioredis.FakeRedis(decode_responses=True) if redis is None else redis
    if cache is None:
        cache = RedisServingCache(redis, ServingKeys("recs", "v1"), clock=clock)
    return RecommendationService(
        source=Source() if source is None else source,
        cache=cache,
        placements=kw.pop("placements", None) or load_placements(),
        popularity_store=kw.pop("popularity_store", None),
        attribute_store=kw.pop("attribute_store", None),
        nearline=kw.pop("nearline", None),
        ranker_loader=TrainedRankerLoader(cache, clock=clock),
        candidate_top_k=100,
        result_top_k=10,
        model_version=kw.pop("model_version", "test-v1"),
        clock=clock,
        **kw,
    )


def _ids(result: Any) -> list[str]:
    return [i.listing_id for i in result.items]


async def test_a_cache_hit_skips_retrieval() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:user:u1", '["l1","l2","l3"]')
    source = Source({"seed": [Candidate("x", 1.0)]})
    result = await _service(source, redis).recommend(
        RecommendationQuery(user_id="u1", seed_listing_id="seed")
    )
    assert _ids(result) == ["l1", "l2", "l3"]
    assert [i.rank for i in result.items] == [1, 2, 3]
    assert result.model_version == "test-v1"
    assert source.calls == []


async def test_a_cache_miss_falls_to_vector_similarity() -> None:
    source = Source({"seed": [Candidate("l9", 0.9), Candidate("l8", 0.8)]})
    result = await _service(source).recommend(
        RecommendationQuery(user_id="u1", seed_listing_id="seed")
    )
    assert _ids(result) == ["l9", "l8"]
    assert [c[0] for c in source.calls] == ["seed"]


async def test_an_anonymous_visitor_skips_the_user_list() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:user:", '["never"]')
    source = Source({"seed": [Candidate("l1", 0.9)]})
    result = await _service(source, redis).recommend(
        RecommendationQuery(anonymous_id="anon", seed_listing_id="seed")
    )
    assert _ids(result) == ["l1"]
    assert result.placement_id == "similar_items"


async def test_the_request_uses_one_generation_for_lists_and_collection() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:serving", "g7")
    await redis.set("recs:v1:gen:g7:user:u1", "[]")
    source = Source({"seed": [Candidate("l1", 0.9)]})
    await _service(source, redis).recommend(
        RecommendationQuery(user_id="u1", seed_listing_id="seed")
    )
    assert source.calls == [("seed", "g7")]


class _MovingPointerCache(RedisServingCache):
    """The serving pointer moves between reads; one request must see one generation."""

    def __init__(self, redis: Any) -> None:
        super().__init__(redis, ServingKeys("recs", "v1"))
        self.reads = 0

    async def serving_generation(self) -> str | None:
        self.reads += 1
        return f"g{self.reads}"


async def test_the_generation_is_resolved_once_per_request() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    cache = _MovingPointerCache(redis)
    source = Source({"seed": [Candidate("l1", 0.9)]})
    service = _service(source, redis, cache=cache)
    await service.check_ready()
    cache.reads = 0
    await service.recommend(RecommendationQuery(user_id="u1", seed_listing_id="seed"))
    assert cache.reads == 1
    assert [gen for _, gen in source.calls] == ["g1"]


async def test_retrieval_that_finds_nothing_falls_to_the_popular_list() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:popular", '["hot-1","hot-2"]')
    result = await _service(Source({"seed": []}), redis).recommend(
        RecommendationQuery(anonymous_id="anon", seed_listing_id="seed")
    )
    assert _ids(result) == ["hot-1", "hot-2"]
    assert not result.fallback


async def test_limit_overrides_the_placement_default() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:popular", json.dumps([f"p{i}" for i in range(20)]))
    service = _service(redis=redis)
    assert len((await service.recommend(RecommendationQuery(limit=3))).items) == 3
    assert (
        len((await service.recommend(RecommendationQuery())).items) == 10
    )  # home_feed
    cart = RecommendationQuery(placement_id="cart_cross_sell")
    assert len((await service.recommend(cart)).items) == 8


async def test_a_placement_without_a_limit_uses_result_top_k(tmp_path: Path) -> None:
    path = tmp_path / "placements.yaml"
    path.write_text(
        "placements:\n  home_feed:\n    candidate_ladder:\n"
        "      - strategy: global_popular\n",
        encoding="utf-8",
    )
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:popular", json.dumps([f"p{i}" for i in range(20)]))
    service = _service(redis=redis, placements=load_placements(path))
    assert len((await service.recommend(RecommendationQuery())).items) == 10


class _BrokenUserCache(RedisServingCache):
    async def user_candidates(
        self, user_id: str, generation: str | None
    ) -> list[Candidate]:
        raise RuntimeError("bug in the user read")


async def test_an_internal_error_serves_the_popular_list_as_the_fallback() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:popular", '["p1","p2"]')
    await redis.set("recs:v1:model_version", "v0")
    cache = _BrokenUserCache(redis, ServingKeys("recs", "v1"))
    result = await _service(redis=redis, cache=cache).recommend(
        RecommendationQuery(user_id="u1")
    )
    assert _ids(result) == ["p1", "p2"]
    assert result.model_version == "serving-fallback"
    assert result.fallback


async def test_everything_missing_is_an_empty_ok_result() -> None:
    result = await _service().recommend(RecommendationQuery(user_id="u1"))
    assert result.items == []
    assert result.fallback
    assert result.model_version == "test-v1"  # the ladder floor keeps the model version


async def test_an_unreachable_redis_is_an_empty_ok_result() -> None:
    class Down:
        def __getattr__(self, name: str) -> Any:
            raise ConnectionError("down")

    redis = Down()
    cache = RedisServingCache(redis, ServingKeys("recs", "v1"))
    result = await _service(redis=redis, cache=cache).recommend(
        RecommendationQuery(user_id="u1")
    )
    assert result.items == []
    assert result.fallback


async def test_the_serving_fallback_model_version_follows_the_generation() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:serving", "g4")
    await redis.set("recs:v1:gen:g4:user:u1", '["a"]')
    result = await _service(redis=redis, model_version="serving-fallback").recommend(
        RecommendationQuery(user_id="u1")
    )
    assert result.model_version == "g4"


async def test_online_features_reorder_the_fixed_weight_ranking() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set(
        "recs:v1:user:u1",
        json.dumps(
            [
                {"listing_id": "plain", "score": 0.95},
                {"listing_id": "hot", "score": 0.80},
            ]
        ),
    )
    store = MemoryFeatureStore(
        {
            "hot": {
                "ctr_7d": 0.15,
                "views_7d": 900,
                "clicks_7d": 135,
                "add_to_cart_7d": 40,
            }
        }
    )
    service = _service(redis=redis, popularity_store=store)
    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )
    assert _ids(result) == ["hot", "plain"]


async def test_nearline_ctr_reorders_equal_candidates() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set(
        "recs:v1:user:u1",
        '[{"listing_id":"a","score":0.8},{"listing_id":"b","score":0.8}]',
    )
    service = _service(
        redis=redis, nearline=MemoryNearlineSignals({"b": 0.12, "a": 0.04})
    )
    result = await service.recommend(RecommendationQuery(user_id="u1"))
    assert _ids(result) == ["b", "a"]


# --- readiness ----------------------------------------------------------------------


async def test_recommend_is_unavailable_while_the_collection_check_fails() -> None:
    source = Source()
    source.ok = False
    service = _service(source)
    with pytest.raises(ServiceUnavailableError):
        await service.recommend(RecommendationQuery(user_id="u1"))


async def test_a_failed_check_is_retried_at_most_every_thirty_seconds() -> None:
    source = Source()
    source.ok = False
    clock = Clock()
    service = _service(source, clock=clock)
    assert not await service.check_ready()
    assert not await service.check_ready()
    assert source.checks == 1
    clock.now += 29
    assert not await service.check_ready()
    assert source.checks == 1
    source.ok = True  # e.g. the training job created the collection
    clock.now += 2
    assert await service.check_ready()
    assert source.checks == 2
    result = await service.recommend(RecommendationQuery(user_id="u1"))
    assert result.items == []


async def test_a_passed_check_is_not_repeated() -> None:
    source = Source()
    service = _service(source)
    assert await service.check_ready()
    assert await service.check_ready()
    await service.recommend(RecommendationQuery(user_id="u1"))
    assert source.checks == 1


async def test_the_check_names_the_serving_generation() -> None:
    class Recording(Source):
        async def ready(self, generation: str | None) -> bool:
            self.generation = generation
            return True

    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:serving", "g3")
    source = Recording()
    await _service(source, redis).check_ready()
    assert source.generation == "g3"


async def test_concurrent_requests_do_not_leave_tasks_behind() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:popular", '["p1"]')
    service = _service(redis=redis)
    before = len(asyncio.all_tasks())
    await asyncio.gather(
        *(service.recommend(RecommendationQuery(user_id=f"u{i}")) for i in range(20))
    )
    assert len(asyncio.all_tasks()) == before
