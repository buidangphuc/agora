"""Structural gate: reject a degenerate candidate before the metric gate sees it.

Holdout metrics cannot see a model that recommends the same few items to everyone, covers
almost no users, or carries NaN factors. These checks run on the candidate's own artifacts
(the precomputed Top-N lists and the factor matrices), in this order, and the first failure
is the reason. Pure numpy/stdlib so it unit-tests without Spark.
"""

from __future__ import annotations

import random
from collections.abc import Iterable, Sequence
from itertools import combinations

import numpy as np

from .config import Settings

MAX_OVERLAP_PAIRS = 500
_SAMPLE_SEED = 0


def mean_list_overlap(
    user_recs: dict[str, list[tuple[str, float]]], max_pairs: int = MAX_OVERLAP_PAIRS
) -> float:
    """Mean Jaccard overlap of users' top-N item sets over (a deterministic sample of) user pairs.

    Fewer than two users with a list gives 0.0: there is no pair to compare.
    """
    users = sorted(u for u, recs in user_recs.items() if recs)
    n = len(users)
    if n < 2:
        return 0.0
    sets = {u: {lid for lid, _ in user_recs[u]} for u in users}
    total_pairs = n * (n - 1) // 2
    if total_pairs <= max_pairs:
        pairs: Iterable[tuple[str, str]] = combinations(users, 2)
    else:
        rng = random.Random(_SAMPLE_SEED)
        chosen: set[tuple[int, int]] = set()
        while len(chosen) < max_pairs:
            i, j = rng.sample(range(n), 2)
            chosen.add((min(i, j), max(i, j)))
        pairs = ((users[i], users[j]) for i, j in sorted(chosen))
    acc = 0.0
    count = 0
    for a, b in pairs:
        union = len(sets[a] | sets[b])
        acc += (len(sets[a] & sets[b]) / union) if union else 0.0
        count += 1
    return acc / count if count else 0.0


def structural_check(
    user_recs: dict[str, list[tuple[str, float]]],
    item_recs: dict[str, list[tuple[str, float]]],
    factors: Sequence,
    dataset_users: int,
    dataset_items: int,
    settings: Settings,
) -> tuple[bool, str]:
    """Return (ok, reason). ``factors`` is the sequence of factor matrices (users, items).

    ``item_recs`` is accepted for the contract of the check and for future item-side rules;
    the catalogue coverage is measured on the users' top-N lists.
    """
    # Non-finite factors first: every later ratio is meaningless on a poisoned model.
    for idx, matrix in enumerate(factors):
        arr = np.asarray(matrix, dtype=np.float64)
        if arr.size and not np.isfinite(arr).all():
            return False, f"structural gate: factor matrix {idx} contains NaN or infinite values"

    users_with_list = sum(1 for recs in user_recs.values() if recs)
    user_coverage = users_with_list / dataset_users if dataset_users else 0.0
    if user_coverage < settings.gate_min_user_coverage:
        return False, (
            f"structural gate: user coverage {user_coverage:.4f} is below "
            f"GATE_MIN_USER_COVERAGE {settings.gate_min_user_coverage}"
        )

    recommended = {lid for recs in user_recs.values() for lid, _ in recs}
    item_coverage = len(recommended) / dataset_items if dataset_items else 0.0
    if item_coverage < settings.gate_min_item_coverage:
        return False, (
            f"structural gate: item coverage {item_coverage:.4f} is below "
            f"GATE_MIN_ITEM_COVERAGE {settings.gate_min_item_coverage}"
        )

    overlap = mean_list_overlap(user_recs)
    if overlap > settings.gate_max_list_overlap:
        return False, (
            f"structural gate: list overlap {overlap:.4f} is above "
            f"GATE_MAX_LIST_OVERLAP {settings.gate_max_list_overlap}"
        )
    return True, "structural gate passed"
