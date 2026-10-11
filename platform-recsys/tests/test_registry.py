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


def test_force_promotes_a_candidate_the_gate_would_reject() -> None:
    """PROMOTION_FORCE path: an equal-scoring retrain is rejected unless forced."""
    registry = ModelRegistry()
    for v in ("als_v1", "als_v2", "als_v3"):
        registry.register_model(
            ModelMetadata(
                model_name="als",
                model_version=v,
                model_type="als",
                metrics={"ndcg@10": 0.5, "coverage@10": 0.8},
            )
        )
    assert registry.evaluate_and_promote("als_v1")[0] is True
    assert registry.evaluate_and_promote("als_v2", min_relative_improvement=0.01)[0] is False
    promoted, reason = registry.evaluate_and_promote("als_v3", min_relative_improvement=0.01, force=True)
    assert promoted is True
    assert "by force" in reason
    assert registry.get_champion_version() == "als_v3"


def test_champion_from_another_eval_protocol_is_not_compared() -> None:
    """Leaky pre-v1 metrics must not block (or be beaten by) honest v1 metrics."""
    registry = ModelRegistry()
    old = ModelMetadata(
        model_name="als",
        model_version="als_old",
        model_type="als",
        metrics={"ndcg@10": 0.75, "coverage@10": 0.8},
    )
    new = ModelMetadata(
        model_name="als",
        model_version="als_new",
        model_type="als",
        metrics={"ndcg@10": 0.05, "coverage@10": 0.3, "eval_protocol": "leave-last-new-item-v1"},
    )
    registry.register_model(old)
    registry.register_model(new)
    assert registry.evaluate_and_promote("als_old")[0] is True
    promoted, reason = registry.evaluate_and_promote("als_new", min_relative_improvement=0.01)
    assert promoted is True
    assert "not comparable" in reason
    assert registry.get_champion_version() == "als_new"
    assert registry.get_model("als_old").status == "archived"


def _model(version: str, metrics: dict) -> ModelMetadata:
    return ModelMetadata(model_name="als", model_version=version, model_type="als", metrics=metrics)


def test_evaluate_changes_nothing_until_the_decision_is_applied():
    registry = ModelRegistry()
    registry.register_model(_model("als_v1", {"ndcg@10": 0.30, "coverage@10": 0.9}))
    registry.evaluate_and_promote("als_v1")
    registry.register_model(_model("als_v2", {"ndcg@10": 0.40, "coverage@10": 0.9}))

    decision = registry.evaluate("als_v2")

    assert decision.promoted is True
    assert registry.get_champion_version() == "als_v1"
    assert registry.get_model("als_v2").status == "candidate"

    registry.reject(decision, reason="publish failed: RuntimeError")

    assert registry.get_champion_version() == "als_v1"
    assert registry.get_model("als_v1").status == "champion"
    assert registry.get_model("als_v2").status == "rejected"
    assert registry.get_model("als_v2").metrics["gate_reason"] == "publish failed: RuntimeError"

    registry.register_model(_model("als_v3", {"ndcg@10": 0.50, "coverage@10": 0.9}))
    registry.apply_promotion(registry.evaluate("als_v3"))
    assert registry.get_champion_version() == "als_v3"
    assert registry.get_model("als_v1").status == "archived"
