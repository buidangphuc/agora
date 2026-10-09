"""The pipeline publishes generations and applies the structural gate (needs PySpark: runs in the image)."""

from __future__ import annotations

from datetime import datetime, timedelta, timezone

import pytest

pytest.importorskip("pyspark")

from recsys.config import Settings  # noqa: E402
from recsys.pipeline import run  # noqa: E402
from recsys.registry.registry import ModelRegistry  # noqa: E402
from tests.dataset_fixture import write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402


def _clustered_rows():
    """Two taste clusters over six listings: lists differ across users, catalogue is well covered."""
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
                {
                    "user_key": u,
                    "listing_id": f"l{ch}",
                    "weight": 3.0 - h,
                    "interactions": 1,
                    "last_occurred_at": now - timedelta(hours=h + 1),
                }
            )
    return rows


def _one_size_fits_all_rows():
    now = datetime.now(timezone.utc)
    return [
        {
            "user_key": f"u{u}",
            "listing_id": lid,
            "weight": 1.0,
            "interactions": 1,
            "last_occurred_at": now - timedelta(hours=h + 1),
        }
        for u in range(1, 5)
        for h, lid in enumerate(["l1", "l2", "l3"])
    ]


def _settings(tmp_path, version, **kw):
    return Settings(
        spark_master="local[1]",
        dataset_dir=str(tmp_path),
        als_max_iter=3,
        als_rank=4,
        top_n=2,
        model_version=version,
        promotion_force=True,
        **kw,
    )


def _gens(fake_r):
    return {k.split(":")[3] for k in fake_r.store if k.startswith("recs:v1:gen:")}


def test_successive_promotions_keep_two_generations_and_move_the_pointers(tmp_path):
    write_dataset(tmp_path, _clustered_rows())
    registry = ModelRegistry()
    fake_r, fake_q = FakeRedis(), FakeQdrantClient()

    summaries = [
        run(_settings(tmp_path, v), registry=registry, redis_client=fake_r, qdrant_client=fake_q)
        for v in ("g1", "g2", "g3")
    ]

    assert [s["decision"] for s in summaries] == ["promoted"] * 3
    assert summaries[1]["serving"] == "g2" and summaries[1]["previous"] == "g1"
    assert fake_r.store["recs:v1:serving"] == "g3"
    assert fake_r.store["recs:v1:previous"] == "g2"
    assert fake_r.store["recs:v1:model_version"] == "g3"
    assert _gens(fake_r) == {"g2", "g3"}
    assert fake_q.aliases["item_als_vectors"] == "item_als_vectors__g3"
    assert set(fake_q.collections) == {
        "item_als_vectors__g2",
        "item_als_vectors__g3",
        "user_als_vectors__g2",
        "user_als_vectors__g3",
    }


def test_one_size_fits_all_candidate_is_rejected_and_serving_is_unchanged(tmp_path):
    registry = ModelRegistry()
    fake_r, fake_q = FakeRedis(), FakeQdrantClient()
    good = tmp_path / "good"
    write_dataset(good, _clustered_rows())
    run(_settings(good, "g1"), registry=registry, redis_client=fake_r, qdrant_client=fake_q)
    before = (dict(fake_r.store), dict(fake_q.aliases), set(fake_q.collections))

    bad = tmp_path / "bad"
    write_dataset(bad, _one_size_fits_all_rows())
    settings = Settings(
        spark_master="local[1]",
        dataset_dir=str(bad),
        als_max_iter=3,
        als_rank=4,
        top_n=5,
        model_version="g2-degenerate",
        promotion_force=True,  # the structural gate is not an operator-overridable metric gate
    )
    summary = run(settings, registry=registry, redis_client=fake_r, qdrant_client=fake_q)

    assert summary["decision"] == "rejected" and summary["gate"] == "structural"
    assert "list overlap" in summary["reason"] or "item coverage" in summary["reason"]
    rejected = registry.get_model("g2-degenerate")
    assert rejected.status == "rejected"
    assert rejected.metrics["gate_reason"] == summary["reason"]
    assert rejected.parameters["gate_reason"] == summary["reason"]
    assert registry.get_champion_version() == "g1"
    after = (dict(fake_r.store), dict(fake_q.aliases), set(fake_q.collections))
    assert after == before


class _AliasFailingQdrant(FakeQdrantClient):
    def update_collection_aliases(self, change_aliases_operations):
        raise RuntimeError("qdrant unreachable")


def test_failed_publish_keeps_the_previous_champion_and_pointers(tmp_path):
    write_dataset(tmp_path, _clustered_rows())
    registry = ModelRegistry()
    fake_r, fake_q = FakeRedis(), FakeQdrantClient()
    run(_settings(tmp_path, "g1"), registry=registry, redis_client=fake_r, qdrant_client=fake_q)
    pointers = {k: v for k, v in fake_r.store.items() if k.startswith("recs:v1:") and ":gen:" not in k}
    aliases = dict(fake_q.aliases)

    broken = _AliasFailingQdrant()
    broken.collections, broken.aliases, broken.dims = fake_q.collections, fake_q.aliases, fake_q.dims
    with pytest.raises(RuntimeError, match="qdrant unreachable"):
        run(_settings(tmp_path, "g2"), registry=registry, redis_client=fake_r, qdrant_client=broken)

    assert registry.get_champion_version() == "g1"
    assert registry.get_model("g1").status == "champion"
    rejected = registry.get_model("g2")
    assert rejected.status == "rejected"
    assert rejected.metrics["gate_reason"] == "publish failed: RuntimeError"
    assert rejected.parameters["gate_reason"] == "publish failed: RuntimeError"
    assert {k: v for k, v in fake_r.store.items() if k.startswith("recs:v1:") and ":gen:" not in k} == pointers
    assert fake_q.aliases == aliases
