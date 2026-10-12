"""Ranker position debias by (item_list_id, position) -- spec: recsys-consent-and-context / recommendations.

Seam: ``training.ctr_stats(frame, train)`` -> {listing_id: (position-weighted clicks, impressions)}.
"""

from __future__ import annotations

from datetime import datetime

import numpy as np
import pandas as pd
import pytest

from recsys.ranker import training

T0 = datetime(2026, 9, 10, 8)


def _frame(rows, with_list_id=True):
    cols = list(training.DATASET_COLUMNS)
    data = []
    for i, (lid, pos, label) in enumerate(rows):
        row = {"user_key": "u", "impression_id": f"i{i}", "listing_id": lid, "position": pos,
               "label": label, "occurred_at": T0}  # fmt: skip
        if with_list_id:
            row["item_list_id"] = "list-1"
        data.append(row)
    if with_list_id:
        cols.append("item_list_id")
    df = pd.DataFrame(data, columns=cols)
    df["occurred_at"] = pd.to_datetime(df["occurred_at"])
    return df


def _stats(df):
    return training.ctr_stats(df, np.arange(len(df)))


def test_same_ctr_worse_position_higher_weight():
    # Scenario: Same CTR at worse position gets higher weight
    rows = [("far", 8, 1), ("far", 8, 0), ("near", 1, 1), ("near", 1, 0)]
    stats = _stats(_frame(rows))
    assert stats["far"][1] == stats["near"][1] == 2
    assert stats["far"][0] > stats["near"][0]


def test_missing_item_list_id_keeps_position_only():
    # Scenario: Missing item_list_id keeps position-only behaviour
    rows = [("a", 8, 1), ("a", 3, 0), ("b", 1, 1), ("b", 5, 1)]
    stats = _stats(_frame(rows, with_list_id=False))
    gamma = training.contract.IPS_GAMMA
    assert stats["a"] == (8**gamma, 2)
    assert stats["b"] == (1 + 5**gamma, 2)


def test_non_positive_position_is_clamped():
    # Scenario: Non-positive position is clamped
    clamped = _stats(_frame([("z", 0, 1), ("n", -3, 1), ("one", 1, 1)]))
    assert clamped["z"] == clamped["one"]
    assert clamped["n"] == clamped["one"]
    assert clamped["one"][0] > 0


def _cell(lid, pos, imps, clicks):
    return [(lid, pos, 1)] * clicks + [(lid, pos, 0)] * (imps - clicks)


def test_list_propensity_replaces_position_only_weight():
    # Scenario: list propensity: smoothed CTR 0.20 at pos 1 vs 0.05 at pos 4 (198 imps each) -> weight 4.0
    rows = _cell("p1", 1, 198, 39) + _cell("p4", 4, 198, 9)
    stats = _stats(_frame(rows))
    assert stats["p1"] == (pytest.approx(39.0), 198)
    assert stats["p4"][0] == pytest.approx(9 * 4.0)  # position-only would give 9 * 2.0
    assert stats["p4"][1] == 198


def test_sparse_list_cell_falls_back_to_position_only():
    # Scenario: a cell with < 100 impressions falls back to max(pos,1) ** IPS_GAMMA
    rows = _cell("p1", 1, 198, 39) + _cell("p4", 4, 50, 5)
    stats = _stats(_frame(rows))
    assert stats["p4"][0] == pytest.approx(5 * 4**training.contract.IPS_GAMMA)
