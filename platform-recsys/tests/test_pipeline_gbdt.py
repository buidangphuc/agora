"""The GBDT stage inside pipeline.run (needs PySpark: runs in the image)."""

from __future__ import annotations

import json
from datetime import datetime, timedelta, timezone

import pytest

pytest.importorskip("pyspark")

from recsys.config import ConfigError, Settings  # noqa: E402
from recsys.pipeline import run  # noqa: E402
from recsys.ranker import gbdt  # noqa: E402
from recsys.registry.registry import GBDT_CHAMPION_KEY, ModelRegistry  # noqa: E402
from tests import ranker_fixture as fx  # noqa: E402
from tests.dataset_fixture import write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402


def _als_rows():
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
            rows.append({"user_key": u, "listing_id": f"l{ch}", "weight": 3.0 - h, "interactions": 1,
                         "last_occurred_at": now - timedelta(hours=h + 1)})  # fmt: skip
    return rows


def _run(tmp_path, version, registry, redis, qdrant, **kw):
    write_dataset(tmp_path / "als", _als_rows())
    values = fx.write_all(tmp_path)
    settings = Settings(
        spark_master="local[1]", dataset_dir=str(tmp_path / "als"), als_max_iter=3, als_rank=4, top_n=2,
        model_version=version, enable_gbdt=True, gbdt_trees=20, gbdt_ctr_min_impressions=10**6,
        **{**values, **kw},
    )  # fmt: skip
    return run(settings, registry=registry, redis_client=redis, qdrant_client=qdrant)


def test_a_promoted_ranker_ships_inside_the_generation(tmp_path):
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()
    summary = _run(tmp_path, "g1", registry, redis, qdrant, promotion_force=True)
    assert summary["decision"] == "promoted" and summary["gbdt"]["decision"] == "promoted"
    assert summary["gbdt"]["ranker_key"] == "recs:v1:gen:g1:ranker"
    art = json.loads(redis.get("recs:v1:gen:g1:ranker"))
    assert art["generation"] == "g1" and gbdt.score_vector(art, [0.0] * 8) is not None
    assert redis.get("recs:v1:serving") == "g1"
    assert registry.get_champion_version() == "g1"  # the ALS champion
    assert registry.scoped(GBDT_CHAMPION_KEY).get_champion_version() == "gbdt-g1"


def test_a_rejected_ranker_does_not_block_the_generation(tmp_path):
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()
    summary = _run(tmp_path, "g1", registry, redis, qdrant, promotion_min_relative_improvement=10.0)
    assert summary["decision"] == "promoted" and summary["gbdt"]["decision"] == "rejected"
    assert redis.get("recs:v1:serving") == "g1" and redis.get("recs:v1:gen:g1:ranker") is None
    assert registry.get_model("gbdt-g1").status == "rejected"
    assert registry.scoped(GBDT_CHAMPION_KEY).get_champion_version() is None


def test_disabled_trainer_leaves_the_run_unchanged(tmp_path):
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()
    write_dataset(tmp_path / "als", _als_rows())
    settings = Settings(
        spark_master="local[1]",
        dataset_dir=str(tmp_path / "als"),
        als_max_iter=3,
        als_rank=4,
        top_n=2,
        model_version="g1",
        promotion_force=True,
    )
    summary = run(settings, registry=registry, redis_client=redis, qdrant_client=qdrant)
    assert "gbdt" not in summary and not [k for k in redis.store if k.endswith(":ranker")]
    assert registry.get_model("gbdt-g1") is None


def test_a_missing_ranking_dataset_stops_the_run_before_anything_is_registered(tmp_path):
    registry, redis, qdrant = ModelRegistry(), FakeRedis(), FakeQdrantClient()
    with pytest.raises(ConfigError, match="RANK_DATASET_DIR"):
        _run(tmp_path, "g1", registry, redis, qdrant, rank_dataset_dir=str(tmp_path / "none"))
    assert registry.get_model("g1") is None and not qdrant.collections and not redis.store
