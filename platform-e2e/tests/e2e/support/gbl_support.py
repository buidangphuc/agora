"""Helpers for the trained-ranker serving scenario (recsys-gbdt-trainer, D7, area gbl).

The recsys job publishes the LambdaMART as JSON at `recs:v1:gen:<generation>:ranker`. The scenario writes
that key as the job would (a single tree on `item_attributes.price`), seeds the candidates' online feature
rows and reads the order through the gateway. Every key is restored in teardown by `rss.Keys`.
"""

from __future__ import annotations

import json
import uuid

from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World

# `RANKING_FEATURES` of the artifact (platform-recsys contract, team-ai recommend/features.py).
FEATURES = [
    "item_popularity.views_7d",
    "item_popularity.clicks_7d",
    "item_popularity.add_to_cart_7d",
    "item_popularity.favorites_current",
    "item_popularity.review_count",
    "item_popularity.avg_rating",
    "item_popularity.ctr_7d",
    "item_attributes.price",
]
PRICE_INDEX = FEATURES.index("item_attributes.price")
PRICE_THRESHOLD = 100.0


def artifact(features: list[str] | None = None) -> str:
    """One tree: price above the threshold scores 1.0 more, so the dearer item ranks first."""
    return json.dumps(
        {
            "format": "agora-gbdt/1",
            "objective": "lambdarank",
            "model_version": "gbdt-e2e",
            "features": FEATURES if features is None else features,
            "feature_views": {"item_popularity": 1, "item_attributes": 1},
            "ctr_feature": "item_popularity.ctr_7d",
            "base_score": 0.0,
            "learning_rate": 0.1,
            "trees": [
                {
                    "feature": [PRICE_INDEX, -1, -1],
                    "threshold": [PRICE_THRESHOLD, 0.0, 0.0],
                    "left": [1, -1, -1],
                    "right": [2, -1, -1],
                    "value": [0.0, 0.0, 1.0],
                }
            ],
        }
    )


def ranker_key(serving: rss.Keys) -> str:
    pointer = serving.get(rss.SERVING_KEY)
    assert pointer, "no serving generation pointer: the ranker artifact is generation-scoped"
    return f"recs:v1:gen:{pointer}:ranker"


def seed(world: World) -> tuple[str, str, rss.Keys]:
    """Home-feed list [cheap (higher similarity), dear] and their online rows; no artifact yet."""
    serving = rss.keys(world, rss.SERVING_DB)
    features = rss.keys(world, rss.FEATURES_DB)
    run = uuid.uuid4().hex[:8]
    first, second = f"e2e-gbl-{run}-cheap", f"e2e-gbl-{run}-dear"
    for view, rows in (
        ("item_popularity", {first: {"views_7d": 1}, second: {"views_7d": 1}}),
        ("item_attributes", {first: {"price": 50}, second: {"price": 500}}),
    ):
        version = features.get(f"fs:{view}:current")
        if not version:
            version = "1"
            features.put(f"fs:{view}:current", version)
        for lid, row in rows.items():
            features.put(f"fs:{view}:v{version}:{lid}", json.dumps(row))
    serving.put(
        f"{rss.serving_prefix(serving)}:user:{rss.buyer_id(world)}",
        json.dumps([{"listing_id": first, "score": 0.9}, {"listing_id": second, "score": 0.8}]),
    )
    return first, second, serving
