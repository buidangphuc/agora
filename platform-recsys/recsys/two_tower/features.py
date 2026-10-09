"""Tower inputs from the featurestore's offline snapshots (governed, versioned, point in time).

The towers read ``item_popularity@v1`` and ``user_activity@v2`` snapshots written by
``python -m featurestore materialize`` to ``<offline dir>/<view>/v<n>/as_of=<YYYYMMDDTHHMMSSZ>.parquet``
(the entity column is ``listing_id`` / ``user_key``). The job mounts that offline dir read-only, like the
dataset. The latest ``as_of`` in the directory is used, or an explicit file; with none the run refuses to
start (ConfigError, exit 2) rather than train on invented features.

What the views do not carry yet: item category and price, user preferred categories and average order
value. Those tower inputs stay 0 until the featurestore adds an item/user attribute view; nothing here
substitutes a constant for them.

The mappings below fix the scale of every feature (no catalogue-relative normalisation), so a feature
means the same thing in training and wherever the vectors are later queried.
"""

from __future__ import annotations

import hashlib
import math
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from recsys.config import ConfigError

# log1p(engagement) / log1p(POPULARITY_SCALE) reaches 1 at this weighted engagement count (7 days).
POPULARITY_SCALE = 500.0
# Same for a user's views + clicks + add-to-carts in 7 days.
ACTIVITY_SCALE = 200.0


@dataclass(frozen=True)
class Snapshot:
    """A resolved feature snapshot: where it is and what it was (recorded as run lineage)."""

    view: str
    version: int
    path: str
    sha256: str

    @property
    def lineage(self) -> dict:
        return {
            "view": self.view,
            "version": self.version,
            "snapshot": Path(self.path).name,
            "sha256": self.sha256,
        }


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def resolve_snapshot(directory: str, explicit: str, view: str, version: int, setting: str) -> Snapshot:
    """``explicit`` (a file), else the lexically latest ``as_of=*.parquet`` under ``directory``."""
    explicit = explicit.strip()
    if explicit:
        path = Path(explicit)
        if not path.is_file() or path.suffix != ".parquet":
            raise ConfigError(
                f"no feature snapshot at {setting}_PATH={explicit} (a .parquet file is required)"
            )
    else:
        candidates = sorted(p for p in Path(directory).glob("as_of=*.parquet") if p.is_file())
        if not candidates:
            raise ConfigError(
                f"no {view}@v{version} feature snapshot under {setting}_DIR={directory}: the two-tower "
                "stage trains on featurestore features only (run `python -m featurestore materialize`)"
            )
        path = candidates[-1]
    return Snapshot(view=view, version=version, path=str(path), sha256=_sha256(path))


def read_rows(snapshot: Snapshot, entity: str) -> dict[str, dict[str, Any]]:
    """The snapshot's rows keyed by entity id."""
    import pyarrow.parquet as pq  # noqa: PLC0415

    table = pq.read_table(snapshot.path)
    if entity not in table.column_names:
        raise ConfigError(f"{snapshot.path} has no {entity} column (columns: {table.column_names})")
    return {str(row[entity]): row for row in table.to_pylist() if row.get(entity)}


def _num(row: dict[str, Any], key: str) -> float:
    value = row.get(key)
    return float(value) if value is not None else 0.0


def item_features(row: dict[str, Any]) -> dict[str, float]:
    """ItemTower inputs from an ``item_popularity@v1`` row. ``category_id`` and ``price`` are not in
    the view and are left out (the tower reads them as 0)."""
    engagement = (
        _num(row, "views_7d")
        + 2.0 * _num(row, "clicks_7d")
        + 5.0 * _num(row, "add_to_cart_7d")
        + 3.0 * _num(row, "favorites_current")
    )
    return {
        "historical_ctr": min(1.0, max(0.0, _num(row, "ctr_7d"))),
        "popularity_score": min(1.0, math.log1p(engagement) / math.log1p(POPULARITY_SCALE)),
    }


def user_features(row: dict[str, Any]) -> dict[str, float]:
    """UserTower inputs from a ``user_activity@v2`` row. Preferred categories and average order value
    are not in the view and are left out (the tower reads them as 0)."""
    activity = _num(row, "views_7d") + _num(row, "clicks_7d") + _num(row, "add_to_cart_7d")
    return {
        "lifetime_purchases": _num(row, "paid_orders_30d"),
        "activity_score": min(1.0, math.log1p(activity) / math.log1p(ACTIVITY_SCALE)),
    }
