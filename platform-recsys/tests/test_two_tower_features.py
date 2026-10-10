"""Featurestore snapshot resolution and the feature mappings of the two-tower stage (no Spark)."""

from __future__ import annotations

import hashlib

import pandas as pd
import pytest

from recsys.config import ConfigError, Settings
from recsys.two_tower import features, stage


def _write(path, rows, columns):
    path.parent.mkdir(parents=True, exist_ok=True)
    pd.DataFrame(rows, columns=columns).to_parquet(path, index=False)
    return path


ITEM_COLS = [
    "listing_id",
    "views_7d",
    "clicks_7d",
    "add_to_cart_7d",
    "favorites_current",
    "review_count",
    "avg_rating",
    "ctr_7d",
]
USER_COLS = [
    "user_key",
    "views_7d",
    "clicks_7d",
    "add_to_cart_7d",
    "favorites_current",
    "follows_current",
    "paid_orders_30d",
]


def test_the_latest_snapshot_is_used_and_its_lineage_recorded(tmp_path):
    _write(tmp_path / "as_of=20261001T000000Z.parquet", [["a", 1, 0, 0, 0, 0, None, 0.0]], ITEM_COLS)
    newest = _write(tmp_path / "as_of=20261005T000000Z.parquet", [["b", 2, 0, 0, 0, 0, None, 0.0]], ITEM_COLS)

    snap = features.resolve_snapshot(str(tmp_path), "", "item_popularity", 1, "ITEM_FEATURES")

    assert snap.path == str(newest)
    assert snap.lineage == {
        "view": "item_popularity",
        "version": 1,
        "snapshot": "as_of=20261005T000000Z.parquet",
        "sha256": hashlib.sha256(newest.read_bytes()).hexdigest(),
    }
    assert set(features.read_rows(snap, "listing_id")) == {"b"}


def test_an_explicit_path_wins_over_the_directory(tmp_path):
    _write(tmp_path / "as_of=20261005T000000Z.parquet", [["b", 2, 0, 0, 0, 0, None, 0.0]], ITEM_COLS)
    explicit = _write(tmp_path / "other" / "mine.parquet", [["z", 1, 0, 0, 0, 0, None, 0.0]], ITEM_COLS)
    snap = features.resolve_snapshot(str(tmp_path), str(explicit), "item_popularity", 1, "ITEM_FEATURES")
    assert set(features.read_rows(snap, "listing_id")) == {"z"}


def test_no_snapshot_refuses_to_run_naming_the_setting(tmp_path):
    with pytest.raises(ConfigError, match="ITEM_FEATURES_DIR=") as exc:
        features.resolve_snapshot(str(tmp_path), "", "item_popularity", 1, "ITEM_FEATURES")
    assert "item_popularity@v1" in str(exc.value)
    with pytest.raises(ConfigError, match="USER_FEATURES_PATH="):
        features.resolve_snapshot(
            str(tmp_path), str(tmp_path / "x.parquet"), "user_activity", 2, "USER_FEATURES"
        )


def test_stage_resolves_both_snapshots_or_refuses(tmp_path):
    settings = Settings(item_features_dir=str(tmp_path / "item"), user_features_dir=str(tmp_path / "user"))
    with pytest.raises(ConfigError):
        stage.resolve_inputs(settings)
    _write(tmp_path / "item" / "as_of=20261005T000000Z.parquet", [["a", 1, 0, 0, 0, 0, None, 0.0]], ITEM_COLS)
    with pytest.raises(ConfigError, match="user_activity@v2"):
        stage.resolve_inputs(settings)
    _write(tmp_path / "user" / "as_of=20261005T000000Z.parquet", [["u", 1, 0, 0, 0, 0, 0]], USER_COLS)
    inputs = stage.resolve_inputs(settings)
    assert inputs.lineage["items"]["view"] == "item_popularity"
    assert inputs.lineage["users"]["version"] == 2


def test_item_features_map_the_view_and_are_bounded():
    quiet = features.item_features(
        {"views_7d": 0, "clicks_7d": 0, "add_to_cart_7d": 0, "favorites_current": 0}
    )
    busy = features.item_features(
        {"views_7d": 900, "clicks_7d": 90, "add_to_cart_7d": 30, "favorites_current": 40, "ctr_7d": 0.1}
    )
    assert quiet == {"historical_ctr": 0.0, "popularity_score": 0.0}
    assert busy["historical_ctr"] == pytest.approx(0.1)
    assert busy["popularity_score"] == 1.0  # capped
    mid = features.item_features({"views_7d": 20, "ctr_7d": None})
    assert 0.0 < mid["popularity_score"] < 1.0 and mid["historical_ctr"] == 0.0


def test_user_features_map_the_view():
    f = features.user_features(
        {"views_7d": 10, "clicks_7d": 3, "add_to_cart_7d": 1, "paid_orders_30d": 2, "follows_current": 5}
    )
    assert f["lifetime_purchases"] == 2.0
    assert 0.0 < f["activity_score"] < 1.0
    assert features.user_features({}) == {"lifetime_purchases": 0.0, "activity_score": 0.0}


def test_run_stage_gives_a_cold_item_its_own_non_zero_vector(tmp_path):
    items = [
        ["hot", 300, 60, 20, 30, 4, 4.5, 0.2],
        ["warm", 40, 6, 1, 2, 0, None, 0.15],
        ["cold-fav", 0, 0, 0, 1, 0, None, 0.0],  # favourited, never viewed: no interactions to train on
    ]
    _write(tmp_path / "i" / "as_of=20261005T000000Z.parquet", items, ITEM_COLS)
    _write(
        tmp_path / "u" / "as_of=20261005T000000Z.parquet",
        [["u1", 20, 5, 1, 1, 0, 1], ["u2", 2, 0, 0, 0, 0, 0]],
        USER_COLS,
    )
    settings = Settings(
        item_features_dir=str(tmp_path / "i"), user_features_dir=str(tmp_path / "u"), two_tower_dim=8
    )
    pairs = [("u1", "hot"), ("u1", "warm"), ("u2", "warm"), ("u2", "hot")]

    result = stage.run_stage(settings, stage.resolve_inputs(settings), pairs)

    assert set(result.vectors) == {"hot", "warm", "cold-fav"}
    assert all(any(v) for v in result.vectors.values())
    assert len({tuple(v) for v in result.vectors.values()}) == 3
    params = result.as_parameters(8)
    assert params["dim"] == 8 and params["items"] == 3 and params["pairs"] == 4
    assert params["features"]["items"]["snapshot"] == "as_of=20261005T000000Z.parquet"
