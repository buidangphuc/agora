"""Unit tests for ModelRegistry and automated promotion gate."""

from __future__ import annotations

from recsys.registry.metadata import ModelMetadata
from recsys.registry.registry import ModelRegistry


def test_model_metadata_serialization() -> None:
    meta = ModelMetadata(
        model_name="als",
        model_version="als_v1",
        model_type="als",
        metrics={"ndcg@10": 0.42, "coverage@10": 0.60},
        parameters={"rank": 32, "iterations": 10},
    )
    json_str = meta.to_json()
    loaded = ModelMetadata.from_json(json_str)
    assert loaded.model_version == "als_v1"
    assert loaded.metrics["ndcg@10"] == 0.42
    assert loaded.parameters["rank"] == 32


def test_registry_initial_promotion() -> None:
    registry = ModelRegistry()
    m1 = ModelMetadata(
        model_name="als",
        model_version="als_v1",
        model_type="als",
        metrics={"ndcg@10": 0.40, "coverage@10": 0.50},
    )
    registry.register_model(m1)
    promoted, msg = registry.evaluate_and_promote("als_v1")
    assert promoted is True
    assert registry.get_champion_version() == "als_v1"
    assert registry.get_model("als_v1").status == "champion"


def test_registry_challenger_promotion_gate() -> None:
    registry = ModelRegistry()
    m1 = ModelMetadata(
        model_name="als",
        model_version="als_v1",
        model_type="als",
        metrics={"ndcg@10": 0.40, "coverage@10": 0.50},
    )
    registry.register_model(m1)
    registry.evaluate_and_promote("als_v1")

    # Superior candidate
    m2 = ModelMetadata(
        model_name="two_tower",
        model_version="tt_v1",
        model_type="two_tower",
        metrics={"ndcg@10": 0.45, "coverage@10": 0.55},
    )
    registry.register_model(m2)
    promoted, msg = registry.evaluate_and_promote("tt_v1", min_relative_improvement=0.05)
    assert promoted is True
    assert registry.get_champion_version() == "tt_v1"
    assert registry.get_model("tt_v1").status == "champion"
    assert registry.get_model("als_v1").status == "archived"

    # Inferior candidate
    m3 = ModelMetadata(
        model_name="als",
        model_version="als_v2_bad",
        model_type="als",
        metrics={"ndcg@10": 0.35, "coverage@10": 0.50},
    )
    registry.register_model(m3)
    promoted, msg = registry.evaluate_and_promote("als_v2_bad", min_relative_improvement=0.0)
    assert promoted is False
    assert registry.get_champion_version() == "tt_v1"  # still tt_v1
    assert registry.get_model("als_v2_bad").status == "rejected"
