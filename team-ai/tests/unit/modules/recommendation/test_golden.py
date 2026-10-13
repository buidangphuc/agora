"""Golden replay: the new service reproduces what team-ai's RecommendationService recorded."""

import json
from pathlib import Path
from typing import Any

import pytest
from fakeredis import aioredis

from app.modules.business.recommendation.adapters.featurestore import (
    RedisFeatureStore,
)
from app.modules.business.recommendation.adapters.memory import MemoryCandidateSource
from app.modules.business.recommendation.adapters.nearline import RedisNearlineSignals
from app.modules.business.recommendation.adapters.redis_cache import RedisServingCache
from app.modules.business.recommendation.contract.features import (
    ITEM_ATTRIBUTES_VIEW,
    ITEM_POPULARITY_VIEW,
)
from app.modules.business.recommendation.contract.keys import ServingKeys
from app.modules.business.recommendation.placements import load_placements
from app.modules.business.recommendation.ranking.artifact import TrainedRankerLoader
from app.modules.business.recommendation.schemas import (
    Candidate,
    RecommendationQuery,
    RecommendationResult,
)
from app.modules.business.recommendation.service import RecommendationService

FIXTURE = Path(__file__).parents[3] / "fixtures" / "recommendation_golden.json"
GOLDEN: dict[str, Any] = json.loads(FIXTURE.read_text(encoding="utf-8"))
CASES: list[dict[str, Any]] = GOLDEN["cases"]
BY_NAME = {c["name"]: c for c in CASES}


class UserReadFails(RedisServingCache):
    """The recorded fault: the per-user list read raises instead of failing open."""

    async def user_candidates(
        self, user_id: str, generation: str | None
    ) -> list[Candidate]:
        raise ConnectionError("user list unreadable")


def _service(case: dict[str, Any], redis: Any) -> RecommendationService:
    overrides = case["settings"]
    keys = ServingKeys("recs", "v1")
    cache_type = (
        UserReadFails if case["fault"] == "user_cache_raises" else RedisServingCache
    )
    cache = cache_type(redis, keys)
    source = MemoryCandidateSource(
        {
            seed: [Candidate(r["listing_id"], r["score"], r["in_stock"]) for r in rows]
            for seed, rows in case["similar"].items()
        }
    )
    on = case["featurestore"]
    return RecommendationService(
        source=source,
        cache=cache,
        placements=load_placements(),
        popularity_store=RedisFeatureStore(redis, ITEM_POPULARITY_VIEW) if on else None,
        attribute_store=RedisFeatureStore(redis, ITEM_ATTRIBUTES_VIEW) if on else None,
        nearline=RedisNearlineSignals(
            redis,
            prefix="recs:nearline",
            min_impressions=overrides.get("nearline_min_impressions", 1.0),
            timeout_s=1.0,
        )
        if case["nearline"]
        else None,
        ranker_loader=TrainedRankerLoader(cache),
        candidate_top_k=100,
        result_top_k=10,
        model_version=overrides.get("model_version", "serving-fallback"),
    )


def _wire(result: RecommendationResult) -> dict[str, Any]:
    return {
        "items": [
            {"listing_id": i.listing_id, "score": round(i.score, 6), "rank": i.rank}
            for i in result.items
        ],
        "model_version": result.model_version,
        "placement_id": result.placement_id,
        "fallback": result.fallback,
    }


async def _replay(case: dict[str, Any]) -> dict[str, Any]:
    redis = aioredis.FakeRedis(decode_responses=True)
    for key, value in case["redis"].items():
        await redis.set(key, value)
    for key, fields in case["hashes"].items():
        await redis.hset(key, mapping=fields)  # pyright: ignore[reportGeneralTypeIssues]
    result = await _service(case, redis).recommend(RecommendationQuery(**case["query"]))
    return _wire(result)


def test_fixture_names_its_source() -> None:
    assert GOLDEN["source"].startswith("team-ai@")


def test_fixture_covers_the_ladder() -> None:
    names = {c["name"] for c in CASES}
    assert len(names) == len(CASES)
    assert {
        "user_precomputed_gbdt_no_featurestore",
        "seed_vector_similarity",
        "cosine_filters_stock_duplicates_and_seed",
        "user_precomputed_gbdt_featurestore",
        "nearline_ctr_above_min_impressions",
        "nearline_ctr_below_min_impressions",
        "trained_artifact",
        "trained_artifact_feature_mismatch_rejected",
        "ladder_exhausted_popular_floor",
        "internal_error_serving_fallback_popular",
        "model_version_serving_fallback_uses_generation",
        "model_version_serving_fallback_without_generation",
        "legacy_unscoped_keys_without_generation",
        "limit_zero_uses_placement_limit",
        "limit_explicit_overrides_placement",
    } <= names


def test_fixture_cases_differ_where_the_scenario_says_they_do() -> None:
    def items(name: str) -> list[dict[str, Any]]:
        return BY_NAME[name]["result"]["items"]

    assert items("trained_artifact") != items(
        "trained_artifact_feature_mismatch_rejected"
    )
    assert items("nearline_ctr_above_min_impressions") != items(
        "nearline_ctr_below_min_impressions"
    )
    assert BY_NAME["internal_error_serving_fallback_popular"]["result"]["fallback"]
    assert BY_NAME["ladder_exhausted_popular_floor"]["result"]["fallback"]
    versions = {c["result"]["model_version"] for c in CASES}
    assert {"serving-fallback", "g1", "g2", "v0", "als-7"} <= versions


@pytest.mark.parametrize("case", CASES, ids=lambda c: c["name"])
async def test_replay_matches_team_ai(case: dict[str, Any]) -> None:
    assert await _replay(case) == case["result"]
