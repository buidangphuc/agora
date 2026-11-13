"""The composition root: settings and clients in, one wired service out."""

import json
from dataclasses import dataclass
from types import SimpleNamespace
from typing import Any, Literal

from fakeredis import aioredis

from app.modules.business.recommendation.adapters.memory import (
    MemoryCandidateSource,
    MemoryStockLookup,
)
from app.modules.business.recommendation.adapters.flipt import FliptExperimentAssigner
from app.modules.business.recommendation.adapters.qdrant import QdrantCandidateSource
from app.modules.business.recommendation.contract.features import RANKING_FEATURES
from app.modules.business.recommendation.factory import (
    RecommendationClients,
    build_candidate_source,
    build_experiment_assigner,
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
    RECS_FLIPT_URL: str = "http://flipt:8080"
    RECS_FLIPT_TIMEOUT_MS: int = 50
    RECS_EXPERIMENT_SALT: str = "salt"


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
                {"listing_id": "cheap", "score": 0.9, "visible": True},
                {"listing_id": "dear", "score": 0.8, "visible": True},
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
            qdrant=NO_QDRANT,
            featurestore=featurestore,
            nearline=nearline,
            stock=MemoryStockLookup({"cheap": 5, "dear": 5}),
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
    await serving.set(
        "recs:v1:user:u1",
        json.dumps(
            [{"listing_id": "a", "visible": True}, {"listing_id": "b", "visible": True}]
        ),
    )
    service = build_recommendation_service(
        RecsSettings(),
        source=MemoryCandidateSource(),
        serving_redis=serving,
        clients=RecommendationClients(
            qdrant=NO_QDRANT,
            featurestore=None,
            nearline=None,
            stock=MemoryStockLookup({"a": 5, "b": 5}),
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


def test_factory_builds_the_flipt_assigner_from_settings_flipt_unreachable_serves_default() -> None:
    assigner = build_experiment_assigner(RecsSettings())

    assert isinstance(assigner, FliptExperimentAssigner)


def test_factory_builds_no_assigner_without_a_flipt_url_flipt_unreachable_serves_default() -> None:
    assert build_experiment_assigner(RecsSettings(RECS_FLIPT_URL="")) is None


def _flipt_assigner(handler: Any) -> FliptExperimentAssigner:
    import httpx

    return FliptExperimentAssigner(
        httpx.AsyncClient(
            base_url="http://flipt", transport=httpx.MockTransport(handler)
        ),
        salt="salt",
        timeout_s=1.0,
    )


async def _home_feed_service(assigner: Any) -> Any:
    serving = _fake()
    await serving.set(
        "recs:v1:user:u1",
        json.dumps([{"listing_id": "a", "visible": True}]),
    )
    return build_recommendation_service(
        RecsSettings(),
        source=MemoryCandidateSource(),
        serving_redis=serving,
        clients=RecommendationClients(
            qdrant=NO_QDRANT,
            featurestore=None,
            nearline=None,
            stock=MemoryStockLookup({"a": 5}),
            assigner=assigner,
        ),
    )


async def test_service_built_with_flipt_assigner_reports_a_stable_arm_a_visitor_always_lands_in_the_same_arm() -> (
    None
):
    import httpx

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"match": True, "variantKey": "treatment"})

    service = await _home_feed_service(_flipt_assigner(handler))
    query = RecommendationQuery(user_id="u1", placement_id="home_feed")

    first = await service.recommend(query)
    second = await service.recommend(query)

    assert first.arm != "none"
    assert first.arm == "recs-home-feed-ranker:treatment"
    assert second.arm == first.arm


async def test_service_built_without_an_assigner_reports_no_arm_a_disabled_flag_means_no_arm() -> (
    None
):
    service = await _home_feed_service(None)

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert result.arm == "none"


async def test_service_built_with_a_disabled_flag_reports_no_arm_a_disabled_flag_means_no_arm() -> (
    None
):
    import httpx

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"match": False})

    service = await _home_feed_service(_flipt_assigner(handler))

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert result.arm == "none"
    assert [i.listing_id for i in result.items] == ["a"]


async def test_service_built_with_unreachable_flipt_serves_default_ranker_unassigned_flipt_unreachable_serves_the_default_ranker_as_unassigned() -> (
    None
):
    import httpx

    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("down", request=request)

    service = await _home_feed_service(_flipt_assigner(handler))

    result = await service.recommend(
        RecommendationQuery(user_id="u1", placement_id="home_feed")
    )

    assert result.arm == "recs-home-feed-ranker:unassigned"
    assert [i.listing_id for i in result.items] == ["a"]
