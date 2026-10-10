"""Point-in-time features, temporal split and debiased CTR of the GBDT trainer (no Spark)."""

from __future__ import annotations

from datetime import datetime, timedelta

import numpy as np
import pandas as pd
import pytest

from recsys.ranker import contract, training
from tests import ranker_fixture as fx


def _frame(rows):
    df = pd.DataFrame(rows, columns=list(training.DATASET_COLUMNS))
    df["occurred_at"] = pd.to_datetime(df["occurred_at"])
    return df.sort_values(["occurred_at", "impression_id", "listing_id"], kind="stable").reset_index(
        drop=True
    )


def _row(imp, lid, label, at, pos=1, user="u"):
    return {"user_key": user, "impression_id": imp, "listing_id": lid, "position": pos, "label": label,
            "occurred_at": at}  # fmt: skip


T0 = datetime(2026, 9, 10, 8)


def _series(tmp_path, name, view, entity, snapshots):
    d = tmp_path / name
    d.mkdir(exist_ok=True)
    for stamp, rows, cols in snapshots:
        pd.DataFrame(rows, columns=cols).to_parquet(d / f"as_of={stamp}.parquet", index=False)
    return training.SnapshotSeries(str(d), view, 1, entity)


def _pop(views):
    return ["gb", views, 0, 0, 0, 0, None, 0.5]


def test_the_snapshot_in_force_is_the_latest_not_after_the_impression(tmp_path):
    s = _series(tmp_path, "p", "item_popularity", "listing_id", [
        ("20260901T000000Z", [_pop(10)], fx.POP_COLS),
        ("20260909T000000Z", [_pop(20)], fx.POP_COLS),
        ("20260920T000000Z", [_pop(30)], fx.POP_COLS),
    ])  # fmt: skip
    assert s.at(datetime(2026, 8, 31)) is None
    assert s.at(datetime(2026, 9, 1)).path.name == "as_of=20260901T000000Z.parquet"  # equal counts
    assert s.at(datetime(2026, 9, 12)).path.name == "as_of=20260909T000000Z.parquet"
    assert s.at(datetime(2027, 1, 1)).path.name == "as_of=20260920T000000Z.parquet"


def test_rows_join_the_snapshot_of_their_time_in_contract_order_and_missing_is_zero(tmp_path):
    pop = _series(tmp_path, "p", "item_popularity", "listing_id", [
        ("20260901T000000Z", [["gb", 1, 2, 3, 4, 5, 3.5, 0.25]], fx.POP_COLS),
        ("20260915T000000Z", [["gb", 100, 2, 3, 4, 5, None, 0.25]], fx.POP_COLS),
    ])  # fmt: skip
    attrs = _series(tmp_path, "a", "item_attributes", "listing_id", [
        ("20260901T000000Z", [["gb", "s", "cat", 4200]], ["listing_id", "seller_id", "category_id", "price"]),
    ])  # fmt: skip
    frame = _frame([
        _row("i1", "gb", 1, T0), _row("i2", "gb", 0, datetime(2026, 9, 16)), _row("i3", "other", 0, T0),
        _row("i0", "gb", 0, datetime(2026, 8, 1)),
    ])  # fmt: skip
    m = training.build_matrix(frame, pop, attrs)
    assert (
        m.counts["rows_dropped_no_snapshot"] == 1 and len(m.frame) == 3
    )  # the August impression predates every snapshot
    by_imp = dict(zip(m.frame["impression_id"], m.x, strict=True))
    assert list(by_imp["i1"]) == [
        1,
        2,
        3,
        4,
        5,
        3.5,
        0.25,
        4200,
    ]  # the September 1 snapshot, in contract order
    assert by_imp["i2"][0] == 100 and by_imp["i2"][5] == 0.0  # the 15th snapshot; null avg_rating is 0
    assert list(by_imp["i3"]) == [0.0] * 8 and m.counts["listing_not_in_popularity"] == 1


def test_the_lineage_names_only_the_snapshots_that_were_used(tmp_path):
    pop = _series(tmp_path, "p", "item_popularity", "listing_id", [
        ("20260901T000000Z", [_pop(1)], fx.POP_COLS), ("20261201T000000Z", [_pop(2)], fx.POP_COLS),
    ])  # fmt: skip
    training.build_matrix(_frame([_row("i1", "gb", 0, T0)]), pop, None)
    used = pop.lineage["snapshots"]
    assert [u["snapshot"] for u in used] == ["as_of=20260901T000000Z.parquet"]
    assert len(used[0]["sha256"]) == 64 and used[0]["rows"] == 1


def test_the_split_holds_out_the_latest_impressions_and_never_mixes_a_list():
    rows = [_row(f"i{n}", f"l{k}", 0, T0 + timedelta(hours=n)) for n in range(10) for k in range(3)]
    f = _frame(rows)
    split = training.temporal_split(f, 0.2)
    assert (split.train_lists, split.test_lists) == (8, 2)
    train_imps, test_imps = set(f["impression_id"].iloc[split.train]), set(
        f["impression_id"].iloc[split.test]
    )
    assert not train_imps & test_imps and test_imps == {"i8", "i9"}
    assert split.train_last_at < split.test_first_at == split.cutoff


def test_lists_are_bounded_by_position_and_recency():
    rows = [_row(f"i{n}", f"l{k}", 0, T0 + timedelta(hours=n), pos=k + 1) for n in range(6) for k in range(5)]
    f = training.limit_lists(_frame(rows), max_list=3, max_lists=4)
    assert sorted(set(f["impression_id"])) == ["i2", "i3", "i4", "i5"]  # the latest four impressions
    assert f.groupby("impression_id").size().eq(3).all() and f["position"].max() == 3


def test_debiased_ctr_uses_train_rows_only_and_never_a_rows_own_outcome(tmp_path):
    pop = _series(
        tmp_path, "p", "item_popularity", "listing_id", [("20260901T000000Z", [_pop(1)], fx.POP_COLS)]
    )
    # item "gb": 5 train impressions (clicked in 2, positions 1 and 4), 1 held-out impression that was clicked
    rows = [_row(f"t{n}", "gb", 1 if n in (0, 3) else 0, T0 + timedelta(hours=n), pos=4 if n == 3 else 1)
            for n in range(5)]  # fmt: skip
    rows.append(_row("h0", "gb", 1, T0 + timedelta(days=3)))
    m = training.build_matrix(_frame(rows), pop, None)
    split = training.Split(np.arange(5), np.array([5]), None, None, None, 5, 1)
    x, source = training.apply_debiased_ctr(m, split.train, min_impressions=3)
    ctr = x[:, training.CTR_COL]
    # row t0 (clicked, pos 1): others are t1..t4 -> clicks_ips = 4**0.5, impressions 4
    assert ctr[0] == pytest.approx(min(1.0, 2.0 / 4))
    # row t1 (not clicked): others clicked at pos 1 and pos 4 -> (1 + 2) / 4
    assert ctr[1] == pytest.approx(3.0 / 4)
    # the held-out row sees the full train stats (3 / 5) and none of its own click
    assert ctr[5] == pytest.approx(3.0 / 5)
    assert set(source) == {training.DEBIASED}


def test_ctr_source_is_fallback_below_the_impression_threshold(tmp_path):
    pop = _series(
        tmp_path, "p", "item_popularity", "listing_id", [("20260901T000000Z", [_pop(1)], fx.POP_COLS)]
    )
    rows = [_row(f"t{n}", "gb", 1, T0 + timedelta(hours=n)) for n in range(3)]  # 3 impressions: 2 others each
    m = training.build_matrix(_frame(rows), pop, None)
    x, source = training.apply_debiased_ctr(m, np.arange(3), min_impressions=3)
    assert set(source) == {training.FALLBACK} and list(x[:, training.CTR_COL]) == [
        0.5,
        0.5,
        0.5,
    ]  # the snapshot's ctr_7d
    x, source = training.apply_debiased_ctr(m, np.arange(3), min_impressions=2)
    assert set(source) == {training.DEBIASED} and list(x[:, training.CTR_COL]) == [1.0, 1.0, 1.0]


def test_the_baseline_is_the_incumbent_weights_on_popularity_and_ctr():
    x = np.zeros((2, 8))
    x[1, contract.feature_index("item_popularity.add_to_cart_7d")] = 100
    x[1, training.CTR_COL] = 0.5
    base = training.baseline_scores(x)
    pop = min(1.0, np.log1p(500) / np.log1p(500))
    assert base[0] == 0.0 and base[1] == pytest.approx(0.15 * pop + 0.10 * 0.5)
    assert training.baseline_scores(np.array([[1e9] * 8]))[0] <= 0.15 + 0.10  # capped


def test_the_ranking_dataset_must_have_its_columns(tmp_path):
    bad = tmp_path / "x.parquet"
    pd.DataFrame({"user_key": ["u"]}).to_parquet(bad)
    with pytest.raises(Exception, match="lacks columns"):
        training.read_dataset(str(bad))
