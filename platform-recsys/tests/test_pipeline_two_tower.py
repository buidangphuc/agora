"""The two-tower stage inside pipeline.run (needs PySpark: runs in the image)."""

from __future__ import annotations

from datetime import datetime, timedelta, timezone

import pandas as pd
import pytest

pytest.importorskip("pyspark")

from recsys.config import ConfigError, Settings  # noqa: E402
from recsys.pipeline import run  # noqa: E402
from recsys.registry.registry import ModelRegistry  # noqa: E402
from tests.dataset_fixture import write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402

ITEM_COLS = ["listing_id", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "review_count",
             "avg_rating", "ctr_7d"]  # fmt: skip
USER_COLS = ["user_key", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "follows_current",
             "paid_orders_30d"]  # fmt: skip


def _rows():
    now = datetime.now(timezone.utc)
    rows = []
    for u, items in [
        ("u1", "abc"),
        ("u2", "abc"),
        ("u3", "abc"),
        ("u4", "def"),
        ("u5", "def"),
        ("u6", "def"),
    ]:
        for h, ch in enumerate(items):
            rows.append(
                {"user_key": u, "listing_id": f"l{ch}", "weight": 3.0 - h, "interactions": 1,
                 "last_occurred_at": now - timedelta(hours=h + 1)}
            )  # fmt: skip
    return rows


def _features(tmp_path, cold_extra=()):
    items = [[f"l{c}", 10 * (i + 1), i, i // 2, i, 0, None, 0.1 * i] for i, c in enumerate("abcdef")]
    items.append(["l-cold", 0, 0, 0, 2, 1, 4.0, 0.0])  # favourited and reviewed, never in the dataset
    items.extend(cold_extra)
    d = tmp_path / "feat"
    (d / "i").mkdir(parents=True)
    (d / "u").mkdir()
    pd.DataFrame(items, columns=ITEM_COLS).to_parquet(d / "i" / "as_of=20261005T000000Z.parquet", index=False)
    users = [[f"u{n}", 5 * n, n, 0, 0, 0, n % 2] for n in range(1, 7)]
    pd.DataFrame(users, columns=USER_COLS).to_parquet(d / "u" / "as_of=20261005T000000Z.parquet", index=False)
    return {"item_features_dir": str(d / "i"), "user_features_dir": str(d / "u")}


def _settings(data, version, **kw):
    return Settings(
        spark_master="local[1]",
        dataset_dir=str(data),
        als_max_iter=3,
        als_rank=4,
        top_n=2,
        model_version=version,
        promotion_force=True,
        two_tower_dim=8,
        **kw,
    )


def _run(data, version, registry, redis, qdrant, **kw):
    return run(_settings(data, version, **kw), registry=registry, redis_client=redis, qdrant_client=qdrant)


def test_enabled_stage_indexes_a_cold_item_with_the_run_generation(tmp_path):
    write_dataset(tmp_path / "d", _rows())
    feats = _features(tmp_path)
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()

    summary = _run(tmp_path / "d", "g1", registry, redis, qdrant, enable_two_tower=True, **feats)

    assert summary["two_tower_items"] == 7 > 0
    coll = qdrant.collections["item_two_tower_vectors__g1"]
    assert len(coll) == summary["two_tower_items"]
    assert {p["payload"]["model_version"] for p in coll.values()} == {"g1"}
    by_listing = {p["payload"]["listing_id"]: p["vector"] for p in coll.values()}
    als_items = {p["payload"]["listing_id"] for p in qdrant.collections["item_als_vectors__g1"].values()}
    assert "l-cold" in by_listing and "l-cold" not in als_items  # a vector ALS cannot produce
    assert any(by_listing["l-cold"]) and by_listing["l-cold"] != by_listing["la"]
    meta = registry.get_model("g1").parameters["two_tower"]
    assert meta["items"] == 7 and meta["epochs"] == 5 and meta["loss_last"] is not None
    assert meta["features"]["items"]["view"] == "item_popularity"


def test_earlier_generations_are_pruned(tmp_path):
    write_dataset(tmp_path / "d", _rows())
    feats = _features(tmp_path)
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()
    for v in ("g1", "g2", "g3"):
        _run(tmp_path / "d", v, registry, redis, qdrant, enable_two_tower=True, **feats)
    assert sorted(c for c in qdrant.collections if "two_tower" in c) == [
        "item_two_tower_vectors__g2",
        "item_two_tower_vectors__g3",
    ]


def test_disabled_stage_leaves_the_als_run_unchanged(tmp_path):
    write_dataset(tmp_path / "d", _rows())
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()
    summary = _run(tmp_path / "d", "g1", registry, redis, qdrant)  # no feature snapshots needed either
    assert "two_tower_items" not in summary and "two_tower" not in summary
    assert not [c for c in qdrant.collections if "two_tower" in c]
    assert "two_tower" not in registry.get_model("g1").parameters


def test_missing_feature_snapshots_stop_the_run_before_anything_is_registered(tmp_path):
    write_dataset(tmp_path / "d", _rows())
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()
    with pytest.raises(ConfigError, match="ITEM_FEATURES_DIR"):
        _run(tmp_path / "d", "g1", registry, redis, qdrant, enable_two_tower=True,
             item_features_dir=str(tmp_path / "none"))  # fmt: skip
    assert registry.get_model("g1") is None and not qdrant.collections and not redis.store


def test_a_zero_vector_is_refused_and_never_reaches_qdrant(tmp_path):
    write_dataset(tmp_path / "d", _rows())
    feats = _features(tmp_path, cold_extra=[["l-blank", 0, 0, 0, 0, 0, None, 0.0]])
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()
    # epochs 0: the towers keep their zero bias, so a feature-less item embeds to zero
    summary = _run(tmp_path / "d", "g1", registry, redis, qdrant, enable_two_tower=True,
                   two_tower_epochs=0, **feats)  # fmt: skip
    assert summary["two_tower"]["refused"] == 1 and summary["two_tower_items"] == 7
    listed = {p["payload"]["listing_id"] for p in qdrant.collections["item_two_tower_vectors__g1"].values()}
    assert "l-blank" not in listed


def test_a_stage_that_cannot_produce_vectors_rejects_the_candidate_and_publishes_nothing(tmp_path):
    from recsys.two_tower.pipeline import DegenerateEmbeddingError

    write_dataset(tmp_path / "d", _rows())
    d = tmp_path / "feat"
    (d / "i").mkdir(parents=True)
    (d / "u").mkdir()
    pd.DataFrame([["l-blank", 0, 0, 0, 0, 0, None, 0.0]], columns=ITEM_COLS).to_parquet(
        d / "i" / "as_of=20261005T000000Z.parquet", index=False
    )
    pd.DataFrame([["u1", 1, 0, 0, 0, 0, 0]], columns=USER_COLS).to_parquet(
        d / "u" / "as_of=20261005T000000Z.parquet", index=False
    )
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()
    with pytest.raises(DegenerateEmbeddingError):
        _run(tmp_path / "d", "g1", registry, redis, qdrant, enable_two_tower=True, two_tower_epochs=0,
             item_features_dir=str(d / "i"), user_features_dir=str(d / "u"))  # fmt: skip
    assert registry.get_model("g1").status == "rejected"
    assert "two-tower stage failed" in registry.get_model("g1").metrics["gate_reason"]
    assert not qdrant.collections and "recs:v1:serving" not in redis.store
