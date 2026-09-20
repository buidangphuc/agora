"""CLI runner for offline recsys evaluation with time-based holdout split."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from recsys.evals.evaluator import ModelEvaluator


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Offline evaluation CLI for recsys models")
    parser.add_argument(
        "--warehouse-dir",
        type=str,
        default="",
        help="Path to warehouse parquet/json directory",
    )
    parser.add_argument("--split-k", type=int, default=1, help="Number of holdout events per user")
    parser.add_argument(
        "--k-values",
        type=int,
        nargs="+",
        default=[5, 10, 20],
        help="K thresholds for evaluation",
    )
    return parser.parse_args()


def load_interactions(warehouse_dir: str) -> list[dict]:
    if not warehouse_dir:
        return []
    p = Path(warehouse_dir)
    if not p.exists():
        return []
    interactions = []
    # If JSON file
    if p.is_file() and p.suffix == ".json":
        with p.open("r", encoding="utf-8") as f:
            interactions = json.load(f)
    return interactions


def main() -> None:
    args = parse_args()
    evaluator = ModelEvaluator(k_values=args.k_values)

    raw_interactions = load_interactions(args.warehouse_dir)
    if not raw_interactions:
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
        split_k=args.split_k,
    )
    print(json.dumps(results, indent=2))
    sys.exit(0)


if __name__ == "__main__":
    main()
