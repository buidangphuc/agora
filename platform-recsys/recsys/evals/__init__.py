"""Offline evaluation metrics, temporal splitters, and validation harness."""

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

__all__ = [
    "precision_at_k",
    "recall_at_k",
    "hit_rate_at_k",
    "mrr_at_k",
    "ndcg_at_k",
    "mean_average_precision",
    "catalog_coverage",
    "ModelEvaluator",
    "temporal_train_test_split",
    "user_leave_k_out_temporal_split",
]
