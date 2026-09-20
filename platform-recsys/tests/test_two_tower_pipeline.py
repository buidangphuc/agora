"""Execution-proof tests for Two-Tower batch pipeline wiring (ADR-0011 / P3-T3)."""

from __future__ import annotations

from unittest.mock import MagicMock

from recsys.config import Settings
from recsys.load.qdrant import load_two_tower_vectors
from recsys.two_tower.model import TwoTowerModel
from recsys.two_tower.pipeline import train_and_index_two_tower


class FakeQdrantClient:
    def __init__(self):
        self.collections = set()
        self.upserted_points: dict[str, list] = {}
        self.deleted_filters: dict[str, list] = {}

    def get_collections(self):
        return type("Resp", (), {"collections": [type("C", (), {"name": c})() for c in self.collections]})()

    def create_collection(self, collection_name, vectors_config):
        self.collections.add(collection_name)

    def upsert(self, collection_name, points):
        self.upserted_points.setdefault(collection_name, []).extend(points)

    def delete(self, collection_name, points_selector):
        self.deleted_filters.setdefault(collection_name, []).append(points_selector)


def test_two_tower_loader_indexes_and_stamps_model_version():
    client = FakeQdrantClient()
    settings = Settings(
        enable_two_tower=True,
        qdrant_two_tower_collection="item_two_tower_vectors",
        two_tower_dim=16,
    )
    model_version = "recs-2026-09-20-001"
    item_vectors = {
        "item_cold_1": [0.1] * 16,
        "item_active_2": [0.2] * 16,
    }

    count = load_two_tower_vectors(
        settings=settings,
        model_version=model_version,
        item_vectors=item_vectors,
        client=client,
    )

    assert count == 2
    assert "item_two_tower_vectors" in client.collections
    points = client.upserted_points["item_two_tower_vectors"]
    assert len(points) == 2
    # Verify every point carries the run's model_version
    assert all(p.payload["model_version"] == model_version for p in points)
    # Verify prune_stale was called
    assert len(client.deleted_filters["item_two_tower_vectors"]) >= 1


def test_cold_start_item_receives_two_tower_vector_without_interactions():
    catalog = [
        {"listing_id": "cold_item_never_clicked", "category_id": "electronics", "price": 49.9},
    ]

    model, vectors = train_and_index_two_tower(catalog, embedding_dim=16)

    assert "cold_item_never_clicked" in vectors
    assert len(vectors["cold_item_never_clicked"]) == 16
    # Vector is normalized
    norm = sum(v * v for v in vectors["cold_item_never_clicked"]) ** 0.5
    assert abs(norm - 1.0) < 1e-4


def test_disabled_stage_leaves_summary_unchanged():
    settings_disabled = Settings(enable_two_tower=False)
    assert settings_disabled.enable_two_tower is False
    assert settings_disabled.qdrant_two_tower_collection == "item_two_tower_vectors"
