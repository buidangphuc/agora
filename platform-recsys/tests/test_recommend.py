"""Ranking helpers (numpy, no Spark) — Top-N, similar items, popularity."""

import numpy as np

from recsys import recommend


def test_l2_normalize_unit_length():
    v = recommend.l2_normalize([3.0, 4.0])
    assert abs(np.linalg.norm(v) - 1.0) < 1e-9


def test_l2_normalize_zero_vector_safe():
    assert recommend.l2_normalize([0.0, 0.0]) == [0.0, 0.0]


def test_top_n_for_users_ranks_by_dot_product():
    user_ids = ["u1"]
    user_vecs = [[1.0, 0.0]]
    item_ids = ["a", "b", "c"]
    item_vecs = [[1.0, 0.0], [0.0, 1.0], [0.5, 0.0]]
    out = recommend.top_n_for_users(user_ids, user_vecs, item_ids, item_vecs, top_n=2)
    ranked = [lid for lid, _ in out["u1"]]
    assert ranked == ["a", "c"]  # a (1.0) > c (0.5) > b (0.0)


def test_similar_items_excludes_self():
    item_ids = ["a", "b", "c"]
    item_vecs = [[1.0, 0.0], [0.9, 0.1], [0.0, 1.0]]
    out = recommend.similar_items(item_ids, item_vecs, top_n=2)
    assert "a" not in [lid for lid, _ in out["a"]]
    assert out["a"][0][0] == "b"  # b most similar to a


def test_popularity_ranking_sums_weight():
    pairs = [("a", 1.0), ("b", 5.0), ("a", 2.0), ("c", 0.5)]
    ranked = recommend.popularity_ranking(pairs, top_n=2)
    assert ranked[0] == ("b", 5.0)
    assert ranked[1] == ("a", 3.0)


def test_empty_inputs_safe():
    assert recommend.top_n_for_users([], [], [], [], 5) == {}
    assert recommend.similar_items([], [], 5) == {}
