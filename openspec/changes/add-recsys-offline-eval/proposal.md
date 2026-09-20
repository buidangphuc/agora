## Why

Recommendation systems require rigorous, reproducible offline evaluation before deploying model artifacts to production. Currently, ALS models in `platform-recsys` are trained and written to Redis/Qdrant without metric gates. Without an offline evaluation harness, it is impossible to:
1. Measure ranking quality objectively (NDCG@K, Recall@K, Precision@K, MAP, MRR).
2. Measure catalog coverage and diversity to prevent popularity collapse.
3. Compare champion vs. challenger models (e.g., ALS baseline vs. Two-Tower vs. GBDT ranker).
4. Enforce automated promotion gates in CI / scheduled training jobs.

## What Changes

- **platform-recsys** (`recsys/evals/`):
  - Add offline evaluation metrics suite:
    - `ndcg_at_k(actual, predicted, k)`: Normalized Discounted Cumulative Gain.
    - `recall_at_k(actual, predicted, k)`: Proportion of relevant items retrieved in top-k.
    - `precision_at_k(actual, predicted, k)`: Precision of top-k recommendations.
    - `mean_average_precision(actual_dict, predicted_dict, k)`: MAP across all evaluated users.
    - `mrr_at_k(actual, predicted, k)`: Mean Reciprocal Rank of first relevant item.
    - `catalog_coverage(all_catalog_items, recommended_items_set)`: Percentage of catalog surfaced.
    - `hit_rate_at_k(actual, predicted, k)`: Binary hit rate at k.
  - Add `recsys/evals/evaluator.py`: Comprehensive `ModelEvaluator` evaluating predictions against ground truth test sets.
  - Add `recsys/evals/__main__.py` CLI and `make eval` target in `Makefile`.
  - Add full unit test suite `tests/test_evals.py` verifying mathematical correctness of all ranking metrics.

## Non-goals

- No online A/B testing in this change (handled in Phase 2/3).
- No external heavy dependencies — metrics are implemented in clean, vector-optimized NumPy / pure Python.
