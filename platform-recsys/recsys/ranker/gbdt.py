"""A small numpy LambdaMART: histogram-binned regression trees fitted to LambdaRank gradients.

Why not LightGBM/XGBoost: the model must be scored by a service that takes no new dependency (team-ai), so it
is exported as plain JSON (``agora-gbdt/1``, see ``to_artifact``) and the trainer needs nothing beyond numpy.
The data is one window of impressions with a handful of features, so a few hundred nodes are enough.

Training is deterministic (no sampling). A tree node ``i`` is internal when ``feature[i] >= 0``: go to
``left[i]`` when ``x[feature[i]] <= threshold[i]`` else ``right[i]``; a leaf has ``feature[i] == -1`` and
the score ``value[i]``. ``score(x) = base_score + learning_rate * sum(leaf values)``.
"""

from __future__ import annotations

import math
from collections.abc import Sequence
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any

import numpy as np

from recsys.ranker import contract

N_BINS = 32
_EPS = 1e-12


@dataclass
class Tree:
    feature: list[int]
    threshold: list[float]
    left: list[int]
    right: list[int]
    value: list[float]

    def to_dict(self) -> dict[str, list]:
        return {
            "feature": self.feature,
            "threshold": self.threshold,
            "left": self.left,
            "right": self.right,
            "value": self.value,
        }


@dataclass
class Model:
    trees: list[Tree]
    learning_rate: float
    base_score: float = 0.0

    def predict(self, x: np.ndarray) -> np.ndarray:
        """Scores of the rows of ``x`` (raw feature values)."""
        out = np.full(len(x), self.base_score, dtype=float)
        for tree in self.trees:
            out += self.learning_rate * _leaf_values(tree, x)
        return out


def _leaf_values(tree: Tree, x: np.ndarray) -> np.ndarray:
    feature, threshold = np.array(tree.feature), np.array(tree.threshold)
    left, right, value = np.array(tree.left), np.array(tree.right), np.array(tree.value)
    node = np.zeros(len(x), dtype=int)
    while True:
        f = feature[node]
        internal = f >= 0
        if not internal.any():
            return value[node]
        go_left = x[np.arange(len(x)), np.where(internal, f, 0)] <= threshold[node]
        node = np.where(internal, np.where(go_left, left[node], right[node]), node)


def score_vector(artifact: dict[str, Any], x: Sequence[float]) -> float:
    """Score one feature vector from a published artifact in pure Python (the reader's algorithm)."""
    total = 0.0
    for tree in artifact["trees"]:
        i = 0
        while tree["feature"][i] >= 0:
            i = tree["left"][i] if x[tree["feature"][i]] <= tree["threshold"][i] else tree["right"][i]
        total += tree["value"][i]
    return float(artifact["base_score"] + artifact["learning_rate"] * total)


# ── binning ──────────────────────────────────────────────────────────────────
def make_edges(x: np.ndarray, n_bins: int = N_BINS) -> list[np.ndarray]:
    """Per feature, the split candidates: interior quantiles of its values (none for a constant feature)."""
    qs = np.linspace(0.0, 1.0, n_bins + 1)[1:-1]
    return [np.unique(np.quantile(x[:, j], qs)) if len(x) else np.array([]) for j in range(x.shape[1])]


def bin_matrix(x: np.ndarray, edges: list[np.ndarray]) -> np.ndarray:
    """Bin index per value: ``x <= edges[b]`` exactly when the bin index is <= b."""
    out = np.zeros(x.shape, dtype=np.int32)
    for j, e in enumerate(edges):
        out[:, j] = np.searchsorted(e, x[:, j], side="left")
    return out


# ── lambdas ──────────────────────────────────────────────────────────────────
@dataclass
class Lists:
    """Impression lists padded to the longest: ``row_of[l, s]`` is the row at slot s of list l (-1 pads)."""

    row_of: np.ndarray
    labels: np.ndarray  # float, 0 in padding
    mask: np.ndarray  # bool
    idcg_inv: np.ndarray  # 1 / IDCG per list (0 for a list without a positive)

    @classmethod
    def build(cls, groups: Sequence[Sequence[int]], labels: np.ndarray, k: int = 10) -> Lists:
        m = max((len(g) for g in groups), default=1)
        row_of = np.full((len(groups), m), -1, dtype=np.int64)
        for i, g in enumerate(groups):
            row_of[i, : len(g)] = g
        mask = row_of >= 0
        lab = np.where(mask, labels[np.where(mask, row_of, 0)], 0.0)
        gains = np.sort(np.where(mask, 2.0**lab - 1.0, 0.0), axis=1)[:, ::-1]
        disc = 1.0 / np.log2(np.arange(2, m + 2))
        idcg = (gains[:, :k] * disc[:k]).sum(axis=1)
        return cls(row_of, lab, mask, np.where(idcg > 0, 1.0 / np.maximum(idcg, _EPS), 0.0))


def lambdarank_gradients(scores: np.ndarray, lists: Lists, chunk: int = 512) -> tuple[np.ndarray, np.ndarray]:
    """Ascent gradient and curvature per row: for each pair (i more relevant than j) in a list,
    ``rho = 1 / (1 + exp(s_i - s_j))`` and ``|delta NDCG|`` weight the push of i up and j down."""
    grad = np.zeros(len(scores))
    hess = np.zeros(len(scores))
    for start in range(0, len(lists.row_of), chunk):
        sl = slice(start, start + chunk)
        rows, mask, lab, inv = lists.row_of[sl], lists.mask[sl], lists.labels[sl], lists.idcg_inv[sl]
        s = np.where(mask, scores[np.where(mask, rows, 0)], -1e30)
        order = np.argsort(-s, axis=1, kind="stable")
        rank = np.empty_like(order)
        np.put_along_axis(rank, order, np.arange(order.shape[1])[None, :].repeat(len(order), 0), axis=1)
        disc = 1.0 / np.log2(rank + 2.0)
        gain = np.where(mask, 2.0**lab - 1.0, 0.0)
        delta = np.abs((gain[:, :, None] - gain[:, None, :]) * (disc[:, :, None] - disc[:, None, :]))
        delta *= inv[:, None, None]
        pair = (lab[:, :, None] > lab[:, None, :]) & mask[:, :, None] & mask[:, None, :]
        diff = np.clip(s[:, :, None] - s[:, None, :], -30.0, 30.0)
        rho = 1.0 / (1.0 + np.exp(diff))
        lam = np.where(pair, rho * delta, 0.0)
        cur = np.where(pair, rho * (1.0 - rho) * delta, 0.0)
        g = lam.sum(axis=2) - lam.sum(axis=1)  # i is pushed up by pairs it wins, down by those it loses
        h = cur.sum(axis=2) + cur.sum(axis=1)
        flat = rows[mask]
        np.add.at(grad, flat, g[mask])
        np.add.at(hess, flat, h[mask])
    return grad, hess


# ── trees ────────────────────────────────────────────────────────────────────
def _fit_tree(
    xb: np.ndarray,
    edges: list[np.ndarray],
    grad: np.ndarray,
    hess: np.ndarray,
    max_depth: int,
    min_leaf: int,
    l2: float,
) -> Tree:
    feature: list[int] = []
    threshold: list[float] = []
    left: list[int] = []
    right: list[int] = []
    value: list[float] = []

    def leaf_value(rows: np.ndarray) -> float:
        return float(grad[rows].sum() / (hess[rows].sum() + l2))

    def new_node() -> int:
        for arr, fill in ((feature, -1), (threshold, 0.0), (left, -1), (right, -1), (value, 0.0)):
            arr.append(fill)
        return len(feature) - 1

    def best_split(rows: np.ndarray) -> tuple[float, int, int] | None:
        g_tot, h_tot = grad[rows].sum(), hess[rows].sum()
        parent = g_tot * g_tot / (h_tot + l2)
        best: tuple[float, int, int] | None = None
        for j, e in enumerate(edges):
            nb = len(e) + 1
            if nb < 2:
                continue
            col = xb[rows, j]
            gh = np.cumsum(np.bincount(col, weights=grad[rows], minlength=nb))[:-1]
            hh = np.cumsum(np.bincount(col, weights=hess[rows], minlength=nb))[:-1]
            ch = np.cumsum(np.bincount(col, minlength=nb))[:-1]
            ok = (ch >= min_leaf) & (len(rows) - ch >= min_leaf)
            if not ok.any():
                continue
            gain = gh * gh / (hh + l2) + (g_tot - gh) ** 2 / (h_tot - hh + l2) - parent
            gain = np.where(ok, gain, -np.inf)
            b = int(np.argmax(gain))
            if gain[b] > 1e-12 and (best is None or gain[b] > best[0]):
                best = (float(gain[b]), j, b)
        return best

    def grow(rows: np.ndarray, depth: int, node: int) -> None:
        split = best_split(rows) if depth < max_depth and len(rows) >= 2 * min_leaf else None
        if split is None:
            value[node] = leaf_value(rows)
            return
        _, j, b = split
        mask = xb[rows, j] <= b
        feature[node], threshold[node] = j, float(edges[j][b])
        left[node], right[node] = new_node(), new_node()
        grow(rows[mask], depth + 1, left[node])
        grow(rows[~mask], depth + 1, right[node])

    root = new_node()
    grow(np.arange(len(xb)), 0, root)
    return Tree(feature, threshold, left, right, value)


def fit(
    x: np.ndarray,
    lists: Lists,
    *,
    trees: int = 40,
    max_depth: int = 3,
    learning_rate: float = 0.1,
    min_leaf: int = 10,
    l2: float = 1.0,
) -> Model:
    """Boost ``trees`` LambdaRank trees over the rows of ``x`` (lists index into its rows)."""
    edges = make_edges(x)
    xb = bin_matrix(x, edges)
    scores = np.zeros(len(x))
    model = Model([], learning_rate)
    for _ in range(trees):
        grad, hess = lambdarank_gradients(scores, lists)
        if not np.any(grad):
            break  # nothing left to learn (e.g. no list mixes labels)
        tree = _fit_tree(xb, edges, grad, hess, max_depth, min_leaf, l2)
        model.trees.append(tree)
        scores += learning_rate * _leaf_values(tree, x)
    return model


# ── metric ───────────────────────────────────────────────────────────────────
def ndcg_at_k(labels_in_rank_order: Sequence[float], k: int = 10) -> float | None:
    """Graded NDCG@k (gain 2**label - 1) of one list in the order a ranker put it; None without a positive."""
    gains = [2.0 ** float(v) - 1.0 for v in labels_in_rank_order]
    ideal = sorted(gains, reverse=True)

    def dcg(g: Sequence[float]) -> float:
        return sum(v / math.log2(i + 2) for i, v in enumerate(g[:k]))

    best = dcg(ideal)
    return None if best <= 0 else dcg(gains) / best


def mean_ndcg(
    groups: Sequence[Sequence[int]], labels: np.ndarray, scores: np.ndarray, tiebreak: np.ndarray, k: int = 10
) -> tuple[float, int]:
    """(mean NDCG@k over the lists with a positive, how many such lists). Ties break by ``tiebreak``."""
    values = []
    for g in groups:
        idx = np.array(g)
        order = np.lexsort((tiebreak[idx], -scores[idx]))
        v = ndcg_at_k(labels[idx][order], k)
        if v is not None:
            values.append(v)
    return (float(np.mean(values)) if values else 0.0), len(values)


# ── artifact ─────────────────────────────────────────────────────────────────
def to_artifact(
    model: Model,
    *,
    model_version: str,
    generation: str,
    metrics: dict[str, Any],
    trained_at: datetime | None = None,
) -> dict[str, Any]:
    """The published ``agora-gbdt/1`` document (see the change's design D7)."""
    when = trained_at or datetime.now(timezone.utc)
    return {
        "format": contract.MODEL_FORMAT,
        "objective": contract.OBJECTIVE,
        "model_version": model_version,
        "generation": generation,
        "features": list(contract.RANKING_FEATURES),
        "feature_views": dict(contract.FEATURE_VIEWS),
        "defaults": {name: contract.DEFAULT_VALUE for name in contract.RANKING_FEATURES},
        "ctr_feature": contract.CTR_FEATURE,
        "base_score": model.base_score,
        "learning_rate": model.learning_rate,
        "trees": [t.to_dict() for t in model.trees],
        "metrics": metrics,
        "trained_at": when.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
