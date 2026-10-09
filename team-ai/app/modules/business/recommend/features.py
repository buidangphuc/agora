"""The item features serving reads from the feature store, by their registry names.

Single explicit list: ``ITEM_POPULARITY_FEATURES`` mirrors ``item_popularity@v1`` in
``platform-featurestore/registry/features.yaml`` (the online rows at
``fs:item_popularity:v<N>:<listing_id>``, ``N`` from ``fs:item_popularity:current``). Each
name maps to the default used when the row lacks it or holds a non-number. A default is
never an error: ``missing_item_features`` names what was defaulted so the service can
count it (``explain["feature_defaults"]``). ``tests/.../test_item_feature_contract.py``
fails when this list and the registry drift apart.
"""

from __future__ import annotations

import math
from typing import Any

ITEM_POPULARITY_VIEW = "item_popularity"

# name -> default when missing. Order and names follow the registry entry.
ITEM_POPULARITY_FEATURES: dict[str, float] = {
    "views_7d": 0.0,
    "clicks_7d": 0.0,
    "add_to_cart_7d": 0.0,
    "favorites_current": 0.0,
    "review_count": 0.0,
    "avg_rating": 0.0,  # NULL in the view for an item without reviews
    "ctr_7d": 0.0,
}

# Popularity is one derived input of the ranker: a weighted engagement count squashed to
# [0, 1] by log scale (saturates at POPULARITY_SATURATION weighted events).
POPULARITY_WEIGHTS = {
    "views_7d": 1.0,
    "clicks_7d": 2.0,
    "add_to_cart_7d": 5.0,
    "favorites_current": 3.0,
}
POPULARITY_SATURATION = 500.0


def item_feature(row: dict[str, Any], name: str) -> tuple[float, bool]:
    """``(value, missing)``: the registry feature ``name`` of ``row``, or its default."""
    raw = row.get(name)
    try:
        value = float(raw)  # type: ignore[arg-type]
    except (TypeError, ValueError):
        return ITEM_POPULARITY_FEATURES[name], True
    if math.isnan(value) or math.isinf(value):
        return ITEM_POPULARITY_FEATURES[name], True
    return value, False


def missing_item_features(row: dict[str, Any]) -> list[str]:
    """The registry features ``row`` does not carry (the old ad-hoc names do not count)."""
    return [n for n in ITEM_POPULARITY_FEATURES if item_feature(row, n)[1]]


def popularity(row: dict[str, Any]) -> float:
    weighted = sum(
        w * max(0.0, item_feature(row, n)[0]) for n, w in POPULARITY_WEIGHTS.items()
    )
    return min(1.0, math.log1p(weighted) / math.log1p(POPULARITY_SATURATION))
