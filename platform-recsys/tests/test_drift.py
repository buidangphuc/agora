"""Unit tests for feature and prediction drift monitoring."""

import random

from recsys.monitoring.drift import (
    DriftDetector,
    DriftLevel,
    calculate_categorical_psi,
    calculate_numerical_psi,
    calculate_psi,
)


def test_categorical_psi_identical_vs_drifted():
    base = ["electronics"] * 50 + ["fashion"] * 50
    target_same = ["electronics"] * 50 + ["fashion"] * 50
    psi_same = calculate_categorical_psi(base, target_same)
    assert psi_same < 0.01

    target_shifted = ["electronics"] * 95 + ["fashion"] * 5
    psi_shifted = calculate_categorical_psi(base, target_shifted)
    assert psi_shifted > 0.25


def test_numerical_psi_identical_vs_drifted():
    rng = random.Random(42)
    base = [rng.gauss(50, 10) for _ in range(500)]
    target_same = [rng.gauss(50, 10) for _ in range(500)]

    psi_same = calculate_numerical_psi(base, target_same, num_bins=5)
    assert psi_same < 0.1

    target_shifted = [rng.gauss(80, 10) for _ in range(500)]
    psi_shifted = calculate_numerical_psi(base, target_shifted, num_bins=5)
    assert psi_shifted > 0.25


def test_drift_detector_multi_feature_and_prometheus():
    detector = DriftDetector(alert_threshold=0.25)

    baseline_data = {
        "category": ["electronics"] * 100 + ["fashion"] * 100,
        "price": [10.0 + i for i in range(100)],
    }

    # category is identical, price is drastically shifted
    target_data = {
        "category": ["electronics"] * 100 + ["fashion"] * 100,
        "price": [500.0 + i for i in range(100)],
    }

    report = detector.evaluate(baseline_data, target_data)
    assert report.is_drifted is True
    assert report.num_features_drifted == 1

    cat_res = report.feature_results["category"]
    assert cat_res.drift_level == DriftLevel.NO_DRIFT

    price_res = report.feature_results["price"]
    assert price_res.drift_level == DriftLevel.SIGNIFICANT_DRIFT

    prom = detector.to_prometheus_metrics(report)
    assert "recsys_feature_psi" in prom
    assert 'feature="category"' in prom
    assert 'feature="price"' in prom
    assert "recsys_model_drift_alert 1" in prom
