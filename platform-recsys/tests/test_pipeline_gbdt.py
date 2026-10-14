"""The GBDT stage inside pipeline.run (needs PySpark: runs in the image).

Ported to the MLflow registry: the GBDT champion is the ``recsys-gbdt@champion`` alias (own policy).
"""

from __future__ import annotations

import json

import pytest

pytest.importorskip("pyspark")

from recsys.config import ConfigError  # noqa: E402
from recsys.pipeline import run  # noqa: E402
from recsys.ranker import gbdt  # noqa: E402
from tests import ranker_fixture as fx  # noqa: E402
from tests.dataset_fixture import write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402
from tests.pipeline_fixture import (  # noqa: E402
    ALS,
    GBDT,
    als_rows,
    newest,
    outcome,
    registered_models,
    requires_spark,
    seed_strong,
    seed_weak,
    settings,
    versions,
)

pytestmark = requires_spark


def _cfg(tmp_path, reg, version="g1", **kw):
    write_dataset(tmp_path / "als", als_rows())
    values = fx.write_all(tmp_path)
    return settings(
        tmp_path / "als", version, reg, enable_gbdt=True, gbdt_trees=20, gbdt_ctr_min_impressions=10**6,
        **{**values, **kw},
    )  # fmt: skip


def test_a_promoted_ranker_ships_inside_the_generation(tmp_path, reg):
    redis, qdrant = FakeRedis(), FakeQdrantClient()

    summary = run(_cfg(tmp_path, reg), redis_client=redis, qdrant_client=qdrant)

    assert summary["decision"] == "promoted" and summary["gbdt"]["decision"] == "promoted"
    assert summary["gbdt"]["ranker_key"] == "recs:v1:gen:g1:ranker"
    art = json.loads(redis.get("recs:v1:gen:g1:ranker"))
    assert art["generation"] == "g1" and gbdt.score_vector(art, [0.0] * 8) is not None
    assert redis.get("recs:v1:serving") == "g1"
    assert reg.aliases(ALS) == {"champion": newest(reg, ALS)}  # the ALS champion
    (v,) = versions(reg, GBDT)  # the GBDT champion is its own model's, never the ALS one's
    assert reg.aliases(GBDT) == {"champion": v} and outcome(reg, GBDT, v) == "promoted"


def test_a_rejected_ranker_does_not_block_the_generation(tmp_path, reg):
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    old = seed_strong(reg, GBDT)  # an ndcg@10 no model can reach

    summary = run(_cfg(tmp_path, reg), redis_client=redis, qdrant_client=qdrant)

    assert summary["decision"] == "promoted" and summary["gbdt"]["decision"] == "rejected"
    assert redis.get("recs:v1:serving") == "g1" and redis.get("recs:v1:gen:g1:ranker") is None
    rejected = newest(reg, GBDT)
    assert rejected != old[GBDT] and outcome(reg, GBDT, rejected) == "rejected"
    assert reg.tags(GBDT, rejected)["gate.reason"]
    assert reg.aliases(GBDT) == {"champion": old[GBDT]}


def test_a_weak_champion_is_replaced_and_kept_as_previous(tmp_path, reg):
    old = seed_weak(reg, GBDT)

    run(_cfg(tmp_path, reg), redis_client=FakeRedis(), qdrant_client=FakeQdrantClient())

    assert reg.aliases(GBDT) == {"champion": newest(reg, GBDT), "previous": old[GBDT]}


def test_disabled_trainer_leaves_the_run_unchanged(tmp_path, reg):
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    write_dataset(tmp_path / "als", als_rows())

    summary = run(settings(tmp_path / "als", "g1", reg), redis_client=redis, qdrant_client=qdrant)

    assert "gbdt" not in summary and not [k for k in redis.store if k.endswith(":ranker")]
    assert GBDT not in registered_models(reg)


def test_a_missing_ranking_dataset_stops_the_run_before_anything_is_registered(tmp_path, reg):
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    with pytest.raises(ConfigError, match="RANK_DATASET_DIR"):
        run(
            _cfg(tmp_path, reg, rank_dataset_dir=str(tmp_path / "none")),
            redis_client=redis,
            qdrant_client=qdrant,
        )
    assert not registered_models(reg) and not qdrant.collections and not redis.store
