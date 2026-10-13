"""The composition root: settings and clients in, one wired service out."""

import json
from dataclasses import dataclass
from types import SimpleNamespace
from typing import Any, Literal

from fakeredis import aioredis

from app.modules.business.recommendation.adapters.memory import MemoryCandidateSource
from app.modules.business.recommendation.adapters.qdrant import QdrantCandidateSource
from app.modules.business.recommendation.contract.features import RANKING_FEATURES
from app.modules.business.recommendation.factory import (
    RecommendationClients,
    build_candidate_source,
    build_recommendation_service,
)
from app.modules.business.recommendation.schemas import RecommendationQuery


@dataclass(frozen=True)
class RecsSettings:
    """The ``RECS_*`` names the factory reads, with team-ai's defaults."""

    RECS_QDRANT_COLLECTION: str = "item_als_vectors"
    RECS_VECTOR_DIM: int = 64
    RECS_QDRANT_DISTANCE: Literal["Cosine", "Dot", "Euclid"] = "Cosine"
    RECS_CANDIDATE_TOP_K: int = 100
    RECS_RESULT_TOP_K: int = 10
    RECS_CACHE_PREFIX: str = "recs"
    RECS_CACHE_SCHEMA_VERSION: str = "v1"
    RECS_RETRIEVE_TIMEOUT_MS: int = 15
    RECS_MODEL_VERSION: str = "serving-fallback"
    RECS_NEARLINE_PREFIX: str = "recs:nearline"
    RECS_NEARLINE_MIN_IMPRESSIONS: float = 1.0
    RECS_NEARLINE_TIMEOUT_MS: int = 20


# The memory source never touches Qdrant, so a placeholder client is enough.
NO_QDRANT: Any = SimpleNamespace()


def _fake() -> Any:
    return aioredis.FakeRedis(decode_responses=True)


def _artifact() -> str:
    price = RANKING_FEATURES.index("item_attributes.price")
    return json.dumps(
        {
            "format": "agora-gbdt/1",
            "features": list(RANKING_FEATURES),
            "ctr_feature": "item_popularity.ctr_7d",
            "base_score": 0.5,
            "learning_rate": 0.1,
            "trees": [
                {
                    "feature": [price, -1, -1],
                    "threshold": [100.0, 0.0, 0.0],
                    "left": [1, -1, -1],
                    "right": [2, -1, -1],
                    "value": [0.0, 0.0, 1.0],
                }
            ],
        }
    )


async def test_memory_backend_wires_the_whole_pipeline_over_the_given_clients() -> None:
    serving, featurestore, nearline = _fake(), _fake(), _fake()
    await serving.set("recs:v1:serving", "g1")
    await serving.set(
        "recs:v1:gen:g1:user:u1",
        json.dumps(
            [
                {"listing_id": "cheap", "score": 0.9},
                {"listing_id": "dear", "score": 0.8},
            ]
        ),
    )
    await serving.set("recs:v1:gen:g1:ranker", _artifact())
    await featurestore.set("fs:item_popularity:current", "1")
    await featurestore.set("fs:item_attributes:current", "1")
    for listing, price in (("cheap", 50), ("dear", 500)):
        await featurestore.set(
            f"fs:item_popularity:v1:{listing}", json.dumps({"views_7d": 1})
        )
        await featurestore.set(
            f"fs:item_attributes:v1:{listing}", json.dumps({"price": price})
        )
    await nearline.hset(
        "recs:nearline:ctr:cheap", mapping={"clicks_ips": "1", "imprs_ips": "10"}
    )

    service = build_recommendation_service(
        RecsSettings(),
        source=MemoryCandidateSource(),
        serving_redis=serving,
        clients=RecommendationClients(
            qdrant=NO_QDRANT, featurestore=featurestore, nearline=nearline
        ),
    )

    assert await service.check_ready()
    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed", limit=2)
    )
    # the trained model (price > 100 scores higher) beats retrieval order
    assert [i.listing_id for i in result.items] == ["dear", "cheap"]
    assert result.model_version == "g1"


async def test_without_featurestore_or_nearline_clients_ranking_still_serves() -> None:
    serving = _fake()
    await serving.set("recs:v1:user:u1", '["a","b"]')
    service = build_recommendation_service(
        RecsSettings(),
        source=MemoryCandidateSource(),
        serving_redis=serving,
        clients=RecommendationClients(
            qdrant=NO_QDRANT, featurestore=None, nearline=None
        ),
    )
    result = await service.recommend(RecommendationQuery(user_id="u1"))
    assert [i.listing_id for i in result.items] == ["a", "b"]


async def test_the_memory_source_has_no_vectors_of_its_own() -> None:
    service = build_recommendation_service(
        RecsSettings(),
        source=MemoryCandidateSource(),
        serving_redis=_fake(),
        clients=RecommendationClients(
            qdrant=NO_QDRANT, featurestore=None, nearline=None
        ),
    )
    result = await service.recommend(
        RecommendationQuery(anonymous_id="a", seed_listing_id="seed")
    )
    assert result.items == []


def test_the_candidate_source_wraps_the_qdrant_client() -> None:
    clients = RecommendationClients(
        qdrant=SimpleNamespace(),  # pyright: ignore[reportArgumentType]
        featurestore=None,
        nearline=None,
    )

    assert isinstance(
        build_candidate_source(RecsSettings(), clients), QdrantCandidateSource
    )
