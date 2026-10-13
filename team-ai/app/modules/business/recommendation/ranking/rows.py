"""Reading numbers out of the feature store's untyped JSON rows."""

import math
from typing import Any

from app.modules.business.recommendation.contract.features import (
    ITEM_POPULARITY_FEATURES,
)

# Popularity is one derived input of the ranker: a weighted engagement count squashed to
# [0, 1] by log scale (saturates at POPULARITY_SATURATION weighted events).
POPULARITY_WEIGHTS = {
    "views_7d": 1.0,
    "clicks_7d": 2.0,
    "add_to_cart_7d": 5.0,
    "favorites_current": 3.0,
}
POPULARITY_SATURATION = 500.0


def number(raw: Any) -> float | None:
    """``raw`` as a finite float, or ``None`` when it is missing, not a number, NaN or inf."""
    try:
        value = float(raw)
    except (TypeError, ValueError):
        return None
    return value if math.isfinite(value) else None


def item_feature(row: dict[str, Any], name: str) -> tuple[float, bool]:
    """``(value, missing)``: the registry feature ``name`` of ``row``, or its default."""
    value = number(row.get(name))
    if value is None:
        return ITEM_POPULARITY_FEATURES[name], True
    return value, False


def missing_item_features(row: dict[str, Any]) -> list[str]:
    """The registry features ``row`` does not carry."""
    return [name for name in ITEM_POPULARITY_FEATURES if item_feature(row, name)[1]]


def popularity(row: dict[str, Any]) -> float:
    weighted = sum(
        weight * max(0.0, item_feature(row, name)[0])
        for name, weight in POPULARITY_WEIGHTS.items()
    )
    return min(1.0, math.log1p(weighted) / math.log1p(POPULARITY_SATURATION))
