"""Drift of one generation's training distribution against the generation it replaces.

Each run summarises what it trained and predicted as a quantile sketch per feature (equally spaced
quantiles: an equal-mass sample of the distribution) and stores the sketches on its model metadata
(``parameters["distribution"]``). The next run computes PSI of its own sketches against the champion's
(``DriftDetector``) and records the verdict on the candidate (``parameters["drift"]``, metric
``drift_psi_max``), in its summary, in the log and, when ``DRIFT_METRICS_PATH`` is set, as a Prometheus
text file. Drift is observational: it never rejects or blocks a candidate (the structural and metric
gates decide that).

Features: what the model was trained on (``weight`` of the dataset pairs, items per user, users per item)
and what it predicts (``top_score``: each user's best recommendation score). Factor values themselves are
not compared: ALS factors of two runs are only defined up to a rotation.
"""

from __future__ import annotations

import os
from collections.abc import Sequence

from recsys.monitoring.drift import DriftDetector, DriftReport

SKETCH_POINTS = 101  # quantiles 0, 0.01 ... 1
FEATURES = ("weight", "user_items", "item_users", "top_score")


def sketch(values: Sequence[float], points: int = SKETCH_POINTS) -> list[float]:
    """``points`` equally spaced quantiles of ``values`` (empty when there are none)."""
    ordered = sorted(float(v) for v in values)
    if not ordered:
        return []
    last = len(ordered) - 1
    return [round(ordered[round(i * last / (points - 1))], 6) for i in range(points)]


def top_scores(user_recs: dict[str, list[tuple[str, float]]]) -> list[float]:
    return [float(recs[0][1]) for recs in user_recs.values() if recs]


def spark_distributions(triples, user_recs: dict[str, list[tuple[str, float]]]) -> dict[str, list[float]]:
    """The run's feature sketches. The dataset side runs in Spark (approximate quantiles), only the
    sketches reach the driver."""
    probs = [i / (SKETCH_POINTS - 1) for i in range(SKETCH_POINTS)]

    def quantiles(frame, column: str) -> list[float]:
        return [round(float(q), 6) for q in frame.approxQuantile(column, probs, 0.001)]

    user_items = triples.groupBy("user_key").count()
    item_users = triples.groupBy("listing_id").count()
    return {
        "weight": quantiles(triples, "weight"),
        "user_items": quantiles(user_items, "count"),
        "item_users": quantiles(item_users, "count"),
        "top_score": sketch(top_scores(user_recs)),
    }


def compare(
    baseline: dict[str, list[float]] | None,
    current: dict[str, list[float]],
    baseline_version: str | None,
    alert_threshold: float,
) -> tuple[dict, DriftReport | None]:
    """The drift record of ``current`` against ``baseline``: (record, report).

    ``record`` is what goes on the model metadata and in the summary: status ``no_baseline`` (the
    first run, or a champion registered before drift monitoring), ``ok`` or ``drifted``.
    """
    if not baseline:
        return {"status": "no_baseline", "baseline_version": baseline_version, "is_drifted": False}, None
    report = DriftDetector(alert_threshold=alert_threshold).evaluate(baseline, current)
    record = {
        "status": "drifted" if report.is_drifted else "ok",
        "baseline_version": baseline_version,
        "threshold": alert_threshold,
        "is_drifted": report.is_drifted,
        "num_features_drifted": report.num_features_drifted,
        "max_psi": round(max((r.psi for r in report.feature_results.values()), default=0.0), 4),
        "features": {
            name: {"psi": round(res.psi, 4), "drift_level": res.drift_level.value}
            for name, res in report.feature_results.items()
        },
    }
    return record, report


def write_prometheus(path: str, report: DriftReport, alert_threshold: float) -> None:
    """Write the report in Prometheus text format (node-exporter textfile collector), atomically."""
    text = DriftDetector(alert_threshold=alert_threshold).to_prometheus_metrics(report)
    tmp = f"{path}.tmp"
    with open(tmp, "w") as f:
        f.write(text)
    os.replace(tmp, path)
