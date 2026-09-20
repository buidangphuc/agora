"""ModelEvaluator for offline evaluation of candidate and champion models with temporal split ownership."""

from __future__ import annotations

import logging
from collections.abc import Sequence
from typing import Any

from recsys.evals.metrics import (
    catalog_coverage,
    hit_rate_at_k,
    mean_average_precision,
    mrr_at_k,
    ndcg_at_k,
    precision_at_k,
    recall_at_k,
)
from recsys.evals.split import user_leave_k_out_temporal_split

logger = logging.getLogger(__name__)


class ModelEvaluator:
    """Evaluates top-N recommendation predictions against ground-truth user holdout sets."""

    def __init__(self, k_values: list[int] | None = None) -> None:
        self.k_values = k_values or [5, 10, 20]

    def evaluate(
        self,
        actual_dict: dict[str, Sequence[str] | set[str]] | None = None,
        predicted_dict: dict[str, Sequence[str] | Sequence[tuple[str, float]]] | None = None,
        all_catalog_items: Sequence[str] | set[str] | None = None,
        *,
        raw_interactions: list[dict[str, Any]] | None = None,
        split_k: int = 1,
        split_strategy: str = "external",
        cutoff_timestamp: float | None = None,
        train_events_count: int = 0,
        test_events_count: int = 0,
    ) -> dict[str, Any]:
        """Compute full suite of ranking metrics over test users, recording temporal split metadata."""
        predicted_dict = predicted_dict or {}

        # If raw_interactions are provided, derive temporal split and holdout ground truth
        if raw_interactions is not None:
            train_set, holdout_gt = user_leave_k_out_temporal_split(
                raw_interactions,
                k=split_k,
            )
            actual_dict = holdout_gt
            split_strategy = "temporal"
            train_events_count = len(train_set)
            test_events_count = sum(len(items) for items in holdout_gt.values())
            if train_set:
                cutoff_timestamp = max(float(x.get("timestamp", 0.0)) for x in train_set)
        else:
            actual_dict = actual_dict or {}

        if not actual_dict:
            return {
                "user_count": 0,
                "split_strategy": split_strategy,
                "cutoff_timestamp": cutoff_timestamp,
                "train_events": train_events_count,
                "test_events": test_events_count,
            }

        # Normalize predicted items if list of (item_id, score) tuples is given
        clean_preds: dict[str, list[str]] = {}
        for uid, recs in predicted_dict.items():
            if recs and isinstance(recs[0], tuple):
                clean_preds[uid] = [item[0] for item in recs]
            else:
                clean_preds[uid] = list(recs)  # type: ignore

        results: dict[str, Any] = {
            "user_count": len(actual_dict),
            "split_strategy": split_strategy,
            "cutoff_timestamp": cutoff_timestamp,
            "train_events": train_events_count,
            "test_events": test_events_count,
        }

        for k in self.k_values:
            ndcg_list = []
            recall_list = []
            precision_list = []
            hit_list = []
            mrr_list = []

            for uid, actual in actual_dict.items():
                preds = clean_preds.get(uid, [])
                ndcg_list.append(ndcg_at_k(actual, preds, k=k))
                recall_list.append(recall_at_k(actual, preds, k=k))
                precision_list.append(precision_at_k(actual, preds, k=k))
                hit_list.append(hit_rate_at_k(actual, preds, k=k))
                mrr_list.append(mrr_at_k(actual, preds, k=k))

            n_users = float(len(actual_dict))
            results[f"ndcg@{k}"] = sum(ndcg_list) / n_users
            results[f"recall@{k}"] = sum(recall_list) / n_users
            results[f"precision@{k}"] = sum(precision_list) / n_users
            results[f"hit_rate@{k}"] = sum(hit_list) / n_users
            results[f"mrr@{k}"] = sum(mrr_list) / n_users
            results[f"map@{k}"] = mean_average_precision(actual_dict, clean_preds, k=k)

            if all_catalog_items:
                results[f"coverage@{k}"] = catalog_coverage(all_catalog_items, clean_preds, k=k)

        return results

    @staticmethod
    def compare_models(
        baseline_metrics: dict[str, float],
        candidate_metrics: dict[str, float],
        *,
        primary_metric: str = "ndcg@10",
        min_relative_improvement: float = 0.0,
        min_coverage_ratio: float = 0.8,
    ) -> tuple[bool, str]:
        """Check if candidate model qualifies to replace baseline model (promotion gate)."""
        base_score = baseline_metrics.get(primary_metric, 0.0)
        cand_score = candidate_metrics.get(primary_metric, 0.0)

        if base_score > 0:
            rel_improvement = (cand_score - base_score) / base_score
        else:
            rel_improvement = 1.0 if cand_score > 0 else 0.0

        if rel_improvement < min_relative_improvement:
            msg = (
                f"{primary_metric} relative improvement {rel_improvement:.2%} "
                f"< threshold {min_relative_improvement:.2%}"
            )
            return False, msg

        base_cov = baseline_metrics.get("coverage@10")
        cand_cov = candidate_metrics.get("coverage@10")
        if base_cov is not None and cand_cov is not None and base_cov > 0:
            cov_ratio = cand_cov / base_cov
            if cov_ratio < min_coverage_ratio:
                msg = (
                    f"Catalog coverage ratio {cov_ratio:.2%} "
                    f"dropped below safety floor {min_coverage_ratio:.2%}"
                )
                return False, msg

        return True, f"Candidate passed promotion gate: {primary_metric} improved by {rel_improvement:.2%}"
