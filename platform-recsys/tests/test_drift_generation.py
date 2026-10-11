"""Drift of a generation against the one it replaces (recsys.monitoring.generation), no Spark."""

from __future__ import annotations

import json
import random

from recsys.monitoring import generation as gen


def _dist(seed: int, centre: float) -> dict[str, list[float]]:
    rng = random.Random(seed)
    return {
        "weight": gen.sketch([rng.gauss(centre, 1.0) for _ in range(2000)]),
        "top_score": gen.sketch([rng.random() for _ in range(2000)]),
    }


def test_sketch_is_an_equal_mass_summary():
    s = gen.sketch(range(1001))
    assert len(s) == gen.SKETCH_POINTS
    assert s[0] == 0.0 and s[-1] == 1000.0 and s[50] == 500.0
    assert gen.sketch([]) == []
    assert gen.sketch([3.0]) == [3.0] * gen.SKETCH_POINTS


def test_no_baseline_is_reported_not_guessed():
    record, report = gen.compare(None, _dist(1, 5.0), None, 0.25)
    assert record == {"status": "no_baseline", "baseline_version": None, "is_drifted": False}
    assert report is None


def test_the_same_distribution_is_ok():
    base = _dist(1, 5.0)
    record, report = gen.compare(base, _dist(1, 5.0), "als-1", 0.25)
    assert record["status"] == "ok" and record["is_drifted"] is False
    assert record["baseline_version"] == "als-1"
    assert record["max_psi"] < 0.01
    assert set(record["features"]) == {"weight", "top_score"}
    assert report is not None and report.is_drifted is False


def test_a_shifted_feature_is_flagged_with_its_psi_and_level():
    record, report = gen.compare(_dist(1, 5.0), _dist(2, 9.0), "als-1", 0.25)
    assert record["status"] == "drifted" and record["is_drifted"] is True
    assert record["features"]["weight"]["drift_level"] == "significant_drift"
    assert record["features"]["weight"]["psi"] > 0.25
    assert record["features"]["top_score"]["drift_level"] == "no_drift"
    assert record["num_features_drifted"] == 1
    assert record["max_psi"] == record["features"]["weight"]["psi"]


def test_the_threshold_decides_what_is_flagged():
    base, moved = _dist(1, 5.0), _dist(2, 5.4)
    ok, _ = gen.compare(base, moved, "als-1", 10.0)
    flagged, _ = gen.compare(base, moved, "als-1", 0.0)
    assert ok["is_drifted"] is False and flagged["is_drifted"] is True


def test_the_record_and_sketches_are_json_for_the_registry():
    record, _ = gen.compare(_dist(1, 5.0), _dist(2, 9.0), "als-1", 0.25)
    json.dumps(record)
    json.dumps(_dist(3, 1.0))


def test_prometheus_file_is_written_atomically(tmp_path):
    _, report = gen.compare(_dist(1, 5.0), _dist(2, 9.0), "als-1", 0.25)
    path = tmp_path / "recsys_drift.prom"
    gen.write_prometheus(str(path), report, 0.25)
    text = path.read_text()
    assert 'recsys_feature_psi{feature="weight",level="significant_drift"}' in text
    assert "recsys_model_drift_alert 1" in text
    assert not (tmp_path / "recsys_drift.prom.tmp").exists()


def test_top_scores_takes_each_users_best():
    assert gen.top_scores({"u1": [("a", 0.9), ("b", 0.5)], "u2": [], "u3": [("c", 0.2)]}) == [0.9, 0.2]
