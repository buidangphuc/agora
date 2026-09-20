"""Execution-proof tests for time-based holdout evaluation CLI and harness (P1-T2 / P3-T3)."""

from __future__ import annotations

import pytest

from recsys.evals.evaluator import ModelEvaluator
from recsys.evals.split import temporal_train_test_split


def test_evaluator_owns_temporal_split_and_reports_metadata():
    evaluator = ModelEvaluator(k_values=[5, 10])
    raw_interactions = [
        {"user_id": "u1", "listing_id": "item1", "timestamp": 100.0},
        {"user_id": "u1", "listing_id": "item2", "timestamp": 200.0},
        {"user_id": "u1", "listing_id": "item3", "timestamp": 300.0},  # Holdout test item
        {"user_id": "u2", "listing_id": "item4", "timestamp": 150.0},
        {"user_id": "u2", "listing_id": "item5", "timestamp": 250.0},  # Holdout test item
    ]
    predictions = {
        "u1": ["item3", "item1"],
        "u2": ["item5", "item4"],
    }

    report = evaluator.evaluate(
        raw_interactions=raw_interactions,
        predicted_dict=predictions,
        split_k=1,
    )

    assert report["split_strategy"] == "temporal"
    assert report["cutoff_timestamp"] == 200.0
    assert report["train_events"] == 3
    assert report["test_events"] == 2
    assert report["user_count"] == 2
    assert report["ndcg@10"] > 0.0


def test_temporal_split_zero_leakage():
    interactions = [
        {"user_id": "u1", "listing_id": "i1", "timestamp": 10.0},
        {"user_id": "u2", "listing_id": "i2", "timestamp": 20.0},
        {"user_id": "u3", "listing_id": "i3", "timestamp": 30.0},
        {"user_id": "u4", "listing_id": "i4", "timestamp": 40.0},
    ]

    train, test = temporal_train_test_split(interactions, holdout_ratio=0.5)
    assert len(train) == 2
    assert len(test) == 2

    max_train_ts = max(x["timestamp"] for x in train)
    min_test_ts = min(x["timestamp"] for x in test)
    assert max_train_ts < min_test_ts


def test_post_cutoff_only_signal_yields_zero_recall_on_training_model():
    evaluator = ModelEvaluator(k_values=[5])

    # User u1 ONLY interacted with item_secret after timestamp 500 (not in train)
    raw_interactions = [
        {"user_id": "u1", "listing_id": "item_train_1", "timestamp": 100.0},
        {"user_id": "u1", "listing_id": "item_train_2", "timestamp": 200.0},
        {"user_id": "u1", "listing_id": "item_post_cutoff_secret", "timestamp": 600.0},  # Holdout
    ]

    # Model trained strictly on train set knows only item_train_1 and item_train_2
    model_predictions = {
        "u1": ["item_train_1", "item_train_2", "popular_item"],
    }

    report = evaluator.evaluate(
        raw_interactions=raw_interactions,
        predicted_dict=model_predictions,
        split_k=1,
    )

    # Since item_post_cutoff_secret was never in train, recall is exactly 0.0
    assert report["recall@5"] == 0.0
    assert report["hit_rate@5"] == 0.0


def test_external_ground_truth_is_marked_external():
    evaluator = ModelEvaluator(k_values=[5])
    actual = {"u1": ["i1"]}
    predicted = {"u1": ["i1"]}

    report = evaluator.evaluate(actual_dict=actual, predicted_dict=predicted)
    assert report["split_strategy"] == "external"
