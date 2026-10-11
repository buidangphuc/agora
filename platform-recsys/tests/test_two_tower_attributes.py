"""Item attributes and user preferences in the two-tower stage (featurestore-item-attributes; no Spark)."""

from __future__ import annotations

import hashlib

import pandas as pd
import pytest

from recsys.config import ConfigError, Settings, load_settings
from recsys.two_tower import features, stage

ITEM_COLS = ["listing_id", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "review_count",
             "avg_rating", "ctr_7d"]  # fmt: skip
USER_COLS = ["user_key", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "follows_current",
             "paid_orders_30d"]  # fmt: skip
ATTR_COLS = ["listing_id", "seller_id", "category_id", "price"]
PREF_COLS = ["user_key", "preferred_categories"]


def _write(path, rows, columns):
    path.parent.mkdir(parents=True, exist_ok=True)
    pd.DataFrame(rows, columns=columns).to_parquet(path, index=False)
    return path


def _settings(tmp_path, **kw) -> Settings:
    values = {
        "item_features_dir": str(tmp_path / "pop"),
        "user_features_dir": str(tmp_path / "act"),
        "item_attributes_dir": str(tmp_path / "attr"),
        "user_preferences_dir": str(tmp_path / "pref"),
        "two_tower_dim": 8,
        "two_tower_epochs": 3,
        **kw,
    }
    return Settings(**values)


def _base(tmp_path):
    _write(
        tmp_path / "pop" / "as_of=20261005T000000Z.parquet", [["warm", 30, 3, 1, 1, 0, None, 0.1]], ITEM_COLS
    )
    _write(tmp_path / "act" / "as_of=20261005T000000Z.parquet", [["u1", 10, 2, 0, 0, 0, 1]], USER_COLS)


def _attributes(tmp_path, rows=None):
    rows = rows or [
        ["warm", "s1", "cat-a", 120000],
        ["cold-a", "s1", "cat-a", 50000],
        ["cold-b", "s2", "cat-b", 50000],
    ]
    return _write(tmp_path / "attr" / "as_of=20261005T000000Z.parquet", rows, ATTR_COLS)


def test_new_settings_are_declared_with_their_defaults():
    s = load_settings({})
    assert s.item_attributes_dir == "/features/item_attributes/v1"
    assert s.user_preferences_dir == "/features/user_preferences/v1"
    assert s.two_tower_require_attributes is False and s.two_tower_max_categories == 64
    assert load_settings({"TWO_TOWER_REQUIRE_ATTRIBUTES": "true"}).two_tower_require_attributes is True


def test_attribute_snapshots_are_optional_by_default(tmp_path):
    _base(tmp_path)
    inputs = stage.resolve_inputs(_settings(tmp_path))
    assert inputs.attributes_snapshot is None and inputs.preferences_snapshot is None
    assert inputs.lineage["attributes"] is None and inputs.lineage["preferences"] is None


def test_required_attributes_missing_name_the_setting(tmp_path):
    _base(tmp_path)
    with pytest.raises(ConfigError, match="ITEM_ATTRIBUTES_DIR=") as exc:
        stage.resolve_inputs(_settings(tmp_path, two_tower_require_attributes=True))
    assert "item_attributes@v1" in str(exc.value)


def test_an_explicit_attribute_path_that_does_not_exist_is_an_error_even_when_optional(tmp_path):
    _base(tmp_path)
    with pytest.raises(ConfigError, match="ITEM_ATTRIBUTES_PATH="):
        stage.resolve_inputs(_settings(tmp_path, item_attributes_path=str(tmp_path / "nope.parquet")))


def test_attribute_lineage_names_view_file_and_hash(tmp_path):
    _base(tmp_path)
    attr = _attributes(tmp_path)
    pref = _write(tmp_path / "pref" / "as_of=20261005T000000Z.parquet", [["u1", "cat-a,cat-b"]], PREF_COLS)
    lineage = stage.resolve_inputs(_settings(tmp_path)).lineage
    assert lineage["attributes"] == {
        "view": "item_attributes",
        "version": 1,
        "snapshot": attr.name,
        "sha256": hashlib.sha256(attr.read_bytes()).hexdigest(),
    }
    assert lineage["preferences"]["view"] == "user_preferences"
    assert lineage["preferences"]["sha256"] == hashlib.sha256(pref.read_bytes()).hexdigest()


def test_item_and_user_feature_mappings_carry_attributes():
    item = features.item_features({"views_7d": 10}, {"category_id": "cat-a", "price": 1500})
    assert item["category_id"] == "cat-a" and item["price"] == 1500.0
    assert "category_id" not in features.item_features({"views_7d": 10})  # no attribute row: left out
    assert "price" not in features.item_features({"views_7d": 10})
    unknown = features.item_features({}, {"category_id": None, "price": None})
    assert "category_id" not in unknown and unknown["price"] == 0.0
    user = features.user_features({"views_7d": 4}, {"preferred_categories": "cat-b,cat-a"})
    assert user["preferred_categories"] == ["cat-b", "cat-a"]
    assert "preferred_categories" not in features.user_features({"views_7d": 4})


def test_category_vocabulary_is_most_frequent_first_and_capped():
    rows = {str(i): {"category_id": c} for i, c in enumerate(["b", "a", "b", "c", "a", "b", None, ""])}
    assert features.category_vocabulary(rows, 10) == ["b", "a", "c"]
    assert features.category_vocabulary(rows, 2) == ["b", "a"]
    assert features.category_vocabulary({}, 5) == []


def test_cold_items_with_different_categories_embed_differently_and_non_zero(tmp_path):
    _base(tmp_path)
    _attributes(tmp_path)
    settings = _settings(tmp_path)
    result = stage.run_stage(settings, stage.resolve_inputs(settings), [("u1", "warm"), ("u2", "warm")])
    # cold-a / cold-b are in no engagement snapshot: they exist only through their attributes.
    assert {"cold-a", "cold-b", "warm"} <= set(result.vectors)
    assert any(result.vectors["cold-a"]) and any(result.vectors["cold-b"])
    assert result.vectors["cold-a"] != result.vectors["cold-b"]
    assert result.lineage["category_vocab"] == 2
    assert result.lineage["attributes"]["view"] == "item_attributes"


def test_without_attributes_those_cold_items_do_not_exist_and_the_stage_is_as_before(tmp_path):
    _base(tmp_path)
    settings = _settings(tmp_path)
    result = stage.run_stage(settings, stage.resolve_inputs(settings), [("u1", "warm"), ("u2", "warm")])
    assert set(result.vectors) == {"warm"}
    assert result.lineage["attributes"] is None and result.lineage["category_vocab"] == 0


def test_an_attribute_less_cold_item_is_refused_not_invented(tmp_path):
    _base(tmp_path)
    _attributes(tmp_path, [["bare", "s1", None, None]])
    settings = _settings(tmp_path, two_tower_epochs=0)
    result = stage.run_stage(settings, stage.resolve_inputs(settings), [])
    assert "bare" not in result.vectors and result.report.refused == ["bare"]  # no signal: no vector


def test_preferences_reach_the_user_profiles(tmp_path, monkeypatch):
    _base(tmp_path)
    _attributes(tmp_path)
    _write(
        tmp_path / "pref" / "as_of=20261005T000000Z.parquet",
        [["u1", "cat-b,cat-a"], ["u9", "cat-a"]],
        PREF_COLS,
    )
    seen = {}
    real = stage.train_and_index_two_tower

    def spy(catalog, profiles, *args, **kwargs):
        seen["profiles"], seen["vocab"] = profiles, kwargs.get("category_vocab")
        return real(catalog, profiles, *args, **kwargs)

    monkeypatch.setattr(stage, "train_and_index_two_tower", spy)
    settings = _settings(tmp_path)
    stage.run_stage(
        settings, stage.resolve_inputs(settings), [("u1", "warm"), ("u9", "cold-a"), ("u5", "warm")]
    )
    by_user = {p["user_key"]: p for p in seen["profiles"]}
    assert by_user["u1"]["preferred_categories"] == ["cat-b", "cat-a"]
    assert by_user["u1"]["activity_score"] > 0  # activity row and preference row are merged
    assert by_user["u9"]["preferred_categories"] == ["cat-a"]  # preferences alone make a profile
    assert "u5" not in by_user  # no row in either snapshot
    assert seen["vocab"] == ["cat-a", "cat-b"]  # cat-a has two listings, cat-b one
