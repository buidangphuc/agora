"""Leakage-free offline evaluation split (per-user leave-last-new-item-out).

For each user with at least two distinct listings, the held-out target is the listing
the user discovered most recently (the latest *first* interaction). Training for that
user keeps only events strictly before that discovery. The evaluation model therefore
never sees the target pair or anything the user did afterwards, and the target is a
genuinely new item for the user rather than a repeat of something already in their
history. Predictions are scored with the user's training items excluded.

Users with a single listing contribute training data only. Rows without a user key
("anonymous") are never held out.
"""

from __future__ import annotations

from collections.abc import Iterable
from dataclasses import dataclass
from typing import Any

# Stamped into the metrics: the gate never compares numbers produced under different
# protocols (the earlier protocol trained on the holdout and inflated every metric).
EVAL_PROTOCOL = "leave-last-new-item-v1"


@dataclass(frozen=True)
class Holdout:
    actual: dict[str, list[str]]  # user → [held-out listing]
    cutoffs: dict[str, float]  # user → keep only that user's events before this ts
    train_events: int
    test_events: int
    cutoff_timestamp: float | None  # latest training timestamp across users


def leave_last_new_item_out(interactions: Iterable[dict[str, Any]]) -> Holdout:
    """Split {user_id, listing_id, timestamp} rows; see the module docstring."""
    first_seen: dict[str, dict[str, float]] = {}
    rows: list[tuple[str, str, float]] = []
    for ev in interactions:
        user = str(ev.get("user_id") or "")
        item = str(ev.get("listing_id") or "")
        if not user or not item or user == "anonymous":
            continue
        ts = float(ev.get("timestamp") or 0.0)
        rows.append((user, item, ts))
        items = first_seen.setdefault(user, {})
        if item not in items or ts < items[item]:
            items[item] = ts

    actual: dict[str, list[str]] = {}
    cutoffs: dict[str, float] = {}
    for user, items in first_seen.items():
        if len(items) < 2:
            continue
        # Latest discovery; ties broken by listing id so the split is deterministic.
        target, discovered = max(items.items(), key=lambda kv: (kv[1], kv[0]))
        if all(ts == discovered for ts in items.values()):
            continue  # everything discovered at once: no "before" to train on
        actual[user] = [target]
        cutoffs[user] = discovered

    train_ts = [ts for user, _, ts in rows if user not in cutoffs or ts < cutoffs[user]]
    return Holdout(
        actual=actual,
        cutoffs=cutoffs,
        train_events=len(train_ts),
        test_events=len(actual),
        cutoff_timestamp=max(train_ts) if train_ts else None,
    )
