"""Qdrant backend ↔ platform-recsys producer contract (serve-trained-recs-locally)."""

from __future__ import annotations

import uuid
from types import SimpleNamespace

import pytest

from app.modules.business.recommend import backends
from app.modules.business.recommend.backends import QdrantRetrievalBackend, point_id

# uuid5(namespace, "listing-1") as platform-recsys computes it (recsys/load/qdrant.py).
PRODUCER_PIN = "25a4b2d5-6531-5357-90f2-06e92d1e1191"


def test_point_id_matches_the_producer_pin() -> None:
    producer_ns = uuid.UUID("6f7a1e2c-9b3d-4c5a-8e21-0d9f4a2b1c00")
    assert producer_ns == backends.POINT_ID_NAMESPACE
    assert point_id("listing-1") == PRODUCER_PIN


class _FakeClient:
    def __init__(self, points: list[SimpleNamespace]) -> None:
        self.points = points
        self.queries: list[dict] = []

    def query_points(self, **kwargs):
        self.queries.append(kwargs)
        return SimpleNamespace(points=self.points)


def _backend(client: _FakeClient) -> QdrantRetrievalBackend:
    b = QdrantRetrievalBackend(
        url="http://qdrant:6333",
        collection="item_als_vectors",
        vector_dim=64,
        distance="Cosine",
    )
    b._client = client
    return b


async def test_similar_queries_by_point_id_and_returns_listing_ids() -> None:
    pytest.importorskip("qdrant_client")
    hit = SimpleNamespace(
        id=point_id("lst-b"),
        score=0.9,
        payload={"listing_id": "lst-b", "model_version": "v1"},
    )
    client = _FakeClient([hit])
    got = await _backend(client).retrieve_similar("lst-a", top_k=5)

    query = client.queries[0]["query"]
    assert query.recommend.positive == [point_id("lst-a")]
    assert client.queries[0]["limit"] == 5
    assert [c.listing_id for c in got] == ["lst-b"]
    assert got[0].score == pytest.approx(0.9)


def test_hit_without_payload_falls_back_to_the_point_id() -> None:
    hit = SimpleNamespace(id="raw-id", score=0.1, payload=None)
    assert backends._hit_to_candidate(hit).listing_id == "raw-id"
