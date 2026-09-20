"""CLI runner for offline recsys evaluation."""

from __future__ import annotations

import json
import sys

from recsys.evals.evaluator import ModelEvaluator


def main() -> None:
    evaluator = ModelEvaluator(k_values=[5, 10, 20])
    # Smoke evaluation against dummy test data
    actual = {
        "u1": ["item1", "item2"],
        "u2": ["item3", "item4"],
    }
    predicted = {
        "u1": ["item1", "item5", "item2"],
        "u2": ["item4", "item6", "item3"],
    }
    catalog = ["item1", "item2", "item3", "item4", "item5", "item6", "item7"]

    results = evaluator.evaluate(actual, predicted, all_catalog_items=catalog)
    print(json.dumps(results, indent=2))
    sys.exit(0)


if __name__ == "__main__":
    main()
