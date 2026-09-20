"""Feature and prediction drift monitoring using Population Stability Index (PSI)."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
import math
from typing import Any, Sequence


class DriftLevel(str, Enum):
    NO_DRIFT = "no_drift"  # PSI < 0.1
    MODERATE_DRIFT = "moderate_drift"  # 0.1 <= PSI < 0.25
    SIGNIFICANT_DRIFT = "significant_drift"  # PSI >= 0.25


@dataclass
class FeatureDriftResult:
    feature_name: str
    psi: float
    drift_level: DriftLevel
    baseline_count: int
    target_count: int


@dataclass
class DriftReport:
    is_drifted: bool
    num_features_drifted: int
    feature_results: dict[str, FeatureDriftResult] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "is_drifted": self.is_drifted,
            "num_features_drifted": self.num_features_drifted,
            "features": {
                k: {
                    "psi": round(v.psi, 4),
                    "drift_level": v.drift_level.value,
                    "baseline_count": v.baseline_count,
                    "target_count": v.target_count,
                }
                for k, v in self.feature_results.items()
            },
        }


def _classify_drift(psi: float) -> DriftLevel:
    if psi >= 0.25:
        return DriftLevel.SIGNIFICANT_DRIFT
    elif psi >= 0.1:
        return DriftLevel.MODERATE_DRIFT
    return DriftLevel.NO_DRIFT


def calculate_categorical_psi(
    baseline: Sequence[str],
    target: Sequence[str],
    epsilon: float = 1e-4,
) -> float:
    """Calculates PSI for categorical feature arrays."""
    if not baseline or not target:
        return 0.0

    all_categories = sorted(list(set(baseline) | set(target)))
    b_len = float(len(baseline))
    t_len = float(len(target))

    b_counts: dict[str, int] = {}
    for x in baseline:
        b_counts[x] = b_counts.get(x, 0) + 1

    t_counts: dict[str, int] = {}
    for x in target:
        t_counts[x] = t_counts.get(x, 0) + 1

    psi = 0.0
    for cat in all_categories:
        p = max(epsilon, b_counts.get(cat, 0) / b_len)
        q = max(epsilon, t_counts.get(cat, 0) / t_len)
        psi += (q - p) * math.log(q / p)

    return max(0.0, psi)


def calculate_numerical_psi(
    baseline: Sequence[float],
    target: Sequence[float],
    num_bins: int = 10,
    epsilon: float = 1e-4,
) -> float:
    """Calculates PSI for continuous numerical feature arrays using quantile binning."""
    if not baseline or not target:
        return 0.0

    b_sorted = sorted(baseline)
    b_len = len(b_sorted)
    t_len = len(target)

    # Bin boundaries from baseline quantiles
    bin_edges = [b_sorted[0]]
    for i in range(1, num_bins):
        idx = int(i * b_len / num_bins)
        idx = min(idx, b_len - 1)
        bin_edges.append(b_sorted[idx])
    bin_edges.append(b_sorted[-1])

    # Remove duplicates and ensure monotonic
    unique_edges = [bin_edges[0]]
    for e in bin_edges[1:]:
        if e > unique_edges[-1]:
            unique_edges.append(e)

    if len(unique_edges) <= 2:
        # Fallback to categorical if few unique values
        return calculate_categorical_psi(
            [str(x) for x in baseline],
            [str(x) for x in target],
            epsilon=epsilon,
        )

    # Count frequencies
    num_intervals = len(unique_edges) - 1
    b_bin_counts = [0] * num_intervals
    t_bin_counts = [0] * num_intervals

    for x in baseline:
        placed = False
        for i in range(num_intervals):
            if (i == 0 and x <= unique_edges[1]) or (
                unique_edges[i] < x <= unique_edges[i + 1]
            ) or (i == num_intervals - 1 and x >= unique_edges[i]):
                b_bin_counts[i] += 1
                placed = True
                break
        if not placed:
            b_bin_counts[-1] += 1

    for x in target:
        placed = False
        for i in range(num_intervals):
            if (i == 0 and x <= unique_edges[1]) or (
                unique_edges[i] < x <= unique_edges[i + 1]
            ) or (i == num_intervals - 1 and x >= unique_edges[i]):
                t_bin_counts[i] += 1
                placed = True
                break
        if not placed:
            t_bin_counts[-1] += 1

    psi = 0.0
    for b_c, t_c in zip(b_bin_counts, t_bin_counts):
        p = max(epsilon, b_c / float(b_len))
        q = max(epsilon, t_c / float(t_len))
        psi += (q - p) * math.log(q / p)

    return max(0.0, psi)


def calculate_psi(
    baseline: Sequence[Any],
    target: Sequence[Any],
    num_bins: int = 10,
    epsilon: float = 1e-4,
) -> float:
    """Dispatches to numerical or categorical PSI calculation based on element types."""
    if not baseline or not target:
        return 0.0

    sample = baseline[0]
    if isinstance(sample, (int, float)) and not isinstance(sample, bool):
        return calculate_numerical_psi(
            [float(x) for x in baseline],
            [float(x) for x in target],
            num_bins=num_bins,
            epsilon=epsilon,
        )
    return calculate_categorical_psi(
        [str(x) for x in baseline],
        [str(x) for x in target],
        epsilon=epsilon,
    )


class DriftDetector:
    """Monitors multi-feature distributions for drift against historical baselines."""

    def __init__(self, alert_threshold: float = 0.25):
        self.alert_threshold = alert_threshold

    def evaluate(
        self,
        baseline_data: dict[str, Sequence[Any]],
        target_data: dict[str, Sequence[Any]],
    ) -> DriftReport:
        results: dict[str, FeatureDriftResult] = {}
        drifted_count = 0

        for feature_name, b_vals in baseline_data.items():
            t_vals = target_data.get(feature_name, [])
            if not b_vals or not t_vals:
                continue

            psi = calculate_psi(b_vals, t_vals)
            level = _classify_drift(psi)
            if psi >= self.alert_threshold:
                drifted_count += 1

            results[feature_name] = FeatureDriftResult(
                feature_name=feature_name,
                psi=psi,
                drift_level=level,
                baseline_count=len(b_vals),
                target_count=len(t_vals),
            )

        return DriftReport(
            is_drifted=drifted_count > 0,
            num_features_drifted=drifted_count,
            feature_results=results,
        )

    def to_prometheus_metrics(self, report: DriftReport) -> str:
        """Formats drift metrics in Prometheus text exposition format."""
        lines = [
            "# HELP recsys_feature_psi Population Stability Index per feature",
            "# TYPE recsys_feature_psi gauge",
        ]
        for name, res in report.feature_results.items():
            lines.append(f'recsys_feature_psi{{feature="{name}",level="{res.drift_level.value}"}} {res.psi:.4f}')

        lines.extend([
            "# HELP recsys_model_drift_alert Indicates if any feature has significant drift",
            "# TYPE recsys_model_drift_alert gauge",
            f"recsys_model_drift_alert {1 if report.is_drifted else 0}",
        ])
        return "\n".join(lines) + "\n"
