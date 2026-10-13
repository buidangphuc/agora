"""The two-tower stage inside pipeline.run (needs PySpark: runs in the image).

Ported to the MLflow registry: the candidate is a ``recsys-two-tower`` version judged by its own policy, the
stage failures mark the evaluated ALS candidate rejected, and a run starts from weak seeded champions where
the test needs a promotable candidate (an equal model would not beat its own champion by 0.5 %).
"""

from __future__ import annotations

import pytest

pytest.importorskip("pyspark")

from recsys.config import ConfigError  # noqa: E402
from recsys.pipeline import run  # noqa: E402
from tests.dataset_fixture import write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402
from tests.pipeline_fixture import (  # noqa: E402
    ALS,
    TWO_TOWER,
    als_rows,
    cold_item_features,
    newest,
    outcome,
    registered_models,
    requires_spark,
    seed_weak,
    settings,
    tower_features,
    versions,
)

pytestmark = requires_spark


def _cfg(data, version, reg, **kw):
    return settings(data, version, reg, two_tower_dim=8, **kw)


def test_enabled_stage_indexes_a_cold_item_with_the_run_generation(tmp_path, reg):
    write_dataset(tmp_path / "d", als_rows())
    feats = cold_item_features(tmp_path)
    redis, qdrant = FakeRedis(), FakeQdrantClient()

    summary = run(
        _cfg(tmp_path / "d", "g1", reg, enable_two_tower=True, **feats),
        redis_client=redis,
        qdrant_client=qdrant,
    )

    assert summary["two_tower_items"] == 7 > 0
    coll = qdrant.collections["item_two_tower_vectors__g1"]
    assert len(coll) == summary["two_tower_items"]
    assert {p["payload"]["model_version"] for p in coll.values()} == {"g1"}
    by_listing = {p["payload"]["listing_id"]: p["vector"] for p in coll.values()}
    als_items = {p["payload"]["listing_id"] for p in qdrant.collections["item_als_vectors__g1"].values()}
    assert "l-cold" in by_listing and "l-cold" not in als_items  # a vector ALS cannot produce
    assert any(by_listing["l-cold"]) and by_listing["l-cold"] != by_listing["la"]
    tt = summary["two_tower"]
    assert tt["decision"] == "promoted"
    assert tt["items"] == 7 and tt["epochs"] == 5 and tt["loss_last"] is not None
    assert tt["features"]["items"]["view"] == "item_popularity"
    (v,) = versions(reg, TWO_TOWER)
    assert reg.aliases(TWO_TOWER) == {"champion": v} and outcome(reg, TWO_TOWER, v) == "promoted"


def test_earlier_generations_are_pruned(tmp_path, reg):
    write_dataset(tmp_path / "d", als_rows())
    feats = cold_item_features(tmp_path)
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    for v in ("g1", "g2", "g3"):
        seed_weak(reg, ALS, TWO_TOWER)
        run(
            _cfg(tmp_path / "d", v, reg, enable_two_tower=True, **feats),
            redis_client=redis,
            qdrant_client=qdrant,
        )
    assert sorted(c for c in qdrant.collections if "two_tower" in c) == [
        "item_two_tower_vectors__g2",
        "item_two_tower_vectors__g3",
    ]


def test_disabled_stage_leaves_the_als_run_unchanged(tmp_path, reg):
    write_dataset(tmp_path / "d", als_rows())
    redis, qdrant = FakeRedis(), FakeQdrantClient()

    summary = run(
        _cfg(tmp_path / "d", "g1", reg), redis_client=redis, qdrant_client=qdrant
    )  # no snapshots needed

    assert "two_tower_items" not in summary and "two_tower" not in summary
    assert not [c for c in qdrant.collections if "two_tower" in c]
    assert TWO_TOWER not in registered_models(reg)


def test_missing_feature_snapshots_stop_the_run_before_anything_is_registered(tmp_path, reg):
    write_dataset(tmp_path / "d", als_rows())
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    with pytest.raises(ConfigError, match="ITEM_FEATURES_DIR"):
        run(
            _cfg(tmp_path / "d", "g1", reg, enable_two_tower=True, item_features_dir=str(tmp_path / "none")),
            redis_client=redis,
            qdrant_client=qdrant,
        )
    assert not registered_models(reg) and not qdrant.collections and not redis.store


def test_a_zero_vector_is_refused_and_never_reaches_qdrant(tmp_path, reg):
    write_dataset(tmp_path / "d", als_rows())
    feats = cold_item_features(tmp_path, extra=[["l-blank", 0, 0, 0, 0, 0, None, 0.0]])
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    # epochs 0: the towers keep their zero bias, so a feature-less item embeds to zero
    summary = run(
        _cfg(tmp_path / "d", "g1", reg, enable_two_tower=True, two_tower_epochs=0, **feats),
        redis_client=redis,
        qdrant_client=qdrant,
    )
    assert summary["two_tower"]["refused"] == 1 and summary["two_tower_items"] == 7
    listed = {p["payload"]["listing_id"] for p in qdrant.collections["item_two_tower_vectors__g1"].values()}
    assert "l-blank" not in listed


def test_a_stage_that_cannot_produce_vectors_rejects_the_candidate_and_publishes_nothing(tmp_path, reg):
    from recsys.two_tower.pipeline import DegenerateEmbeddingError

    write_dataset(tmp_path / "d", als_rows())
    # every catalogue item has all-zero features and the towers are untrained: no usable vector at all
    feats = tower_features(
        tmp_path,
        items=[[f"l{c}", 0, 0, 0, 0, 0, None, 0.0] for c in "abcdef"],
        users=[["u1", 1, 0, 0, 0, 0, 0]],
    )
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    with pytest.raises(DegenerateEmbeddingError):
        run(
            _cfg(tmp_path / "d", "g1", reg, enable_two_tower=True, two_tower_epochs=0, **feats),
            redis_client=redis,
            qdrant_client=qdrant,
        )
    v = newest(reg, ALS)
    assert outcome(reg, ALS, v) == "rejected"
    assert reg.tags(ALS, v)["gate.reason"].startswith("two-tower stage failed")
    assert reg.aliases(ALS) == {}
    assert not qdrant.collections and "recs:v1:serving" not in redis.store
