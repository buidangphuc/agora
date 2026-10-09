"""Structural gate: each check rejects on its own, in order, and a healthy candidate passes."""

from __future__ import annotations

import math

import pytest

from recsys.config import Settings
from recsys.structural_gate import mean_list_overlap, structural_check

SETTINGS = Settings()  # 0.5 / 0.05 / 0.9


def _lists(per_user: dict[str, list[str]]):
    return {u: [(lid, 1.0) for lid in items] for u, items in per_user.items()}


def _healthy(n_users=10, n_items=40, n=5):
    """Every user gets its own window of the catalogue: broad coverage, little overlap."""
    return _lists({f"u{u}": [f"l{(u * n + k) % n_items}" for k in range(n)] for u in range(n_users)})


GOOD_FACTORS = ([[0.1, 0.2]], [[0.3, 0.4]])


def test_healthy_candidate_passes():
    ok, reason = structural_check(
        _healthy(), {}, GOOD_FACTORS, dataset_users=10, dataset_items=40, settings=SETTINGS
    )
    assert ok, reason


def test_low_user_coverage_rejects():
    recs = _lists({"u0": ["l1", "l2"], "u1": []})
    recs.update({f"u{i}": [] for i in range(2, 10)})
    ok, reason = structural_check(
        recs, {}, GOOD_FACTORS, dataset_users=10, dataset_items=40, settings=SETTINGS
    )
    assert not ok and "user coverage" in reason and "GATE_MIN_USER_COVERAGE" in reason


def test_users_missing_from_the_lists_count_against_coverage():
    recs = _healthy(n_users=4)  # 4 of the dataset's 10 users have a list
    ok, reason = structural_check(
        recs, {}, GOOD_FACTORS, dataset_users=10, dataset_items=40, settings=SETTINGS
    )
    assert not ok and "user coverage" in reason


def test_low_item_coverage_rejects():
    # 10 users x same-ish 2 items out of a 100-item catalogue: 2% < 5%, with lists that differ.
    recs = _lists({f"u{u}": ["l0"] if u % 2 else ["l1"] for u in range(10)})
    ok, reason = structural_check(
        recs, {}, GOOD_FACTORS, dataset_users=10, dataset_items=100, settings=SETTINGS
    )
    assert not ok and "item coverage" in reason and "GATE_MIN_ITEM_COVERAGE" in reason


def test_identical_lists_reject_on_overlap():
    recs = _lists({f"u{u}": ["l1", "l2", "l3"] for u in range(10)})
    ok, reason = structural_check(
        recs, {}, GOOD_FACTORS, dataset_users=10, dataset_items=3, settings=SETTINGS
    )
    assert not ok and "list overlap" in reason and "GATE_MAX_LIST_OVERLAP" in reason


@pytest.mark.parametrize("bad", [math.nan, math.inf, -math.inf])
@pytest.mark.parametrize("which", [0, 1])
def test_non_finite_factors_reject(bad, which):
    factors = [[[0.1, 0.2]], [[0.3, 0.4]]]
    factors[which] = [[0.1, bad]]
    ok, reason = structural_check(
        _healthy(), {}, tuple(factors), dataset_users=10, dataset_items=40, settings=SETTINGS
    )
    assert not ok and "NaN or infinite" in reason


def test_non_finite_factors_are_reported_before_the_ratios():
    ok, reason = structural_check(
        {}, {}, ([[math.nan]], [[1.0]]), dataset_users=10, dataset_items=40, settings=SETTINGS
    )
    assert not ok and "NaN or infinite" in reason


def test_thresholds_come_from_settings():
    recs = _lists({f"u{u}": ["l1", "l2", "l3"] for u in range(10)})
    lax = Settings(gate_max_list_overlap=1.0)
    ok, _ = structural_check(recs, {}, GOOD_FACTORS, dataset_users=10, dataset_items=3, settings=lax)
    assert ok


def test_overlap_of_disjoint_and_identical_lists():
    assert mean_list_overlap(_lists({"a": ["x", "y"], "b": ["z", "w"]})) == 0.0
    assert mean_list_overlap(_lists({"a": ["x", "y"], "b": ["y", "x"]})) == 1.0
    assert mean_list_overlap(_lists({"a": ["x", "y"], "b": ["y", "z"]})) == pytest.approx(1 / 3)
    assert mean_list_overlap(_lists({"a": ["x"]})) == 0.0


def test_overlap_sampling_is_deterministic_and_bounded():
    recs = _lists({f"u{u}": [f"l{u % 7}", f"l{u % 11}", f"l{u % 13}"] for u in range(200)})  # 19,900 pairs
    first = mean_list_overlap(recs)
    assert first == mean_list_overlap(recs)
    assert 0.0 < first < 1.0
