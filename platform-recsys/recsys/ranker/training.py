"""Training data of the GBDT ranker: governed impressions joined point in time to featurestore snapshots.

Rows come from the ``rank_training@v1`` dataset. For each row the features are those of the latest
``item_popularity@v1`` (and ``item_attributes@v1``) snapshot whose ``as_of`` is not after the row's impression
time, in the order of ``contract.RANKING_FEATURES``. The ``item_popularity.ctr_7d`` slot is then replaced, as
serving does with the nearline CTR, by an offline position-debiased CTR when the item has enough other
impressions in the TRAIN rows (never from held-out rows): ``ctr_source`` says which one each row got.

Pure numpy/pandas/pyarrow, no Spark. Python 3.10 compatible.
"""

from __future__ import annotations

import bisect
import hashlib
import math
import re
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any

import numpy as np
import pandas as pd

from recsys.config import ConfigError
from recsys.ranker import contract

_STAMP = re.compile(r"^as_of=(\d{8}T\d{6}Z)\.parquet$")
CTR_COL = contract.feature_index(contract.CTR_FEATURE)
DATASET_COLUMNS = ("user_key", "impression_id", "listing_id", "position", "label", "occurred_at")
FALLBACK, DEBIASED = "fallback", "debiased"


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


@dataclass(frozen=True)
class SnapshotFile:
    as_of: datetime  # naive UTC
    path: Path


class SnapshotSeries:
    """Every ``as_of=<stamp>.parquet`` of one view, oldest first, and the one in force at a time."""

    def __init__(self, directory: str, view: str, version: int, entity: str) -> None:
        self.directory, self.view, self.version, self.entity = directory, view, version, entity
        files = []
        for p in Path(directory).glob("as_of=*.parquet"):
            m = _STAMP.match(p.name)
            if m and p.is_file():
                files.append(SnapshotFile(datetime.strptime(m.group(1), "%Y%m%dT%H%M%SZ"), p))
        self.files = sorted(files, key=lambda f: f.as_of)
        self._as_of = [f.as_of for f in self.files]
        self._rows: dict[Path, dict[str, dict[str, Any]]] = {}
        self.used: dict[Path, int] = {}

    def __bool__(self) -> bool:
        return bool(self.files)

    def at(self, ts: datetime) -> SnapshotFile | None:
        """The latest snapshot with ``as_of <= ts``, or None when every snapshot is later."""
        i = bisect.bisect_right(self._as_of, ts)
        return self.files[i - 1] if i else None

    def rows(self, snap: SnapshotFile) -> dict[str, dict[str, Any]]:
        if snap.path not in self._rows:
            table = pd.read_parquet(snap.path)
            if self.entity not in table.columns:
                raise ConfigError(f"{snap.path} has no {self.entity} column (columns: {list(table.columns)})")
            self._rows[snap.path] = {
                str(r[self.entity]): r for r in table.to_dict("records") if r.get(self.entity)
            }
        return self._rows[snap.path]

    @property
    def lineage(self) -> dict[str, Any]:
        """The snapshots rows were actually joined to (name and SHA-256), oldest first."""
        used = sorted((p for p in self.used), key=lambda p: p.name)
        return {
            "view": self.view,
            "version": self.version,
            "snapshots": [{"snapshot": p.name, "sha256": _sha256(p), "rows": self.used[p]} for p in used],
        }


def _num(row: dict[str, Any] | None, name: str) -> float | None:
    if row is None:
        return None
    try:
        v = float(row.get(name))  # type: ignore[arg-type]
    except (TypeError, ValueError):
        return None
    return v if math.isfinite(v) else None


@dataclass
class Matrix:
    """The joined rows: ``frame`` (dataset columns) and ``x`` (features, contract order), same length."""

    frame: pd.DataFrame
    x: np.ndarray
    counts: dict[str, int] = field(default_factory=dict)


def read_dataset(path: str) -> pd.DataFrame:
    frame = pd.read_parquet(path)
    missing = [c for c in DATASET_COLUMNS if c not in frame.columns]
    if missing:
        raise ConfigError(f"ranking dataset {path} lacks columns {missing}")
    frame = frame.loc[:, list(DATASET_COLUMNS)].copy()
    frame["occurred_at"] = pd.to_datetime(frame["occurred_at"]).dt.tz_localize(None)
    frame["position"] = frame["position"].fillna(0).astype(int)
    frame["label"] = frame["label"].fillna(0).astype(int)
    return frame.sort_values(["occurred_at", "impression_id", "listing_id"], kind="stable").reset_index(
        drop=True
    )


def build_matrix(frame: pd.DataFrame, pop: SnapshotSeries, attrs: SnapshotSeries | None) -> Matrix:
    """Join every row to the snapshots in force at its impression time. A row with no ``item_popularity``
    snapshot at or before it is dropped (counted); a listing missing from a snapshot takes the defaults."""
    names = contract.RANKING_FEATURES
    keep: list[int] = []
    vectors: list[list[float]] = []
    counts = {
        "rows_in": len(frame),
        "rows_dropped_no_snapshot": 0,
        "listing_not_in_popularity": 0,
        "price_defaulted": 0,
    }
    for i, rec in enumerate(frame.itertuples(index=False)):
        snap = pop.at(rec.occurred_at.to_pydatetime())
        if snap is None:
            counts["rows_dropped_no_snapshot"] += 1
            continue
        pop.used[snap.path] = pop.used.get(snap.path, 0) + 1
        prow = pop.rows(snap).get(str(rec.listing_id))
        if prow is None:
            counts["listing_not_in_popularity"] += 1
        arow = None
        if attrs:
            asnap = attrs.at(rec.occurred_at.to_pydatetime())
            if asnap is not None:
                attrs.used[asnap.path] = attrs.used.get(asnap.path, 0) + 1
                arow = attrs.rows(asnap).get(str(rec.listing_id))
        vec = []
        for name in names:
            view, feature = contract.split_name(name)
            value = _num(prow if view == "item_popularity" else arow, feature)
            if value is None and name == "item_attributes.price":
                counts["price_defaulted"] += 1
            vec.append(contract.DEFAULT_VALUE if value is None else value)
        keep.append(i)
        vectors.append(vec)
    out = frame.iloc[keep].reset_index(drop=True)
    x = np.array(vectors, dtype=float).reshape(len(keep), len(names))
    counts["rows"] = len(out)
    return Matrix(out, x, counts)


# ── lists and the temporal split ─────────────────────────────────────────────
@dataclass
class Split:
    train: np.ndarray  # row indices
    test: np.ndarray
    cutoff: datetime | None
    train_last_at: datetime | None
    test_first_at: datetime | None
    train_lists: int
    test_lists: int


def limit_lists(frame: pd.DataFrame, max_list: int, max_lists: int) -> pd.DataFrame:
    """At most ``max_list`` items per impression (lowest positions) and ``max_lists`` impressions (latest)."""
    first = frame.groupby("impression_id")["occurred_at"].transform("min")
    ranked = frame.assign(_first=first).sort_values(
        ["_first", "impression_id", "position", "listing_id"], kind="stable"
    )
    ranked["_slot"] = ranked.groupby("impression_id").cumcount()
    ranked = ranked[ranked["_slot"] < max_list]
    ids = ranked.drop_duplicates("impression_id")[["impression_id", "_first"]].sort_values(
        "_first", kind="stable"
    )
    if len(ids) > max_lists:
        ranked = ranked[ranked["impression_id"].isin(set(ids["impression_id"].iloc[-max_lists:]))]
    return ranked.drop(columns=["_first", "_slot"]).sort_index()


def temporal_split(frame: pd.DataFrame, holdout_fraction: float) -> Split:
    """The latest ``holdout_fraction`` of impressions (by their first event time) are the holdout."""
    first = frame.groupby("impression_id")["occurred_at"].min().sort_values(kind="stable")
    n_test = max(1, int(round(len(first) * holdout_fraction))) if len(first) > 1 else 0
    test_ids = set(first.index[len(first) - n_test :]) if n_test else set()
    is_test = frame["impression_id"].isin(test_ids).to_numpy()
    train, test = np.flatnonzero(~is_test), np.flatnonzero(is_test)
    train_last = frame["occurred_at"].iloc[train].max() if len(train) else None
    test_first = frame["occurred_at"].iloc[test].min() if len(test) else None
    return Split(
        train,
        test,
        cutoff=None if test_first is None else test_first.to_pydatetime(),
        train_last_at=None if train_last is None else train_last.to_pydatetime(),
        test_first_at=None if test_first is None else test_first.to_pydatetime(),
        train_lists=len(first) - n_test,
        test_lists=n_test,
    )


def group_rows(frame: pd.DataFrame, rows: np.ndarray) -> list[list[int]]:
    """Row indices (into ``frame``) of each impression among ``rows``, in the shown order."""
    sub = frame.iloc[rows]
    groups: dict[str, list[int]] = {}
    for idx, imp in zip(rows, sub["impression_id"], strict=True):
        groups.setdefault(imp, []).append(int(idx))
    return list(groups.values())


# ── debiased CTR ─────────────────────────────────────────────────────────────
def ctr_stats(frame: pd.DataFrame, train: np.ndarray) -> dict[str, tuple[float, float]]:
    """Per listing, over the TRAIN rows: (position-weighted clicks, impressions)."""
    sub = frame.iloc[train]
    weight = np.maximum(sub["position"].to_numpy(), 1) ** contract.IPS_GAMMA
    clicked = (sub["label"].to_numpy() >= 1).astype(float)
    stats: dict[str, list[float]] = {}
    for lid, w, c in zip(sub["listing_id"], weight, clicked, strict=True):
        s = stats.setdefault(lid, [0.0, 0.0])
        s[0] += w * c
        s[1] += 1.0
    return {k: (v[0], v[1]) for k, v in stats.items()}


def apply_debiased_ctr(
    matrix: Matrix, train: np.ndarray, min_impressions: int
) -> tuple[np.ndarray, np.ndarray]:
    """``(x, ctr_source)``: ``x`` with the ctr slot replaced by the debiased CTR where the item has at least
    ``min_impressions`` other TRAIN impressions (a train row never sees its own outcome), else left as the
    snapshot's ctr_7d. ``ctr_source`` is ``debiased`` or ``fallback`` per row."""
    frame = matrix.frame
    stats = ctr_stats(frame, train)
    is_train = np.zeros(len(frame), dtype=bool)
    is_train[train] = True
    x = matrix.x.copy()
    source = np.full(len(frame), FALLBACK, dtype=object)
    weight = np.maximum(frame["position"].to_numpy(), 1) ** contract.IPS_GAMMA
    clicked = (frame["label"].to_numpy() >= 1).astype(float)
    for i, lid in enumerate(frame["listing_id"]):
        clicks, imprs = stats.get(lid, (0.0, 0.0))
        if is_train[i]:
            clicks -= weight[i] * clicked[i]
            imprs -= 1.0
        if imprs >= max(1, min_impressions):
            x[i, CTR_COL] = min(1.0, max(0.0, clicks / imprs))
            source[i] = DEBIASED
    return x, source


def baseline_scores(x: np.ndarray) -> np.ndarray:
    """The incumbent fixed weights on the slots the contract fills: 0.15 * popularity + 0.10 * ctr."""
    f = contract.RANKING_FEATURES
    col = {n: x[:, f.index(n)] for n in f}
    weighted = (
        col["item_popularity.views_7d"]
        + 2.0 * col["item_popularity.clicks_7d"]
        + 5.0 * col["item_popularity.add_to_cart_7d"]
        + 3.0 * col["item_popularity.favorites_current"]
    )
    popularity = np.minimum(
        1.0, np.log1p(np.maximum(weighted, 0.0)) / math.log1p(contract.POPULARITY_SATURATION)
    )
    ctr = np.clip(col[contract.CTR_FEATURE], 0.0, 1.0)
    return contract.BASELINE_POPULARITY_WEIGHT * popularity + contract.BASELINE_CTR_WEIGHT * ctr


def tiebreak(listing_ids: Any) -> np.ndarray:
    """A deterministic order unrelated to shown position, for ties."""
    return np.array([int(hashlib.md5(str(v).encode()).hexdigest()[:12], 16) for v in listing_ids])
