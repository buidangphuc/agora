"""Execution proof that the *factory-built* service reaches gbdt, the feature store, nearline.

Module tests inject collaborators straight into ``RecommendationService`` and so cannot see
a collaborator the factory forgot to pass. Every test here goes through
``build_recommendation_service`` with settings, and only fakes Redis.
"""

from __future__ import annotations

import json
from typing import Any

import pytest
from fakeredis import aioredis

from app.modules.business.recommend.factory import build_recommendation_service
from app.modules.business.recommend.schemas import RecommendQuery
from tests.factories import build_test_settings

# Cosine order is A (0.9) > B (0.8); only B has online features, with a strong ctr_7d.
_USER_LIST = [
    {"listing_id": "item-a", "score": 0.9, "category_id": "c"},
    {"listing_id": "item-b", "score": 0.8, "category_id": "c"},
]


async def _redis(feature_json: dict[str, dict[str, Any]] | None = None) -> Any:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:user:u1", json.dumps(_USER_LIST))
    if feature_json:
        await redis.set("fs:item_popularity:current", "3")
        for lid, feats in feature_json.items():
            await redis.set(f"fs:item_popularity:v3:{lid}", json.dumps(feats))
    return redis


def _settings(**overrides: object):
    return build_test_settings(RECS_ENABLED=True, RECS_BACKEND="memory", **overrides)


@pytest.fixture
def feature_redis(monkeypatch):
    """Make ``RECS_FEATURESTORE_REDIS_URL`` resolve to a given fake Redis."""
    from redis.asyncio import Redis

    holder: dict[str, Any] = {}
    monkeypatch.setattr(
        Redis, "from_url", staticmethod(lambda url, **kw: holder["redis"])
    )
    return holder


async def _home(service, user: str = "u1"):
    return await service.recommend(
        RecommendQuery(
            user_id=user, placement_id="home_feed", limit=2, include_explain=True
        )
    )


async def test_factory_built_service_reads_the_feature_store_and_ranks_with_gbdt(
    feature_redis,
):
    fs = await _redis({"item-b": {"ctr_7d": 0.9, "favorites_current": 3}})
    feature_redis["redis"] = fs
    service = await build_recommendation_service(
        _settings(RECS_FEATURESTORE_REDIS_URL="redis://featurestore/2"), redis=fs
    )

    result = await _home(service)

    assert result.explain["ranking_model"] == "gbdt"
    assert result.explain["featurestore_hit_count"] == 1
    # The feature store flipped the cosine order (A, B): the ranker really consumed it.
    assert [i.listing_id for i in result.items] == ["item-b", "item-a"]


async def test_without_a_feature_store_url_the_same_request_keeps_cosine_order():
    plain = await _redis()
    service = await build_recommendation_service(_settings(), redis=plain)

    result = await _home(service)

    assert result.explain["ranking_model"] == "gbdt"
    assert result.explain["featurestore_hit_count"] == 0
    assert [i.listing_id for i in result.items] == ["item-a", "item-b"]


async def test_non_gbdt_placement_keeps_retrieval_order_through_the_factory():
    redis = await _redis()
    service = await build_recommendation_service(_settings(), redis=redis)

    result = await service.recommend(
        RecommendQuery(
            seed_listing_id="listing-1",
            placement_id="similar_items",
            limit=4,
            include_explain=True,
        )
    )

    scores = [i.score for i in result.items]
    assert result.explain["ranking_model"] == "cosine_rank"
    assert scores == sorted(scores, reverse=True)
    assert result.explain["featurestore_hit_count"] == 0
