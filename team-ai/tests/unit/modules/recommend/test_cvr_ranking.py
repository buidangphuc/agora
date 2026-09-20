"""Unit tests for eGMVRankerAdapter in team-ai."""

import pytest

from app.modules.business.recommend.ranking import (
    InMemoryFeatureStore,
    InMemoryNearlineStore,
    eGMVRankerAdapter,
)
from app.modules.business.recommend.schemas import Candidate, RecommendQuery


def test_egmv_ranker_online_scoring():
    ranker = eGMVRankerAdapter(beta=1.0, gamma=0.5)

    cands = [
        Candidate(listing_id="item-high-cvr", score=0.8, category_id="cat-1"),
        Candidate(listing_id="item-low-cvr", score=0.8, category_id="cat-1"),
    ]

    item_features = {
        "item-high-cvr": {
            "price": 200000.0,
            "category_match": 1.0,
            "popularity_score": 80.0,
            "historical_ctr": 0.05,
            "conversion_rate": 0.10,
            "cart_to_order_ratio": 0.60,
        },
        "item-low-cvr": {
            "price": 200000.0,
            "category_match": 1.0,
            "popularity_score": 80.0,
            "historical_ctr": 0.05,
            "conversion_rate": 0.001,
            "cart_to_order_ratio": 0.01,
        },
    }

    query = RecommendQuery(seed_listing_id="", category_id="cat-1")
    ranked = ranker.rank_candidates(cands, query, item_features_map=item_features)

    assert len(ranked) == 2
    assert ranked[0].listing_id == "item-high-cvr"
    assert ranked[1].listing_id == "item-low-cvr"
    assert ranked[0].score > ranked[1].score
