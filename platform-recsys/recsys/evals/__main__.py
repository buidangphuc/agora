"""CLI runner for offline recsys evaluation with time-based holdout split."""

from __future__ import annotations

import json
import sys

from recsys.evals.evaluator import ModelEvaluator


def main() -> None:
    evaluator = ModelEvaluator(k_values=[5, 10, 20])

    # Sample raw interactions with timestamps demonstrating time-based holdout split
    raw_interactions = [
        {"user_id": "u1", "listing_id": "item1", "timestamp": 1000.0},
        {"user_id": "u1", "listing_id": "item2", "timestamp": 1050.0},
        {"user_id": "u1", "listing_id": "item3", "timestamp": 1200.0},  # Holdout for u1
        {"user_id": "u2", "listing_id": "item4", "timestamp": 1010.0},
        {"user_id": "u2", "listing_id": "item5", "timestamp": 1080.0},
        {"user_id": "u2", "listing_id": "item6", "timestamp": 1300.0},  # Holdout for u2
    ]

    # Predictions produced by a trained model on training data
    predicted = {
        "u1": ["item3", "item1", "item2"],
        "u2": ["item6", "item4", "item5"],
    }
    catalog = ["item1", "item2", "item3", "item4", "item5", "item6", "item7"]

    results = evaluator.evaluate(
        predicted_dict=predicted,
        raw_interactions=raw_interactions,
        all_catalog_items=catalog,
        split_k=1,
    )
    print(json.dumps(results, indent=2))
    sys.exit(0)


if __name__ == "__main__":
    main()
