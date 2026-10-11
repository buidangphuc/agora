"""Every run is compared with the generation it replaces (needs PySpark: runs in the image)."""

from __future__ import annotations

from datetime import datetime, timedelta, timezone

import pytest

pytest.importorskip("pyspark")

from recsys.config import Settings  # noqa: E402
from recsys.pipeline import run  # noqa: E402
from recsys.registry.registry import ModelRegistry  # noqa: E402
from tests.dataset_fixture import write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402


def _rows(weight_scale: float = 1.0):
    """Two taste clusters over six listings; ``weight_scale`` rescales every pair's weight."""
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
                    "weight": (3.0 - h) * weight_scale + 0.01 * int(u[1:]),
                    "interactions": 1,
                    "last_occurred_at": now - timedelta(hours=h + 1),
                }
            )
    return rows


def _settings(path, version, **kw):
    return Settings(
        spark_master="local[1]",
        dataset_dir=str(path),
        als_max_iter=3,
        als_rank=4,
        top_n=2,
        model_version=version,
        promotion_force=True,
        **kw,
    )


def _run(path, version, registry, **kw):
    return run(
        _settings(path, version, **kw),
        registry=registry,
        redis_client=FakeRedis(),
        qdrant_client=FakeQdrantClient(),
    )


def test_first_run_has_no_baseline_and_stores_its_distribution(tmp_path):
    write_dataset(tmp_path, _rows())
    registry = ModelRegistry()
    summary = _run(tmp_path, "g1", registry)

    assert summary["drift"]["status"] == "no_baseline"
    meta = registry.get_model("g1")
    assert meta.parameters["drift"]["status"] == "no_baseline"
    assert set(meta.parameters["distribution"]) == {"weight", "user_items", "item_users", "top_score"}
    assert all(meta.parameters["distribution"][f] for f in meta.parameters["distribution"])
    assert "drift_psi_max" not in meta.metrics


def test_second_run_is_compared_with_the_champion_it_replaces(tmp_path):
    write_dataset(tmp_path, _rows())
    registry = ModelRegistry()
    _run(tmp_path, "g1", registry)
    summary = _run(tmp_path, "g2", registry)

    drift = summary["drift"]
    assert drift["baseline_version"] == "g1"
    assert drift["status"] == "ok" and drift["is_drifted"] is False
    assert set(drift["features"]) == {"weight", "user_items", "item_users", "top_score"}
    meta = registry.get_model("g2")
    assert meta.parameters["drift"] == drift
    assert meta.metrics["drift_psi_max"] == drift["max_psi"]


def test_a_drifted_run_is_flagged_and_still_decided_by_the_gate(tmp_path):
    first, second = tmp_path / "a", tmp_path / "b"
    write_dataset(first, _rows())
    write_dataset(second, _rows(weight_scale=40.0))
    registry = ModelRegistry()
    _run(first, "g1", registry)
    summary = _run(second, "g2", registry)

    assert summary["drift"]["is_drifted"] is True
    assert summary["drift"]["features"]["weight"]["drift_level"] == "significant_drift"
    assert summary["decision"] == "promoted"  # drift is a signal, not a gate
    assert registry.get_model("g2").status == "champion"


def test_the_prometheus_file_is_written_when_configured(tmp_path):
    write_dataset(tmp_path / "d", _rows())
    registry = ModelRegistry()
    prom = tmp_path / "drift.prom"
    _run(tmp_path / "d", "g1", registry, drift_metrics_path=str(prom))
    assert not prom.exists()  # nothing to compare with yet
    _run(tmp_path / "d", "g2", registry, drift_metrics_path=str(prom))
    assert "recsys_feature_psi" in prom.read_text()
