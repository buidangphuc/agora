"""The numpy LambdaMART: it learns a planted ranking signal and the exported JSON scores the same."""

from __future__ import annotations

import json
import math

import numpy as np

from recsys.ranker import gbdt


def _lists(n_lists=120, size=6, seed=0):
    """Rows (price, noise) in lists; relevance rises when the price is low."""
    rng = np.random.default_rng(seed)
    x = rng.uniform(0, 1, size=(n_lists * size, 2))
    p = 0.85 - 0.8 * x[:, 0]
    labels = (rng.uniform(size=len(x)) < p).astype(float)
    groups = [list(range(i * size, (i + 1) * size)) for i in range(n_lists)]
    return x, labels, groups


def _mean_ndcg(model, x, labels, groups):
    tie = np.arange(len(x))
    return gbdt.mean_ndcg(groups, labels, model.predict(x), tie)[0]


def test_it_learns_a_planted_signal_and_beats_random_and_the_wrong_direction():
    x, labels, groups = _lists()
    lists = gbdt.Lists.build(groups, labels)
    model = gbdt.fit(x, lists, trees=30, max_depth=2, learning_rate=0.2, min_leaf=5)
    learned = _mean_ndcg(model, x, labels, groups)
    wrong = gbdt.mean_ndcg(groups, labels, x[:, 0], np.arange(len(x)))[0]  # high price first
    assert learned > 0.9 and learned > wrong + 0.15
    test_x, test_labels, test_groups = _lists(seed=99)
    assert _mean_ndcg(model, test_x, test_labels, test_groups) > 0.85  # and it generalises


def test_a_cheap_item_outscores_an_expensive_one():
    x, labels, groups = _lists()
    model = gbdt.fit(
        x, gbdt.Lists.build(groups, labels), trees=20, max_depth=2, learning_rate=0.2, min_leaf=5
    )
    cheap, dear = model.predict(np.array([[0.05, 0.5], [0.95, 0.5]]))
    assert cheap > dear


def test_training_is_deterministic():
    x, labels, groups = _lists(n_lists=40)
    lists = gbdt.Lists.build(groups, labels)
    a = gbdt.fit(x, lists, trees=5, max_depth=2, min_leaf=5)
    b = gbdt.fit(x, lists, trees=5, max_depth=2, min_leaf=5)
    assert [t.to_dict() for t in a.trees] == [t.to_dict() for t in b.trees]


def test_lists_without_a_mixed_label_teach_nothing():
    x = np.random.default_rng(1).uniform(size=(30, 2))
    groups = [list(range(i * 6, (i + 1) * 6)) for i in range(5)]
    model = gbdt.fit(x, gbdt.Lists.build(groups, np.zeros(30)), trees=5, min_leaf=2)
    assert model.trees == []  # no pair, no gradient: stops instead of fitting noise


def test_binning_and_raw_thresholds_agree():
    x = np.random.default_rng(2).uniform(size=(500, 3)) * [1, 100, 1e6]
    edges = gbdt.make_edges(x)
    xb = gbdt.bin_matrix(x, edges)
    for j, e in enumerate(edges):
        for b in range(len(e)):
            assert np.array_equal(xb[:, j] <= b, x[:, j] <= e[b])


def test_the_exported_json_scores_like_the_trainer():
    x, labels, groups = _lists()
    model = gbdt.fit(
        x, gbdt.Lists.build(groups, labels), trees=15, max_depth=3, learning_rate=0.15, min_leaf=5
    )
    art = json.loads(
        json.dumps(gbdt.to_artifact(model, model_version="gbdt-v", generation="v", metrics={"ndcg@10": 1.0}))
    )
    assert art["trees"] and art["learning_rate"] == 0.15
    reference = model.predict(x)
    for i in (0, 7, 33, 100, 500, len(x) - 1):
        assert math.isclose(gbdt.score_vector(art, list(x[i])), reference[i], rel_tol=1e-12, abs_tol=1e-12)


def test_ndcg_is_graded_and_none_without_a_positive():
    assert gbdt.ndcg_at_k([0, 0, 0]) is None
    assert gbdt.ndcg_at_k([2, 1, 0]) == 1.0
    worse = gbdt.ndcg_at_k([0, 1, 2])
    assert worse is not None and worse < 1.0
    # an add-to-cart (gain 3) first beats a click (gain 1) first
    assert gbdt.ndcg_at_k([2, 1]) > gbdt.ndcg_at_k([1, 2])


def test_lambda_gradients_push_the_more_relevant_item_up():
    lists = gbdt.Lists.build([[0, 1, 2]], np.array([1.0, 0.0, 0.0]))
    grad, hess = gbdt.lambdarank_gradients(np.zeros(3), lists)
    assert grad[0] > 0 > grad[1] and grad[2] < 0 and abs(grad.sum()) < 1e-12 and (hess > 0).all()
    # once it is already far ahead the push fades
    grad_far, _ = gbdt.lambdarank_gradients(np.array([10.0, 0.0, 0.0]), lists)
    assert grad_far[0] < grad[0] * 0.01
