"""Two-tower vectors are published and retired as part of a generation (no Spark)."""

from __future__ import annotations

from recsys.config import Settings
from recsys.load import qdrant as qdrant_load
from recsys.publish import publish_generation
from recsys.two_tower.pipeline import train_and_index_two_tower
from tests.fakes import FakeQdrantClient, FakeRedis

DIM = 4
SETTINGS = Settings(als_rank=DIM, two_tower_dim=8, top_n=2)


def _publish(version, redis, qdrant, tt=None):
    return publish_generation(
        SETTINGS,
        version,
        {"u1": [("a", 0.9)]},
        {"a": [("b", 0.8)]},
        [("a", 1.0)],
        item_rows=[("a", [1.0] * DIM), ("b", [0.5] * DIM)],
        user_rows=[("u1", [1.0] * DIM)],
        redis_client=redis,
        qdrant_client=qdrant,
        two_tower_vectors=tt,
    )


def _tt(n=2):
    return {f"item-{i}": [0.0] * 7 + [1.0] for i in range(n)}


def test_loader_writes_a_collection_named_for_the_generation():
    client = FakeQdrantClient()
    count = qdrant_load.load_two_tower_vectors(SETTINGS, "g1", _tt(3), client=client)

    assert count == 3
    assert set(client.collections) == {"item_two_tower_vectors__g1"}  # no plain collection
    points = client.collections["item_two_tower_vectors__g1"].values()
    assert {p["payload"]["model_version"] for p in points} == {"g1"}
    assert client.dims["item_two_tower_vectors__g1"] == 8


def test_publish_writes_the_two_tower_collection_before_the_switch_and_reports_the_count():
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    out = _publish("g1", redis, qdrant, tt=_tt(2))

    assert out["two_tower_items"] == 2
    assert "item_two_tower_vectors__g1" in qdrant.collections
    assert redis.store["recs:v1:serving"] == "g1"


def test_publish_without_the_stage_writes_no_two_tower_collection():
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    out = _publish("g1", redis, qdrant)

    assert out["two_tower_items"] is None
    assert not [c for c in qdrant.collections if "two_tower" in c]


def test_earlier_generations_are_pruned_with_the_als_ones():
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    for version in ("g1", "g2", "g3"):
        _publish(version, redis, qdrant, tt=_tt())

    tower = sorted(c for c in qdrant.collections if c.startswith("item_two_tower_vectors"))
    assert tower == ["item_two_tower_vectors__g2", "item_two_tower_vectors__g3"]  # serving + previous


def test_the_old_plain_collection_is_retired():
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    qdrant.create_collection("item_two_tower_vectors", None)
    _publish("g1", redis, qdrant, tt=_tt())
    assert "item_two_tower_vectors" not in qdrant.collections


def test_cold_start_item_receives_two_tower_vector_without_interactions():
    catalog = [{"listing_id": "cold_item_never_clicked", "category_id": "electronics", "price": 49.9}]
    _, vectors = train_and_index_two_tower(catalog, embedding_dim=16)
    vec = vectors["cold_item_never_clicked"]
    assert len(vec) == 16
    assert abs(sum(v * v for v in vec) ** 0.5 - 1.0) < 1e-4


def test_disabled_stage_is_the_default():
    settings = Settings()
    assert settings.enable_two_tower is False
