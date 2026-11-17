import asyncio
import hashlib
import json
import logging
import time
from collections.abc import Callable

import httpx
import pytest
from fakeredis import aioredis

from app.modules.business.recommendation.adapters.flipt import FliptExperimentAssigner
from app.modules.business.recommendation.adapters.redis_cache import RedisServingCache
from app.modules.business.recommendation.contract.keys import ServingKeys
from app.modules.business.recommendation.placements import load_placements
from app.modules.business.recommendation.ports import Assignment
from app.modules.business.recommendation.schemas import RecommendationQuery
from tests.unit.recs_support import FakeAssigner, FakeStock, Source, build_service, cand

FLAG = "recs-home-feed-ranker"
SALT = "pepper-salt-9f3"


def _assigner(
    handler: Callable[[httpx.Request], httpx.Response], *, timeout_s: float = 0.5
) -> FliptExperimentAssigner:
    client = httpx.AsyncClient(
        transport=httpx.MockTransport(handler), base_url="http://flipt:8080"
    )
    return FliptExperimentAssigner(client, salt=SALT, timeout_s=timeout_s)


def _match(variant: str = "similarity") -> Callable[[httpx.Request], httpx.Response]:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "match": True,
                "variantKey": variant,
                "variantAttachment": json.dumps({"ranking": {"model": "cosine_rank"}}),
            },
        )

    return handler


def test_placement_without_flag_makes_no_call():
    placements = load_placements()
    assert placements.get("similar_items").experiment is None
    assert placements.get("home_feed").experiment is not None


async def test_placement_without_flag_makes_no_call_empty_flag_is_no_call():
    calls: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"match": False})

    result = await _assigner(handler).assign(None, "u1")

    assert result.status == "none"
    assert calls == []


async def test_no_identity_no_arm():
    calls: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"match": True, "variantKey": "control"})

    for entity in (None, ""):
        result = await _assigner(handler).assign(FLAG, entity)
        assert result.status == "none"
        assert result.variant is None
    assert calls == []


async def test_slow_flipt_times_out():
    gate = asyncio.Event()

    async def handler(request: httpx.Request) -> httpx.Response:
        await gate.wait()
        return httpx.Response(200, json={"match": True, "variantKey": "control"})

    started = time.monotonic()
    result = await _assigner(handler, timeout_s=0.02).assign(FLAG, "u1")

    assert result.status == "unassigned"
    assert result.variant is None
    assert time.monotonic() - started < 0.15


async def test_entity_id_is_salted_hash_and_not_logged(
    caplog: pytest.LogCaptureFixture, capsys: pytest.CaptureFixture[str]
):
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return _match()(request)

    with caplog.at_level(logging.DEBUG):
        result = await _assigner(handler).assign(FLAG, "u1")

    expected = hashlib.sha256(f"{SALT}:u1".encode()).hexdigest()
    assert len(seen) == 1
    body = json.loads(seen[0].content)
    assert body["entityId"] == expected
    assert body["flagKey"] == FLAG
    assert body["namespaceKey"] == "default"
    assert seen[0].method == "POST"
    raw_request = seen[0].url.path + str(seen[0].url.query) + seen[0].content.decode()
    assert "u1" not in raw_request.replace(expected, "")
    assert SALT not in raw_request
    assert result == Assignment("similarity", "cosine_rank", "assigned")

    out = capsys.readouterr()
    logged = caplog.text + out.out + out.err
    assert "u1" not in logged.replace("recs-home-feed-ranker", "")
    assert SALT not in logged
    assert expected not in logged


async def test_entity_hash_is_stable_visitor_always_lands_in_same_arm():
    entities: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        entities.append(json.loads(request.content)["entityId"])
        return _match()(request)

    assigner = _assigner(handler)
    for _ in range(3):
        await assigner.assign(FLAG, "anon-7")
    await assigner.assign(FLAG, "anon-8")

    assert entities[0] == entities[1] == entities[2]
    assert entities[3] != entities[0]


async def test_flipt_500_is_unassigned():
    result = await _assigner(lambda r: httpx.Response(500, text="boom")).assign(
        FLAG, "u1"
    )

    assert result.status == "unassigned"
    assert result.variant is None


async def test_unreachable_flipt_is_unassigned_flipt_unreachable_serves_default():
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("refused", request=request)

    result = await _assigner(handler).assign(FLAG, "u1")

    assert result.status == "unassigned"


async def test_unparsable_body_is_unassigned_flipt_unreachable_serves_default():
    result = await _assigner(lambda r: httpx.Response(200, text="<html>")).assign(
        FLAG, "u1"
    )

    assert result.status == "unassigned"


async def test_disabled_flag_means_no_arm():
    result = await _assigner(
        lambda r: httpx.Response(200, json={"match": False})
    ).assign(FLAG, "u1")

    assert result.status == "none"
    assert result.variant is None


async def test_missing_flag_404_means_no_arm_disabled_flag_means_no_arm():
    result = await _assigner(lambda r: httpx.Response(404, json={})).assign(FLAG, "u1")

    assert result.status == "none"


async def test_matched_variant_without_attachment_has_no_ranking_model_visitor_always_lands_in_same_arm():
    result = await _assigner(
        lambda r: httpx.Response(200, json={"match": True, "variantKey": "control"})
    ).assign(FLAG, "u1")

    assert result == Assignment("control", None, "assigned")


ARM_SIMILARITY = f"{FLAG}:similarity"


def _list(*ids: str) -> str:
    return json.dumps([{"listing_id": i, "visible": True} for i in ids])


async def _redis_with_user_list(*ids: str, popular: tuple[str, ...] = ()):
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:serving", "g1")
    await redis.set("recs:v1:gen:g1:user:u1", _list(*ids))
    if popular:
        await redis.set("recs:v1:gen:g1:popular", _list(*popular))
    return redis


async def test_attachment_cannot_change_ladder():
    redis = await _redis_with_user_list("l1", "l2", "l3", popular=("p1", "p2"))
    stock = FakeStock({"l2": 0})
    assigner = FakeAssigner(Assignment("similarity", "cosine_rank", "assigned"))
    service = build_service(redis=redis, stock_lookup=stock, assigner=assigner)

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert [i.listing_id for i in result.items] == ["l1", "l3"]
    assert result.tier == "user_precomputed"
    assert result.ranker == "similarity"
    assert result.arm == ARM_SIMILARITY
    assert not result.fallback
    assert stock.looked_up


async def test_unknown_ranking_model_ignored():
    redis = await _redis_with_user_list("l1", "l2")
    assigner = FakeAssigner(Assignment("similarity", "lambdamart", "assigned"))
    service = build_service(redis=redis, assigner=assigner)

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert [i.listing_id for i in result.items] == ["l1", "l2"]
    assert result.ranker == "fixed_weights"
    assert result.arm == ARM_SIMILARITY


async def test_floor_reports_popular_tier():
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:serving", "g1")
    await redis.set("recs:v1:gen:g1:popular", _list("p1", "p2"))
    service = build_service(redis=redis)

    result = await service.recommend(
        RecommendationQuery(user_id="new", placement_id="home_feed")
    )

    assert [i.listing_id for i in result.items] == ["p1", "p2"]
    assert result.tier == "popular"
    assert result.model_version == "g1"


class _DownRedis(aioredis.FakeRedis):
    async def get(self, *args, **kwargs):
        raise ConnectionError("redis is down")


async def test_unreadable_cache_reports_fallback_generation_and_tier():
    service = build_service(redis=_DownRedis(decode_responses=True))

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert result.model_version == "serving-fallback"
    assert result.tier == "serving-fallback"
    assert result.ranker == "similarity"


async def test_untrained_gbdt_reports_fixed_weights():
    redis = await _redis_with_user_list("l1", "l2")
    service = build_service(redis=redis)

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert result.tier == "user_precomputed"
    assert result.ranker == "fixed_weights"
    assert result.arm == "none"


class _BrokenUserCache(RedisServingCache):
    async def user_candidates(self, user_id, generation):
        raise RuntimeError("bug in the user read")


async def test_unexpected_failure_reports_fallback_tier():
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:serving", "g1")
    await redis.set("recs:v1:gen:g1:popular", _list("p1", "p2"))
    service = build_service(redis=redis)
    service._cache = _BrokenUserCache(redis, ServingKeys("recs", "v1"))

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert [i.listing_id for i in result.items] == ["p1", "p2"]
    assert result.tier == "serving-fallback"
    assert result.fallback


async def test_similarity_arm_changes_the_ranker_segment_only():
    redis = await _redis_with_user_list("l1", "l2")
    default = await build_service(redis=redis).recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )
    arm = await build_service(
        redis=redis,
        assigner=FakeAssigner(Assignment("similarity", "cosine_rank", "assigned")),
    ).recommend(RecommendationQuery(user_id="u1", placement_id="home_feed"))

    assert default.ranker == "fixed_weights"
    assert arm.ranker == "similarity"
    assert arm.tier == default.tier
    assert arm.arm == ARM_SIMILARITY


async def test_unassigned_names_the_default_ranker_and_unassigned_arm():
    redis = await _redis_with_user_list("l1")
    service = build_service(
        redis=redis, assigner=FakeAssigner(Assignment(None, None, "unassigned"))
    )

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert result.ranker == "fixed_weights"
    assert result.arm == f"{FLAG}:unassigned"


async def test_control_variant_without_attachment_keeps_default_ranker():
    redis = await _redis_with_user_list("l1")
    service = build_service(
        redis=redis, assigner=FakeAssigner(Assignment("control", None, "assigned"))
    )

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert result.ranker == "fixed_weights"
    assert result.arm == f"{FLAG}:control"


async def test_assignment_uses_the_placement_flag_and_the_bound_identity():
    redis = await _redis_with_user_list("l1")
    assigner = FakeAssigner(Assignment(None, None, "none"))
    service = build_service(redis=redis, assigner=assigner)

    await service.recommend(
        RecommendationQuery(user_id="u1", anonymous_id="a1", placement_id="home_feed")
    )
    await service.recommend(
        RecommendationQuery(anonymous_id="a1", placement_id="home_feed")
    )

    assert assigner.calls == [(FLAG, "u1"), (FLAG, "a1")]


async def test_a_buyer_with_precomputed_recommendations_reports_the_user_tier():
    redis = await _redis_with_user_list("l1")
    result = await build_service(redis=redis).recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert result.tier == "user_precomputed"
    assert result.model_version == "g1"


async def test_a_similar_items_request_reports_the_seed_tier_and_similarity_ranker():
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:serving", "g1")
    assigner = FakeAssigner(Assignment("control", None, "assigned"))
    service = build_service(
        Source({"seed": [cand("n1", 0.9)]}), redis, assigner=assigner
    )

    result = await service.recommend(
        RecommendationQuery(seed_listing_id="seed", placement_id="similar_items")
    )

    assert result.tier == "seed_vector_similarity"
    assert result.ranker == "similarity"
    assert result.arm == "none"
