"""Unit tests for offline evaluation metrics, temporal splitters, and ModelEvaluator."""

from __future__ import annotations

import pytest

from recsys.evals.evaluator import ModelEvaluator
from recsys.evals.metrics import (
    catalog_coverage,
    hit_rate_at_k,
    mean_average_precision,
    mrr_at_k,
    ndcg_at_k,
    precision_at_k,
    recall_at_k,
)
from recsys.evals.split import (
    temporal_train_test_split,
    user_leave_k_out_temporal_split,
)


def test_precision_at_k() -> None:
    actual = ["i1", "i2", "i3"]
    predicted = ["i1", "i4", "i2", "i5", "i6"]

    assert precision_at_k(actual, predicted, k=2) == 0.5  # 1 hit out of 2
    assert precision_at_k(actual, predicted, k=3) == 2.0 / 3.0  # 2 hits out of 3
    assert precision_at_k(actual, [], k=5) == 0.0
    assert precision_at_k([], predicted, k=5) == 0.0


def test_recall_at_k() -> None:
    actual = ["i1", "i2", "i3", "i4"]
    predicted = ["i1", "i2", "x1", "x2"]

    assert recall_at_k(actual, predicted, k=2) == 0.5  # 2 out of 4
    assert recall_at_k(actual, predicted, k=4) == 0.5  # still 2 out of 4
    assert recall_at_k(actual, [], k=5) == 0.0


def test_hit_rate_at_k() -> None:
    actual = ["i1", "i2"]
    predicted_hit = ["x1", "x2", "i1"]
    predicted_miss = ["x1", "x2", "x3"]

    assert hit_rate_at_k(actual, predicted_hit, k=3) == 1.0
    assert hit_rate_at_k(actual, predicted_hit, k=2) == 0.0
    assert hit_rate_at_k(actual, predicted_miss, k=3) == 0.0


def test_mrr_at_k() -> None:
    actual = ["i1", "i2"]
    assert mrr_at_k(actual, ["i1", "x2", "x3"], k=3) == 1.0  # rank 1 -> 1/1
    assert mrr_at_k(actual, ["x1", "i2", "x3"], k=3) == 0.5  # rank 2 -> 1/2
    assert mrr_at_k(actual, ["x1", "x2", "i1"], k=3) == 1.0 / 3.0  # rank 3 -> 1/3
    assert mrr_at_k(actual, ["x1", "x2", "x3"], k=3) == 0.0


def test_ndcg_at_k() -> None:
    actual = ["i1", "i2"]
    assert pytest.approx(ndcg_at_k(actual, ["i1", "i2", "x1"], k=3)) == 1.0
    reordered_ndcg = ndcg_at_k(actual, ["x1", "i1", "i2"], k=3)
    assert 0.0 < reordered_ndcg < 1.0
    assert ndcg_at_k(actual, ["x1", "x2", "x3"], k=3) == 0.0


def test_map_at_k() -> None:
    actual_dict = {
        "u1": ["i1", "i2"],
        "u2": ["i3"],
    }
    predicted_dict = {
        "u1": ["i1", "i2", "x1"],
        "u2": ["x1", "i3", "x2"],
    }
    assert pytest.approx(mean_average_precision(actual_dict, predicted_dict, k=3)) == 0.75


def test_catalog_coverage() -> None:
    catalog = ["i1", "i2", "i3", "i4", "i5"]
    predicted_dict = {
        "u1": ["i1", "i2"],
        "u2": ["i2", "i3"],
    }
    assert pytest.approx(catalog_coverage(catalog, predicted_dict, k=2)) == 0.6


def test_model_evaluator() -> None:
    evaluator = ModelEvaluator(k_values=[5, 10])
    actual = {
        "u1": ["item1", "item2"],
        "u2": ["item3", "item4"],
    }
    predicted = {
        "u1": [("item1", 0.9), ("item2", 0.8), ("item5", 0.5)],
        "u2": [("item3", 0.7), ("item6", 0.6), ("item4", 0.4)],
    }
    catalog = ["item1", "item2", "item3", "item4", "item5", "item6", "item7"]

    results = evaluator.evaluate(actual, predicted, all_catalog_items=catalog)
    assert results["user_count"] == 2
    assert "ndcg@10" in results
    assert "recall@10" in results
    assert "precision@10" in results
    assert "coverage@10" in results
    assert results["ndcg@10"] > 0.0


def test_promotion_gate() -> None:
    baseline = {"ndcg@10": 0.40, "coverage@10": 0.50}
    superior_candidate = {"ndcg@10": 0.45, "coverage@10": 0.52}
    inferior_candidate = {"ndcg@10": 0.38, "coverage@10": 0.50}
    narrow_candidate = {"ndcg@10": 0.46, "coverage@10": 0.20}

    passed, msg = ModelEvaluator.compare_models(baseline, superior_candidate, min_relative_improvement=0.05)
    assert passed is True

    passed, msg = ModelEvaluator.compare_models(baseline, inferior_candidate, min_relative_improvement=0.0)
    assert passed is False

    passed, msg = ModelEvaluator.compare_models(
        baseline, narrow_candidate, min_relative_improvement=0.0, min_coverage_ratio=0.8
    )
    assert passed is False


def test_temporal_train_test_split() -> None:
    interactions = [
        {"user_id": "u1", "listing_id": "i1", "timestamp": 100},
        {"user_id": "u2", "listing_id": "i2", "timestamp": 200},
        {"user_id": "u1", "listing_id": "i3", "timestamp": 300},
        {"user_id": "u3", "listing_id": "i4", "timestamp": 400},
        {"user_id": "u2", "listing_id": "i5", "timestamp": 500},
    ]

    train, test = temporal_train_test_split(interactions, holdout_ratio=0.4)
    assert len(train) == 3
    assert len(test) == 2
    max_train_ts = max(x["timestamp"] for x in train)
    min_test_ts = min(x["timestamp"] for x in test)
    assert max_train_ts <= min_test_ts


def test_user_leave_k_out_temporal_split() -> None:
    interactions = [
        {"user_id": "u1", "listing_id": "i1", "timestamp": 100},
        {"user_id": "u1", "listing_id": "i2", "timestamp": 200},
        {"user_id": "u1", "listing_id": "i3", "timestamp": 300},
        {"user_id": "u2", "listing_id": "i4", "timestamp": 150},
    ]

    train, test_gt = user_leave_k_out_temporal_split(interactions, k=1)
    assert len(train) == 3
    assert "u1" in test_gt
    assert test_gt["u1"] == ["i3"]  # Latest item for u1
    assert "u2" not in test_gt  # u2 only had 1 item, kept in train
