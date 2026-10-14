"""Every run is compared with the generation it replaces (needs PySpark: runs in the image).

The baseline is the ``drift_sketches.json`` artifact of the MLflow run behind the ``recsys-als`` champion
(RAT-58, RAT-59); the verdict is recorded as ``drift.*`` params and the metric ``drift_psi_max`` of the run.
"""

from __future__ import annotations

import pytest

pytest.importorskip("pyspark")

from recsys import registry_client as rc  # noqa: E402
from recsys.pipeline import run  # noqa: E402
from tests.dataset_fixture import write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402
from tests.pipeline_fixture import (  # noqa: E402
    ALS,
    als_rows,
    newest,
    outcome,
    requires_spark,
    seed_weak,
    settings,
    versions,
)

pytestmark = requires_spark

FEATURES = {"weight", "user_items", "item_users", "top_score"}


def _run(path, version, reg, **kw):
    return run(settings(path, version, reg, **kw), redis_client=FakeRedis(), qdrant_client=FakeQdrantClient())


def _run_of(reg, version):
    mv = reg.client.get_model_version(ALS, str(version))
    return reg.client.get_run(mv.run_id).data


def test_seeded_champion_no_baseline(tmp_path, reg):
    # RAT-59: the champion is a metrics-only seed (no artifact): no_baseline, gate decision unchanged
    write_dataset(tmp_path, als_rows())
    seeded = seed_weak(reg, ALS)

    summary = _run(tmp_path, "g1", reg)

    assert summary["drift"]["status"] == "no_baseline"
    assert summary["decision"] == "promoted"  # exactly what the gate says without drift
    v = newest(reg, ALS)
    assert v != seeded[ALS] and outcome(reg, ALS, v) == "promoted"
    assert _run_of(reg, v).params["drift.status"] == "no_baseline"
    assert (
        rc.champion_drift_baseline() is not None
    )  # the new champion carries its own sketches for the next run


def test_first_run_has_no_baseline_and_stores_its_distribution(tmp_path, reg):
    write_dataset(tmp_path, als_rows())

    summary = _run(tmp_path, "g1", reg)

    assert summary["drift"]["status"] == "no_baseline"
    (v,) = versions(reg, ALS)
    data = _run_of(reg, v)
    assert data.params["drift.status"] == "no_baseline"
    assert "drift_psi_max" not in data.metrics
    version, sketches = rc.champion_drift_baseline()
    assert version == v and set(sketches) == FEATURES
    assert all(len(sketches[f]) == 101 for f in FEATURES)  # the distribution for the next run


def test_baseline_from_champion_artifact(tmp_path, reg):
    # RAT-58: the second run reads the first run's sketches and names its version as the baseline
    write_dataset(tmp_path, als_rows())
    _run(tmp_path, "g1", reg)
    (first,) = versions(reg, ALS)

    summary = _run(tmp_path, "g2", reg)

    drift = summary["drift"]
    assert str(drift["baseline_version"]) == str(first)
    assert drift["status"] == "ok" and drift["is_drifted"] is False
    assert set(drift["features"]) == FEATURES
    assert all("psi" in drift["features"][f] for f in FEATURES)
    data = _run_of(reg, newest(reg, ALS))
    assert data.params["drift.status"] == "ok" and data.params["drift.baseline_version"] == str(first)
    assert data.metrics["drift_psi_max"] == pytest.approx(drift["max_psi"])
    for f in FEATURES:  # per-feature PSI and level are recorded on the run
        assert any(k.startswith("drift.") and f in k for k in data.params), f


def test_a_drifted_run_is_flagged_and_still_decided_by_the_gate(tmp_path, reg):
    first, second = tmp_path / "a", tmp_path / "b"
    write_dataset(first, als_rows())
    write_dataset(second, als_rows(weight_scale=40.0))
    _run(first, "g1", reg)

    summary = _run(second, "g2", reg)

    assert summary["drift"]["is_drifted"] is True
    assert summary["drift"]["features"]["weight"]["drift_level"] == "significant_drift"
    v = newest(reg, ALS)
    assert outcome(reg, ALS, v) == summary["decision"]  # drift is a signal, not a gate: the gate decided


def test_a_zero_threshold_flags_every_feature_and_does_not_change_the_decision(tmp_path, reg):
    write_dataset(tmp_path, als_rows())
    _run(tmp_path, "g1", reg)

    summary = _run(tmp_path, "g2", reg, drift_alert_threshold=0.0)

    drift = summary["drift"]
    assert drift["is_drifted"] is True and drift["num_features_drifted"] == len(FEATURES)
    v = newest(reg, ALS)
    assert _run_of(reg, v).params["drift.status"] == drift["status"] == "drifted"
    assert outcome(reg, ALS, v) == summary["decision"]  # same data, equal metrics: the gate's own verdict


def test_the_prometheus_file_is_written_when_configured(tmp_path, reg):
    write_dataset(tmp_path / "d", als_rows())
    prom = tmp_path / "drift.prom"
    _run(tmp_path / "d", "g1", reg, drift_metrics_path=str(prom))
    assert not prom.exists()  # nothing to compare with yet
    _run(tmp_path / "d", "g2", reg, drift_metrics_path=str(prom))
    assert "recsys_feature_psi" in prom.read_text()
