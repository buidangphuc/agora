"""Ranking and evaluation metrics for offline recommendation model assessment."""

from __future__ import annotations

import math
from collections.abc import Sequence


def precision_at_k(actual: Sequence[str] | set[str], predicted: Sequence[str], k: int = 10) -> float:
    """Precision@K: Proportion of recommended items in top-K that are relevant."""
    if k <= 0 or not predicted:
        return 0.0
    actual_set = set(actual)
    if not actual_set:
        return 0.0
    pred_k = predicted[:k]
    hits = sum(1 for item in pred_k if item in actual_set)
    return hits / float(k)


def recall_at_k(actual: Sequence[str] | set[str], predicted: Sequence[str], k: int = 10) -> float:
    """Recall@K: Proportion of relevant items that are captured in top-K."""
    if k <= 0 or not predicted:
        return 0.0
    actual_set = set(actual)
    if not actual_set:
        return 0.0
    pred_k = predicted[:k]
    hits = sum(1 for item in pred_k if item in actual_set)
    return hits / float(len(actual_set))


def hit_rate_at_k(actual: Sequence[str] | set[str], predicted: Sequence[str], k: int = 10) -> float:
    """HitRate@K: Binary indicator (1.0 or 0.0) whether at least one relevant item is in top-K."""
    if k <= 0 or not predicted:
        return 0.0
    actual_set = set(actual)
    if not actual_set:
        return 0.0
    pred_k = predicted[:k]
    for item in pred_k:
        if item in actual_set:
            return 1.0
    return 0.0


def mrr_at_k(actual: Sequence[str] | set[str], predicted: Sequence[str], k: int = 10) -> float:
    """Mean Reciprocal Rank @ K: 1/rank of the first relevant item in top-K, or 0.0."""
    if k <= 0 or not predicted:
        return 0.0
    actual_set = set(actual)
    if not actual_set:
        return 0.0
    pred_k = predicted[:k]
    for idx, item in enumerate(pred_k, start=1):
        if item in actual_set:
            return 1.0 / float(idx)
    return 0.0


def dcg_at_k(actual_set: set[str], predicted: Sequence[str], k: int = 10) -> float:
    """Discounted Cumulative Gain @ K with binary relevance."""
    pred_k = predicted[:k]
    dcg = 0.0
    for idx, item in enumerate(pred_k, start=1):
        if item in actual_set:
            dcg += 1.0 / math.log2(idx + 1)
    return dcg


def ndcg_at_k(actual: Sequence[str] | set[str], predicted: Sequence[str], k: int = 10) -> float:
    """Normalized Discounted Cumulative Gain @ K."""
    if k <= 0 or not predicted:
        return 0.0
    actual_set = set(actual)
    if not actual_set:
        return 0.0
    actual_dcg = dcg_at_k(actual_set, predicted, k)
    if actual_dcg == 0.0:
        return 0.0
    # Ideal DCG: all relevant items ranked first
    ideal_hits = min(k, len(actual_set))
    idcg = sum(1.0 / math.log2(i + 1) for i in range(1, ideal_hits + 1))
    if idcg == 0.0:
        return 0.0
    return actual_dcg / idcg


def average_precision_at_k(actual: Sequence[str] | set[str], predicted: Sequence[str], k: int = 10) -> float:
    """Average Precision @ K for a single user."""
    if k <= 0 or not predicted:
        return 0.0
    actual_set = set(actual)
    if not actual_set:
        return 0.0
    pred_k = predicted[:k]
    hits = 0
    sum_precisions = 0.0
    for idx, item in enumerate(pred_k, start=1):
        if item in actual_set:
            hits += 1
            sum_precisions += float(hits) / float(idx)
    denominator = min(len(actual_set), k)
    if denominator == 0:
        return 0.0
    return sum_precisions / float(denominator)


def mean_average_precision(
    actual_dict: dict[str, Sequence[str] | set[str]],
    predicted_dict: dict[str, Sequence[str]],
    k: int = 10,
) -> float:
    """Mean Average Precision across all evaluated users."""
    if not actual_dict:
        return 0.0
    aps = []
    for user_id, actual_items in actual_dict.items():
        pred_items = predicted_dict.get(user_id, [])
        aps.append(average_precision_at_k(actual_items, pred_items, k))
    return sum(aps) / float(len(aps)) if aps else 0.0


def catalog_coverage(
    all_catalog_items: Sequence[str] | set[str],
    predicted_dict: dict[str, Sequence[str]],
    k: int = 10,
) -> float:
    """Catalog Coverage: Proportion of distinct catalog items recommended in top-K across all users."""
    total_catalog = set(all_catalog_items)
    if not total_catalog or not predicted_dict:
        return 0.0
    recommended_set: set[str] = set()
    for preds in predicted_dict.values():
        recommended_set.update(preds[:k])
    # Intersect with valid catalog items
    valid_recs = recommended_set.intersection(total_catalog)
    return len(valid_recs) / float(len(total_catalog))
