"""Time-based holdout splitting for offline evaluation (strict temporal ordering, no random split)."""

from __future__ import annotations

from typing import Any


def temporal_train_test_split(
    interactions: list[dict[str, Any]],
    timestamp_col: str = "timestamp",
    holdout_ratio: float = 0.2,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    """Splits interactions into train and test sets strictly based on global timestamp cutoff.
    
    Guarantees all training events strictly precede test events (zero temporal data leakage).
    """
    if not interactions:
        return [], []

    sorted_events = sorted(interactions, key=lambda x: float(x.get(timestamp_col, 0.0)))
    cutoff_idx = int(len(sorted_events) * (1.0 - max(0.01, min(0.99, holdout_ratio))))
    
    train_set = sorted_events[:cutoff_idx]
    test_set = sorted_events[cutoff_idx:]
    return train_set, test_set


def user_leave_k_out_temporal_split(
    interactions: list[dict[str, Any]],
    user_col: str = "user_id",
    item_col: str = "listing_id",
    timestamp_col: str = "timestamp",
    k: int = 1,
) -> tuple[list[dict[str, Any]], dict[str, list[str]]]:
    """Per-user temporal split: holds out the last K chronological interactions per user for testing."""
    user_events: dict[str, list[dict[str, Any]]] = {}
    for ev in interactions:
        uid = str(ev.get(user_col, ""))
        if uid:
            user_events.setdefault(uid, []).append(ev)

    train_set: list[dict[str, Any]] = []
    test_ground_truth: dict[str, list[str]] = {}

    for uid, events in user_events.items():
        sorted_evs = sorted(events, key=lambda x: float(x.get(timestamp_col, 0.0)))
        if len(sorted_evs) <= k:
            # Not enough interactions to hold out K without starving train
            train_set.extend(sorted_evs)
        else:
            train_set.extend(sorted_evs[:-k])
            test_ground_truth[uid] = [
                str(ev.get(item_col, ""))
                for ev in sorted_evs[-k:]
                if str(ev.get(item_col, ""))
            ]

    return train_set, test_ground_truth
