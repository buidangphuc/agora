"""Unit tests for GBDTRanker demonstrating NDCG@10 improvement over raw cosine and feature store enrichment."""

from __future__ import annotations

from recsys.evals.metrics import ndcg_at_k
from recsys.ranker.features import compute_ips_weight, extract_candidate_features
from recsys.ranker.model import GBDTRanker


def test_feature_extraction_and_ips() -> None:
    cand = {
        "listing_id": "item-1",
        "score": 0.85,
        "category_id": "Electronics",
        "popularity_score": 50.0,
        "price": 100.0,
        "freshness_days": 5.0,
    }
    user_context = {"category_affinities": {"Electronics": 8.0}}
    item_store = {
        "historical_ctr": 0.08,
        "conversion_rate": 0.03,
    }

    features = extract_candidate_features(
        cand,
        user_context=user_context,
        item_store_features=item_store,
        position=4,
    )
    assert features.listing_id == "item-1"
    assert features.similarity_score == 0.85
    assert features.category_match == 0.8
    assert features.historical_ctr == 0.08
    assert features.conversion_rate == 0.03
    assert len(features.to_vector()) == 7
    # IPS weight for pos=4 with gamma=0.5 is 2.0
    assert abs(features.position_weight - 2.0) < 1e-4


def test_gbdt_ranker_outperforms_raw_cosine_ndcg() -> None:
    ground_truth = ["item-tech-2", "item-tech-1"]

    candidates = [
        {
            "listing_id": "item-misc-1",
            "score": 0.95,  # Highest raw cosine
            "category_id": "Home",
            "popularity_score": 10.0,
        },
        {
            "listing_id": "item-tech-1",
            "score": 0.85,
            "category_id": "Tech",
            "popularity_score": 60.0,
        },
        {
            "listing_id": "item-tech-2",
            "score": 0.88,
            "category_id": "Tech",
            "popularity_score": 80.0,
        },
    ]
    user_context = {"category_affinities": {"Tech": 9.0, "Home": 0.0}}
    item_features_map = {
        "item-tech-2": {"historical_ctr": 0.12, "conversion_rate": 0.05},
        "item-tech-1": {"historical_ctr": 0.09, "conversion_rate": 0.04},
        "item-misc-1": {"historical_ctr": 0.01, "conversion_rate": 0.005},
    }

    raw_sorted = sorted(candidates, key=lambda c: -c["score"])
    baseline_ranking = [c["listing_id"] for c in raw_sorted]
    baseline_ndcg = ndcg_at_k(ground_truth, baseline_ranking, k=3)

    ranker = GBDTRanker()
    gbdt_ranked = ranker.rank_candidates(
        candidates,
        user_context=user_context,
        item_features_map=item_features_map,
        top_k=3,
    )
    gbdt_ranking = [item_id for item_id, _ in gbdt_ranked]
    gbdt_ndcg = ndcg_at_k(ground_truth, gbdt_ranking, k=3)

    assert gbdt_ranking[0] in ("item-tech-2", "item-tech-1")
    assert gbdt_ndcg > baseline_ndcg
    assert gbdt_ndcg >= 0.90
