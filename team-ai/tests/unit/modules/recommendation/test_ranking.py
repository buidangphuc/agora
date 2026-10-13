"""Ranking: the one filter, similarity order, fixed weights, and the online boost."""

import pytest

from app.modules.business.recommendation.ranking.boost import apply_online_features
from app.modules.business.recommendation.ranking.filter import eligible
from app.modules.business.recommendation.ranking.linear import (
    rank_by_fixed_weights,
    rank_by_similarity,
)
from app.modules.business.recommendation.ranking.rows import popularity
from app.modules.business.recommendation.schemas import Candidate, RecommendedItem


def _ids(items: list[RecommendedItem]) -> list[str]:
    return [i.listing_id for i in items]


def test_eligible_drops_empty_ids_the_seed_and_out_of_stock_keeping_the_best_duplicate() -> (
    None
):
    got = eligible(
        [
            Candidate("", 9.0),
            Candidate("seed", 1.0),
            Candidate("a", 0.5),
            Candidate("b", 0.8, in_stock=False),
            Candidate("a", 0.9),
            Candidate("a", 0.9),
            Candidate("c", 0.1),
        ],
        "seed",
    )
    assert [(c.listing_id, c.score) for c in got] == [("a", 0.9), ("c", 0.1)]


def test_eligible_keeps_first_seen_order_of_distinct_ids() -> None:
    got = eligible([Candidate("z", 0.1), Candidate("a", 0.9), Candidate("z", 0.2)], "")
    assert [c.listing_id for c in got] == ["z", "a"]


def test_similarity_rank_orders_by_score_with_stable_ties_and_one_based_ranks() -> None:
    items = rank_by_similarity(
        [Candidate("a", 0.5), Candidate("b", 0.9), Candidate("c", 0.5)], "", 0
    )
    assert _ids(items) == ["b", "a", "c"]
    assert [i.rank for i in items] == [1, 2, 3]


def test_similarity_rank_limit_zero_or_negative_returns_everything() -> None:
    cands = [Candidate(c, 1.0 - i / 10) for i, c in enumerate("abcd")]
    assert len(rank_by_similarity(cands, "", 2)) == 2
    assert len(rank_by_similarity(cands, "", 0)) == 4
    assert len(rank_by_similarity(cands, "", -1)) == 4


def test_fixed_weights_score_is_similarity_popularity_and_ctr() -> None:
    row = {"views_7d": 400, "clicks_7d": 80, "ctr_7d": 0.2}
    [item] = rank_by_fixed_weights([Candidate("a", 0.8)], "", {"a": row}, {}, 0)
    assert item.score == pytest.approx(0.8 * 0.30 + popularity(row) * 0.15 + 0.2 * 0.10)


def test_fixed_weights_without_features_scores_similarity_alone() -> None:
    [item] = rank_by_fixed_weights([Candidate("a", 0.5)], "", {}, {}, 0)
    assert item.score == pytest.approx(0.5 * 0.30)


def test_nearline_ctr_replaces_the_prior_when_positive() -> None:
    cands = [Candidate("a", 0.8), Candidate("b", 0.8)]
    prior = {"a": {"ctr_7d": 0.02}, "b": {"ctr_7d": 0.02}}
    items = rank_by_fixed_weights(cands, "", prior, {"b": 0.11, "a": 0.0}, 0)
    assert _ids(items) == ["b", "a"]
    assert items[0].score == pytest.approx(0.8 * 0.30 + 0.11 * 0.10)
    assert items[1].score == pytest.approx(0.8 * 0.30 + 0.02 * 0.10)  # zero is no data


def test_ctr_is_clamped_to_the_unit_interval() -> None:
    [item] = rank_by_fixed_weights(
        [Candidate("a", 0.0)], "", {"a": {"ctr_7d": 50.0}}, {}, 0
    )
    assert item.score == pytest.approx(0.1)


def test_fixed_weights_use_the_one_filter() -> None:
    items = rank_by_fixed_weights(
        [
            Candidate("seed", 1.0),
            Candidate("a", 0.5, in_stock=False),
            Candidate("b", 0.1),
        ],
        "seed",
        {},
        {},
        0,
    )
    assert _ids(items) == ["b"]


def _items(*ids_scores: tuple[str, float]) -> list[RecommendedItem]:
    return [
        RecommendedItem(listing_id=i, score=s, rank=n)
        for n, (i, s) in enumerate(ids_scores, start=1)
    ]


def test_boost_without_features_only_cuts_to_the_limit() -> None:
    items = _items(("a", 0.5), ("b", 0.4), ("c", 0.3))
    assert apply_online_features(items, {}, 2) == items[:2]
    assert apply_online_features(items, {}, 0) == items


def test_boost_breaks_a_tie_by_ctr() -> None:
    got = apply_online_features(
        _items(("first", 0.5), ("second", 0.5)), {"second": {"ctr_7d": 0.5}}, 0
    )
    assert _ids(got) == ["second", "first"]
    assert [i.rank for i in got] == [1, 2]


def test_boost_breaks_a_ctr_tie_by_favorites() -> None:
    features = {
        "first": {"ctr_7d": 0.2, "favorites_current": 1},
        "second": {"ctr_7d": 0.2, "favorites_current": 9},
    }
    got = apply_online_features(_items(("first", 0.5), ("second", 0.5)), features, 0)
    assert _ids(got) == ["second", "first"]


def test_boost_is_bounded_so_the_model_score_stays_dominant() -> None:
    got = apply_online_features(
        _items(("strong", 0.9), ("weak", 0.3)), {"weak": {"ctr_7d": 1e9}}, 0
    )
    assert _ids(got) == ["strong", "weak"]
    assert got[1].score == pytest.approx(0.35)


@pytest.mark.parametrize("limit", [1, 2])
def test_limit_is_applied_after_the_boost(limit: int) -> None:
    got = apply_online_features(
        _items(("first", 0.5), ("second", 0.5)), {"second": {"ctr_7d": 0.5}}, limit
    )
    assert len(got) == limit
    assert got[0].listing_id == "second"
