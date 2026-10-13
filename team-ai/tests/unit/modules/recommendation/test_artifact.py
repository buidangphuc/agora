"""The trained ``agora-gbdt/1`` ranker: parse, score, and the per-generation loader."""

import json
from typing import Any

import pytest
from fakeredis import aioredis

from app.modules.business.recommendation.adapters.redis_cache import RedisServingCache
from app.modules.business.recommendation.contract.features import RANKING_FEATURES
from app.modules.business.recommendation.contract.keys import ServingKeys
from app.modules.business.recommendation.ranking.artifact import (
    ArtifactError,
    TrainedRanker,
    TrainedRankerLoader,
)
from app.modules.business.recommendation.schemas import Candidate

_PRICE = RANKING_FEATURES.index("item_attributes.price")
_VIEWS = RANKING_FEATURES.index("item_popularity.views_7d")
_CTR = RANKING_FEATURES.index("item_popularity.ctr_7d")


def artifact(**overrides: Any) -> dict[str, Any]:
    """Two trees: price > 100 adds 1.0; views_7d > 10 adds 0.5. score = 0.5 + 0.1 * sum."""
    doc: dict[str, Any] = {
        "format": "agora-gbdt/1",
        "objective": "lambdarank",
        "model_version": "gbdt-g1",
        "features": list(RANKING_FEATURES),
        "ctr_feature": "item_popularity.ctr_7d",
        "base_score": 0.5,
        "learning_rate": 0.1,
        "trees": [
            {
                "feature": [_PRICE, -1, -1],
                "threshold": [100.0, 0.0, 0.0],
                "left": [1, -1, -1],
                "right": [2, -1, -1],
                "value": [0.0, 0.0, 1.0],
            },
            {
                "feature": [_VIEWS, -1, -1],
                "threshold": [10.0, 0.0, 0.0],
                "left": [1, -1, -1],
                "right": [2, -1, -1],
                "value": [0.0, 0.0, 0.5],
            },
        ],
    }
    doc.update(overrides)
    return doc


def test_parse_and_score_match_a_hand_computed_tree() -> None:
    model = TrainedRanker.parse(json.dumps(artifact()))
    x = [0.0] * len(RANKING_FEATURES)
    assert model.score(x) == pytest.approx(0.5)
    x[_PRICE], x[_VIEWS] = 200.0, 10.0  # price right (1.0); views <= 10 left (0)
    assert model.score(x) == pytest.approx(0.5 + 0.1 * 1.0)
    x[_VIEWS] = 11.0
    assert model.score(x) == pytest.approx(0.5 + 0.1 * 1.5)


def test_vector_defaults_and_nearline_replaces_ctr() -> None:
    model = TrainedRanker.parse(json.dumps(artifact()))
    pop = {"views_7d": 4, "ctr_7d": 0.2, "avg_rating": None, "clicks_7d": "x"}
    x = model.vector(pop, {"price": float("inf")})
    assert (x[_VIEWS], x[_PRICE], x[_CTR]) == (4.0, 0.0, 0.2)
    x = model.vector(pop, {"price": 5}, nearline_ctr=0.7)
    assert (x[_PRICE], x[_CTR]) == (5.0, 0.7)
    assert model.vector(pop, {}, nearline_ctr=0.0)[_CTR] == 0.2  # zero is no data
    assert model.vector(pop, {}, nearline_ctr=3.0)[_CTR] == 1.0  # capped


@pytest.mark.parametrize(
    ("doc", "reason"),
    [
        (artifact(features=["item_popularity.views_7d"]), "feature_mismatch"),
        (artifact(features=list(reversed(RANKING_FEATURES))), "feature_mismatch"),
        (artifact(format="agora-gbdt/2"), "format_mismatch"),
        (artifact(trees=[]), "malformed"),
        (artifact(trees=[{"feature": [0, -1], "threshold": [1.0]}]), "malformed"),
        (artifact(learning_rate="x"), "malformed"),
        (artifact(base_score=float("inf")), "malformed"),
        (
            artifact(
                trees=[
                    {
                        "feature": [99, -1, -1],
                        "threshold": [0.0, 0.0, 0.0],
                        "left": [1, -1, -1],
                        "right": [2, -1, -1],
                        "value": [0.0, 0.0, 1.0],
                    }
                ]
            ),
            "malformed",
        ),
        (
            artifact(
                trees=[
                    {
                        "feature": [0, -1, -1],
                        "threshold": [0.0, 0.0, 0.0],
                        "left": [1, -1, -1],
                        "right": [7, -1, -1],
                        "value": [0.0, 0.0, 1.0],
                    }
                ]
            ),
            "malformed",
        ),
        (
            # a cycle: node 0 -> node 1 -> node 0, which can never reach a leaf
            artifact(
                trees=[
                    {
                        "feature": [0, 0, -1],
                        "threshold": [0.0, 0.0, 0.0],
                        "left": [1, 0, -1],
                        "right": [2, 0, -1],
                        "value": [0.0, 0.0, 1.0],
                    }
                ]
            ),
            "malformed",
        ),
    ],
)
def test_unusable_artifacts_are_rejected(doc: dict[str, Any], reason: str) -> None:
    with pytest.raises(ArtifactError) as raised:
        TrainedRanker.parse(json.dumps(doc))
    assert str(raised.value) == reason


@pytest.mark.parametrize("raw", ["{not json", "[]", "null"])
def test_garbage_is_malformed(raw: str) -> None:
    with pytest.raises(ArtifactError) as raised:
        TrainedRanker.parse(raw)
    assert str(raised.value) == "malformed"


def test_ranking_orders_by_model_score_then_retrieval_similarity() -> None:
    model = TrainedRanker.parse(json.dumps(artifact()))
    cands = [
        Candidate("cheap", 0.9),
        Candidate("dear", 0.8),
        Candidate("tie-high", 0.7),
        Candidate("tie-low", 0.6),
        Candidate("seed", 1.0),
        Candidate("gone", 0.99, in_stock=False),
    ]
    attrs = {
        "cheap": {"price": 50},
        "dear": {"price": 500},
        "tie-high": {"price": 50},
        "tie-low": {"price": 50},
    }
    items = model.rank_candidates(cands, "seed", {}, attrs, {}, limit=3)
    assert [i.listing_id for i in items] == ["dear", "cheap", "tie-high"]
    assert [i.rank for i in items] == [1, 2, 3]
    assert items[0].score == pytest.approx(0.5 + 0.1 * 1.0)


# --- the loader ----------------------------------------------------------------------


class Clock:
    def __init__(self) -> None:
        self.now = 1000.0

    def __call__(self) -> float:
        return self.now


async def _loader(
    redis: Any, clock: Clock | None = None, recheck_s: float = 5.0
) -> TrainedRankerLoader:
    clock = clock or Clock()
    cache = RedisServingCache(redis, ServingKeys("recs", "v1"), clock=clock)
    return TrainedRankerLoader(cache, clock=clock, recheck_s=recheck_s)


async def test_no_generation_means_no_model() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    assert await (await _loader(redis)).get(None) is None


async def test_loads_the_ranker_of_the_serving_generation() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:gen:g1:ranker", json.dumps(artifact()))
    model = await (await _loader(redis)).get("g1")
    assert model is not None
    x = [0.0] * len(RANKING_FEATURES)
    assert model.score(x) == pytest.approx(0.5)


async def test_an_absent_artifact_means_no_model() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    assert await (await _loader(redis)).get("g1") is None


async def test_a_rejected_artifact_means_no_model() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    bad = json.dumps(artifact(features=list(RANKING_FEATURES[:-1])))
    await redis.set("recs:v1:gen:g1:ranker", bad)
    assert await (await _loader(redis)).get("g1") is None


async def test_the_verdict_is_kept_until_the_recheck_delay() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    clock = Clock()
    loader = await _loader(redis, clock)
    assert await loader.get("g1") is None
    await redis.set("recs:v1:gen:g1:ranker", json.dumps(artifact()))
    clock.now += 4.9
    assert await loader.get("g1") is None  # still within the delay
    clock.now += 0.2
    assert await loader.get("g1") is not None  # late publish picked up


async def test_an_unchanged_payload_is_not_reparsed() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:gen:g1:ranker", json.dumps(artifact()))
    clock = Clock()
    loader = await _loader(redis, clock)
    first = await loader.get("g1")
    clock.now += 10
    assert await loader.get("g1") is first


async def test_a_deleted_or_replaced_artifact_is_seen_in_the_same_generation() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:gen:g1:ranker", json.dumps(artifact()))
    clock = Clock()
    loader = await _loader(redis, clock)
    first = await loader.get("g1")
    assert first is not None
    await redis.delete("recs:v1:gen:g1:ranker")
    clock.now += 6
    assert await loader.get("g1") is None
    await redis.set("recs:v1:gen:g1:ranker", json.dumps(artifact(base_score=1.0)))
    clock.now += 6
    replaced = await loader.get("g1")
    assert replaced is not None and replaced is not first
    x = [0.0] * len(RANKING_FEATURES)
    assert replaced.score(x) == pytest.approx(1.0)


async def test_a_generation_switch_reloads_the_model_at_once() -> None:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:gen:g1:ranker", json.dumps(artifact()))
    await redis.set("recs:v1:gen:g3:ranker", json.dumps(artifact(base_score=1.0)))
    loader = await _loader(redis)
    g1 = await loader.get("g1")
    assert g1 is not None
    assert await loader.get("g2") is None  # g2 shipped no ranker
    g3 = await loader.get("g3")
    assert g3 is not None and g3 is not g1
    assert g3.score([0.0] * len(RANKING_FEATURES)) == pytest.approx(1.0)
