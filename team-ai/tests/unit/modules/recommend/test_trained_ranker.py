"""recsys-gbdt-trainer D7: the loader of the trained ``agora-gbdt/1`` ranker artifact."""

from __future__ import annotations

import json
from typing import Any

import pytest
from fakeredis import aioredis

from app.modules.business.recommend.factory import build_recommendation_service
from app.modules.business.recommend.features import RANKING_FEATURES
from app.modules.business.recommend.ranker_artifact import (
    ArtifactError,
    TrainedRanker,
)
from app.modules.business.recommend.schemas import RecommendQuery
from tests.factories import build_test_settings

_PRICE = RANKING_FEATURES.index("item_attributes.price")
_VIEWS = RANKING_FEATURES.index("item_popularity.views_7d")


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
    x, source = model.vector(pop, {"price": float("inf")})
    assert (x[_VIEWS], x[_PRICE], source) == (4.0, 0.0, "fallback")
    assert x[RANKING_FEATURES.index("item_popularity.ctr_7d")] == 0.2
    x, source = model.vector(pop, {"price": 5}, nearline_ctr=0.7)
    assert (x[_PRICE], source) == (5.0, "nearline")
    assert x[RANKING_FEATURES.index("item_popularity.ctr_7d")] == 0.7


@pytest.mark.parametrize(
    ("doc", "reason"),
    [
        (artifact(features=["item_popularity.views_7d"]), "feature_mismatch"),
        (artifact(features=list(reversed(RANKING_FEATURES))), "feature_mismatch"),
        (artifact(format="agora-gbdt/2"), "format_mismatch"),
        (artifact(trees=[]), "malformed"),
        (
            artifact(trees=[{"feature": [0, -1], "threshold": [1.0]}]),
            "tree.threshold malformed",
        ),
        (artifact(learning_rate="x"), "malformed"),
    ],
)
def test_unusable_artifacts_are_rejected(doc: dict[str, Any], reason: str) -> None:
    with pytest.raises(ArtifactError, match=reason):
        TrainedRanker.parse(json.dumps(doc))


def test_garbage_is_malformed() -> None:
    with pytest.raises(ArtifactError, match="malformed"):
        TrainedRanker.parse("{not json")


# --- through the factory: the serving path ------------------------------------------

_LIST = [
    {"listing_id": "cheap", "score": 0.9, "category_id": "c"},
    {"listing_id": "dear", "score": 0.8, "category_id": "c"},
]


async def _stack(gen: str = "g1", ranker: str | None = None) -> Any:
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set("recs:v1:serving", gen)
    await redis.set(f"recs:v1:gen:{gen}:user:u1", json.dumps(_LIST))
    await redis.set("fs:item_popularity:current", "1")
    await redis.set("fs:item_attributes:current", "1")
    for lid, price in (("cheap", 50), ("dear", 500)):
        await redis.set(f"fs:item_popularity:v1:{lid}", json.dumps({"views_7d": 1}))
        await redis.set(f"fs:item_attributes:v1:{lid}", json.dumps({"price": price}))
    if ranker is not None:
        await redis.set(f"recs:v1:gen:{gen}:ranker", ranker)
    return redis


@pytest.fixture
async def service_for(monkeypatch):
    from redis.asyncio import Redis

    async def build(redis):
        monkeypatch.setattr(Redis, "from_url", staticmethod(lambda url, **kw: redis))
        return await build_recommendation_service(
            build_test_settings(
                RECS_ENABLED=True,
                RECS_BACKEND="memory",
                RECS_FEATURESTORE_REDIS_URL="redis://fs/2",
            ),
            redis=redis,
        )

    return build


async def _home(service):
    return await service.recommend(
        RecommendQuery(
            user_id="u1", placement_id="home_feed", limit=2, include_explain=True
        )
    )


async def test_trained_artifact_reorders_vs_fixed_weights(service_for) -> None:
    fixed = await _home(await service_for(await _stack()))
    assert [i.listing_id for i in fixed.items] == ["cheap", "dear"]
    assert fixed.explain["ranker_source"] == "fixed"
    assert fixed.explain["ranker_fallback"] == "absent"

    trained = await _home(
        await service_for(await _stack(ranker=json.dumps(artifact())))
    )
    assert trained.explain["ranker_source"] == "trained"
    assert [i.listing_id for i in trained.items] == ["dear", "cheap"]
    assert trained.items[0].score == pytest.approx(0.5 + 0.1 * 1.0)


async def test_wrong_feature_list_falls_back_and_is_counted(service_for) -> None:
    bad = json.dumps(artifact(features=list(RANKING_FEATURES[:-1])))
    service = await service_for(await _stack(ranker=bad))
    first, second = await _home(service), await _home(service)
    assert [i.listing_id for i in first.items] == ["cheap", "dear"]
    assert first.explain["ranker_source"] == "fixed"
    assert first.explain["ranker_fallback"] == "feature_mismatch"
    assert second.explain["ranker_fallbacks"] == 1  # counted once per generation load


async def test_generation_switch_reloads_the_model(service_for) -> None:
    redis = await _stack(ranker=json.dumps(artifact()))
    service = await service_for(redis)
    service._cache._pointer_ttl_s = 0.0  # do not memoise the pointer
    assert (await _home(service)).explain["ranker_source"] == "trained"

    # Next generation ships no ranker (e.g. the candidate was rejected): fixed again.
    await redis.set("recs:v1:gen:g2:user:u1", json.dumps(_LIST))
    await redis.set("recs:v1:serving", "g2")
    switched = await _home(service)
    assert switched.explain["ranker_source"] == "fixed"
    assert [i.listing_id for i in switched.items] == ["cheap", "dear"]

    await redis.set("recs:v1:gen:g2:ranker", json.dumps(artifact()))
    await redis.set("recs:v1:serving", "g3")
    await redis.set("recs:v1:gen:g3:user:u1", json.dumps(_LIST))
    await redis.set("recs:v1:gen:g3:ranker", json.dumps(artifact(base_score=1.0)))
    again = await _home(service)
    assert again.explain["ranker_source"] == "trained"
    assert again.items[0].score == pytest.approx(1.0 + 0.1 * 1.0)


async def test_late_publish_in_the_same_generation_is_picked_up(service_for) -> None:
    redis = await _stack()
    service = await service_for(redis)
    loader = service._ranker_loader
    loader._recheck_s = 0.0
    assert (await _home(service)).explain["ranker_source"] == "fixed"
    await redis.set("recs:v1:gen:g1:ranker", json.dumps(artifact()))
    assert (await _home(service)).explain["ranker_source"] == "trained"


async def test_a_deleted_or_replaced_artifact_is_seen_in_the_same_generation(
    service_for,
) -> None:
    # Artifacts are written once per generation, but an operator can remove or replace one
    # (a bad model, an e2e teardown): the loader must not keep serving what is gone.
    redis = await _stack(ranker=json.dumps(artifact()))
    service = await service_for(redis)
    service._ranker_loader._recheck_s = 0.0
    assert (await _home(service)).explain["ranker_source"] == "trained"
    await redis.delete("recs:v1:gen:g1:ranker")
    gone = await _home(service)
    assert gone.explain["ranker_source"] == "fixed"
    assert [i.listing_id for i in gone.items] == ["cheap", "dear"]
    await redis.set("recs:v1:gen:g1:ranker", json.dumps(artifact(base_score=1.0)))
    replaced = await _home(service)
    assert replaced.items[0].score == pytest.approx(1.0 + 0.1 * 1.0)
