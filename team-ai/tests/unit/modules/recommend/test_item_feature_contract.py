"""Serving reads the feature store's registry names, and says when it had to default."""

from __future__ import annotations

import json
from pathlib import Path

import pytest
import yaml
from fakeredis import aioredis

from app.modules.business.recommend.factory import build_recommendation_service
from app.modules.business.recommend.features import (
    ITEM_POPULARITY_FEATURES,
    item_feature,
    missing_item_features,
    popularity,
)
from app.modules.business.recommend.ranking import GBDTRankerAdapter
from app.modules.business.recommend.schemas import Candidate, RecommendQuery
from tests.factories import build_test_settings

REGISTRY = (
    Path(__file__).resolve().parents[5]
    / "platform-featurestore"
    / "registry"
    / "features.yaml"
)

# The pre-registry ad-hoc names the ranker used to read; no view ever produced them.
OLD_NAMES = {
    "category_match": 1.0,
    "popularity_score": 99.0,
    "historical_ctr": 0.9,
    "conversion_rate": 0.9,
    "freshness_score": 1.0,
    "price": 1.0,
}


@pytest.mark.skipif(
    not REGISTRY.exists(), reason="platform-featurestore not checked out"
)
def test_item_feature_list_matches_the_registry_view():
    views = {v["name"]: v for v in yaml.safe_load(REGISTRY.read_text())["views"]}
    assert list(ITEM_POPULARITY_FEATURES) == list(views["item_popularity"]["features"])


def test_old_names_are_not_features_and_count_as_missing():
    assert missing_item_features(OLD_NAMES) == list(ITEM_POPULARITY_FEATURES)
    assert popularity(OLD_NAMES) == 0.0


def test_registry_names_drive_the_ranker_vector():
    ranker = GBDTRankerAdapter()
    query = RecommendQuery(placement_id="home_feed")
    cand = Candidate(listing_id="x", score=0.5)
    real = {"ctr_7d": 0.2, "views_7d": 400, "clicks_7d": 80, "add_to_cart_7d": 20}

    with_real = ranker.extract_features(cand, query, real).values
    with_old = ranker.extract_features(cand, query, OLD_NAMES).values

    assert with_real[5] == 0.2 and with_real[2] > 0
    assert with_old[5] == 0.0 and with_old[2] == 0.0  # the old names change nothing


def test_missing_null_and_garbage_values_take_the_documented_default():
    row = {
        "views_7d": 5,
        "avg_rating": None,
        "ctr_7d": "n/a",
        "clicks_7d": float("nan"),
    }
    assert item_feature(row, "views_7d") == (5.0, False)
    assert item_feature(row, "avg_rating") == (0.0, True)
    assert item_feature(row, "ctr_7d") == (0.0, True)
    assert item_feature(row, "clicks_7d") == (0.0, True)
    assert set(missing_item_features(row)) == set(ITEM_POPULARITY_FEATURES) - {
        "views_7d"
    }


async def test_service_counts_defaulted_features_and_does_not_crash(monkeypatch):
    from redis.asyncio import Redis

    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set(
        "recs:v1:user:u1",
        json.dumps(
            [
                {"listing_id": "full", "score": 0.5},
                {"listing_id": "stale-schema", "score": 0.5},
            ]
        ),
    )
    await redis.set("fs:item_popularity:current", "1")
    full = dict.fromkeys(ITEM_POPULARITY_FEATURES, 1)
    await redis.set("fs:item_popularity:v1:full", json.dumps(full))
    await redis.set("fs:item_popularity:v1:stale-schema", json.dumps(OLD_NAMES))
    monkeypatch.setattr(Redis, "from_url", staticmethod(lambda url, **kw: redis))
    service = await build_recommendation_service(
        build_test_settings(
            RECS_ENABLED=True,
            RECS_BACKEND="memory",
            RECS_FEATURESTORE_REDIS_URL="redis://fs/2",
        ),
        redis=redis,
    )

    result = await service.recommend(
        RecommendQuery(user_id="u1", placement_id="home_feed", include_explain=True)
    )

    assert result.explain["featurestore_hit_count"] == 2
    assert result.explain["feature_defaults"] == len(ITEM_POPULARITY_FEATURES)
    assert result.status != "degraded" and len(result.items) == 2
