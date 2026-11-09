import json

import qdrant_client
from fakeredis import aioredis
from qdrant_client import models

from app.modules.business.recommendation.adapters.qdrant import QdrantCandidateSource
from app.modules.business.recommendation.adapters.redis_cache import RedisServingCache
from app.modules.business.recommendation.contract.keys import ServingKeys
from app.modules.business.recommendation.contract.qdrant import point_id
from app.modules.business.recommendation.schemas import Candidate


async def _client() -> qdrant_client.AsyncQdrantClient:
    client = qdrant_client.AsyncQdrantClient(location=":memory:")
    await client.create_collection(
        collection_name="item_als_vectors",
        vectors_config=models.VectorParams(size=4, distance=models.Distance.COSINE),
    )
    payloads = {
        "a": {"listing_id": "a"},
        "full": {
            "listing_id": "full",
            "seller_id": "S1",
            "category_id": "C1",
            "visible": True,
        },
        "hidden": {
            "listing_id": "hidden",
            "seller_id": "S2",
            "category_id": "C2",
            "visible": False,
        },
        "bare": {"listing_id": "bare"},
    }
    vectors = {
        "a": [1.0, 0.0, 0.0, 0.0],
        "full": [0.9, 0.1, 0.0, 0.0],
        "hidden": [0.8, 0.2, 0.0, 0.0],
        "bare": [0.7, 0.3, 0.0, 0.0],
    }
    await client.upsert(
        collection_name="item_als_vectors",
        points=[
            models.PointStruct(
                id=point_id(lid), vector=vectors[lid], payload=payloads[lid]
            )
            for lid in payloads
        ],
    )
    return client


def _source(client) -> QdrantCandidateSource:
    return QdrantCandidateSource(
        client,
        collection="item_als_vectors",
        vector_dim=4,
        distance="Cosine",
        timeout_s=1.0,
    )


async def test_adapters_read_the_attribute_keys_from_qdrant():
    got = {
        c.listing_id: c
        for c in await _source(await _client()).similar("a", top_k=10, generation=None)
    }
    assert (got["full"].seller_id, got["full"].category_id, got["full"].visible) == (
        "S1",
        "C1",
        True,
    )
    assert got["hidden"].visible is False
    assert (got["bare"].seller_id, got["bare"].category_id, got["bare"].visible) == (
        "",
        "",
        False,
    )


async def test_qdrant_categories_are_read_from_the_item_payload():
    source = _source(await _client())
    got = await source.categories(["full", "hidden", "bare", "nope"], generation=None)
    assert got == {"full": "C1", "hidden": "C2"}


async def test_adapters_read_the_attribute_keys_from_redis():
    redis = aioredis.FakeRedis(decode_responses=True)
    await redis.set(
        "recs:v1:popular",
        json.dumps(
            [
                {
                    "listing_id": "full",
                    "score": 0.9,
                    "seller_id": "S1",
                    "category_id": "C1",
                    "visible": True,
                },
                {"listing_id": "hidden", "score": 0.8, "visible": False},
                {"listing_id": "bare", "score": 0.7},
                "plain",
            ]
        ),
    )
    await redis.set(
        "recs:v1:user:u1",
        json.dumps(
            [
                {
                    "listing_id": "u",
                    "seller_id": "S9",
                    "category_id": "C9",
                    "visible": True,
                }
            ]
        ),
    )
    cache = RedisServingCache(redis, ServingKeys("recs", "v1"))
    got = {c.listing_id: c for c in await cache.popular_candidates(None)}
    assert isinstance(got["full"], Candidate)
    assert (got["full"].seller_id, got["full"].category_id, got["full"].visible) == (
        "S1",
        "C1",
        True,
    )
    assert got["hidden"].visible is False
    for lid in ("bare", "plain"):
        assert (got[lid].seller_id, got[lid].category_id, got[lid].visible) == (
            "",
            "",
            False,
        )
    user = (await cache.user_candidates("u1", None))[0]
    assert (user.seller_id, user.category_id, user.visible) == ("S9", "C9", True)


def test_a_candidate_built_in_process_is_visible_by_default():
    assert Candidate("a", 1.0).visible is True
