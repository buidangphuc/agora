"""Evaluation metrics for time-series probabilistic forecasting."""

from __future__ import annotations

import numpy as np


def compute_wape(actuals: list[float] | np.ndarray, predictions: list[float] | np.ndarray) -> float:
    """Computes Weighted Absolute Percentage Error (WAPE = sum(|y - y_hat|) / sum(y))."""
    y = np.array(actuals, dtype=float)
    y_hat = np.array(predictions, dtype=float)
    total_actual = float(np.sum(y))
    if total_actual == 0:
        return 0.0 if float(np.sum(y_hat)) == 0 else 1.0
    return float(np.sum(np.abs(y - y_hat)) / total_actual)


def compute_pinball_loss(actuals: list[float] | np.ndarray, predictions: list[float] | np.ndarray, q: float) -> float:
    """Computes pinball (quantile) loss for quantile q in (0, 1)."""
    y = np.array(actuals, dtype=float)
    y_hat = np.array(predictions, dtype=float)
    diff = y - y_hat
    loss = np.maximum(q * diff, (q - 1.0) * diff)
    return float(np.mean(loss))


def compute_interval_coverage(
    actuals: list[float] | np.ndarray,
    p10: list[float] | np.ndarray,
    p90: list[float] | np.ndarray,
) -> float:
    """Computes empirical coverage probability for the 80% prediction interval [p10, p90]."""
    y = np.array(actuals, dtype=float)
    lower = np.array(p10, dtype=float)
    upper = np.array(p90, dtype=float)
    covered = (y >= lower) & (y <= upper)
    return float(np.mean(covered))
