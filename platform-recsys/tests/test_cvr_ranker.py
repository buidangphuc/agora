"""Unit tests for CVRModel and eGMVRanker."""

import pytest

from recsys.ranker.cvr import CVRModel, eGMVRanker
from recsys.ranker.features import CandidateFeatures, extract_candidate_features


def test_cvr_model_prediction_range():
    model = CVRModel()
    # High conversion signals: item_cvr=0.2, cart_to_order=0.8, user_cvr=0.3
    high_vec = [0.2, 0.8, 0.3, 0.5, 1.0, 0.8, 1.0]
    p_high = model.predict_p_cvr(high_vec)
    assert 0.0 < p_high < 1.0

    # Low conversion signals: 0 cvr, 0 cart_to_order, 0 user_cvr
    low_vec = [0.0, 0.0, 0.0, 0.5, 1.0, 0.1, 0.0]
    p_low = model.predict_p_cvr(low_vec)
    assert 0.0 < p_low < 1.0
    assert p_high > p_low, f"High CVR {p_high} should exceed Low CVR {p_low}"


def test_egmv_ranker_favors_high_commercial_value():
    ranker = eGMVRanker(beta=1.0, gamma=0.5)

    candidates = [
        {"listing_id": "item-cheap-high-cvr", "price": 50000.0, "score": 0.8},
        {"listing_id": "item-expensive-high-cvr", "price": 500000.0, "score": 0.8},
        {"listing_id": "item-zero-cvr", "price": 500000.0, "score": 0.8},
    ]

    item_features = {
        "item-cheap-high-cvr": {"conversion_rate": 0.15, "cart_to_order_ratio": 0.5, "price": 50000.0},
        "item-expensive-high-cvr": {"conversion_rate": 0.15, "cart_to_order_ratio": 0.5, "price": 500000.0},
        "item-zero-cvr": {"conversion_rate": 0.001, "cart_to_order_ratio": 0.01, "price": 500000.0},
    }

    ranked = ranker.rank_candidates(candidates, item_features_map=item_features)
    assert len(ranked) == 3

    # item-expensive-high-cvr has higher eGMV than item-cheap-high-cvr and item-zero-cvr
    top_item = ranked[0]["listing_id"]
    assert top_item == "item-expensive-high-cvr", f"Expected expensive high-cvr item on top, got {top_item}"
    
    # zero CVR item should rank last
    last_item = ranked[-1]["listing_id"]
    assert last_item == "item-zero-cvr", f"Expected zero CVR item last, got {last_item}"
