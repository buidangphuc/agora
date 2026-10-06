"""Pure-numpy ranking helpers over the collected ALS factors.

The batch collects the (moderate-size) factor matrices to the driver and computes
the precomputed artifacts here — L2-normalized vectors for Qdrant (cosine ≈ dot
order) and the Top-N Redis lists. Kept NumPy-only (no Spark) so the ranking logic
is unit-testable on a laptop; at very large catalog scale this is the seam to
swap for `ALSModel.recommendForAllUsers` / a blocked matmul.
"""

from __future__ import annotations

import numpy as np


def l2_normalize(vec) -> list[float]:
    """L2-normalize a vector; a zero vector is returned unchanged (as floats)."""
    arr = np.asarray(vec, dtype=np.float64)
    norm = float(np.linalg.norm(arr))
    if norm == 0.0:
        return [float(x) for x in arr]
    return [float(x) for x in (arr / norm)]


def _stack(ids: list[str], vectors: list[list[float]]):
    return list(ids), np.asarray(vectors, dtype=np.float64)


def top_n_for_users(
    user_ids: list[str],
    user_vectors: list[list[float]],
    item_ids: list[str],
    item_vectors: list[list[float]],
    top_n: int,
) -> dict[str, list[tuple[str, float]]]:
    """Top-N items per user by dot product of user factor · item factors."""
    if not user_ids or not item_ids:
        return {}
    u_ids, users = _stack(user_ids, user_vectors)
    i_ids, items = _stack(item_ids, item_vectors)
    scores = users @ items.T  # (n_users, n_items)
    out: dict[str, list[tuple[str, float]]] = {}
    k = min(top_n, len(i_ids))
    for r, uid in enumerate(u_ids):
        row = scores[r]
        top = np.argsort(-row)[:k]
        out[uid] = [(i_ids[j], float(row[j])) for j in top]
    return out


def top_n_unseen(
    user_ids: list[str],
    user_vectors: list[list[float]],
    item_ids: list[str],
    item_vectors: list[list[float]],
    top_n: int,
    seen: dict[str, set[str]],
    only_users: set[str] | None = None,
) -> dict[str, list[tuple[str, float]]]:
    """Like top_n_for_users, but never ranks an item the user already interacted with.

    Used by the offline evaluation, where a seen item can never be the held-out target.
    ``only_users`` restricts scoring to the users being evaluated.
    """
    if not user_ids or not item_ids:
        return {}
    i_ids, items = _stack(item_ids, item_vectors)
    col = {iid: j for j, iid in enumerate(i_ids)}
    out: dict[str, list[tuple[str, float]]] = {}
    for uid, uvec in zip(user_ids, user_vectors, strict=False):
        if only_users is not None and uid not in only_users:
            continue
        row = items @ np.asarray(uvec, dtype=float)
        for iid in seen.get(uid, ()):
            if iid in col:
                row[col[iid]] = -np.inf
        order = np.argsort(-row)
        out[uid] = [(i_ids[j], float(row[j])) for j in order[:top_n] if np.isfinite(row[j])]
    return out


def similar_items(
    item_ids: list[str],
    item_vectors: list[list[float]],
    top_n: int,
) -> dict[str, list[tuple[str, float]]]:
    """Top-N nearest items per item by cosine similarity (self excluded)."""
    if not item_ids:
        return {}
    i_ids, items = _stack(item_ids, item_vectors)
    norms = np.linalg.norm(items, axis=1, keepdims=True)
    norms[norms == 0.0] = 1.0
    unit = items / norms
    sims = unit @ unit.T
    out: dict[str, list[tuple[str, float]]] = {}
    k = min(top_n, len(i_ids) - 1) if len(i_ids) > 1 else 0
    for r, iid in enumerate(i_ids):
        row = sims[r].copy()
        row[r] = -np.inf  # exclude self
        if k <= 0:
            out[iid] = []
            continue
        top = np.argsort(-row)[:k]
        out[iid] = [(i_ids[j], float(row[j])) for j in top]
    return out


def popularity_ranking(item_weight_pairs: list[tuple[str, float]], top_n: int) -> list[tuple[str, float]]:
    """Global Top-N by summed interaction weight — the cold-start floor list."""
    agg: dict[str, float] = {}
    for listing_id, weight in item_weight_pairs:
        agg[listing_id] = agg.get(listing_id, 0.0) + float(weight)
    ranked = sorted(agg.items(), key=lambda kv: -kv[1])
    return ranked[:top_n]
