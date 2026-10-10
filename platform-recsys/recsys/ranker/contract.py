"""The GBDT ranker's feature contract: one explicit list, in the serving order.

``RANKING_FEATURES`` are featurestore registry names written ``<view>.<feature>``. The trainer builds its
input vector in exactly this order from the featurestore offline snapshots, and the artifact it publishes
carries the list, so a reader can refuse a model whose list is not the one it serves (team-ai's
``recommend/features.py::RANKING_FEATURES``). ``tests/test_ranker_contract.py`` fails when this tuple
drifts from ``platform-featurestore/registry/features.yaml`` or from team-ai's tuple.
"""

from __future__ import annotations

MODEL_FORMAT = "agora-gbdt/1"
OBJECTIVE = "lambdarank"
EVAL_PROTOCOL = "temporal-lists-v1"

# The seven item_popularity@v1 features in registry order, then the attribute view's price.
RANKING_FEATURES: tuple[str, ...] = (
    "item_popularity.views_7d",
    "item_popularity.clicks_7d",
    "item_popularity.add_to_cart_7d",
    "item_popularity.favorites_current",
    "item_popularity.review_count",
    "item_popularity.avg_rating",
    "item_popularity.ctr_7d",
    "item_attributes.price",
)

# Registry version of each view the features come from (the online keys are fs:<view>:v<version>:<id>).
FEATURE_VIEWS: dict[str, int] = {"item_popularity": 1, "item_attributes": 1}

# The slot serving replaces with the nearline position-debiased CTR when usable; the trainer mirrors it.
CTR_FEATURE = "item_popularity.ctr_7d"

# A missing or null value is this (the serving defaults of team-ai's recommend/features.py).
DEFAULT_VALUE = 0.0

# Incumbent fixed weights (team-ai GBDTRankerAdapter) for the two slots the contract can fill: the baseline.
BASELINE_POPULARITY_WEIGHT = 0.15
BASELINE_CTR_WEIGHT = 0.10
POPULARITY_SATURATION = 500.0

# Impression-level debiasing as the nearline layer does it: clicks weighted by position ** this.
IPS_GAMMA = 0.5


def split_name(name: str) -> tuple[str, str]:
    view, _, feature = name.partition(".")
    return view, feature


def feature_index(name: str) -> int:
    return RANKING_FEATURES.index(name)
