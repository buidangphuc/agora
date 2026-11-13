"""Time-series feature engineering for demand forecasting."""

from __future__ import annotations

import datetime as dt

import numpy as np

FEATURE_COLS = [
    "lag_1",
    "lag_7",
    "lag_14",
    "rolling_mean_7",
    "rolling_std_7",
    "rolling_mean_14",
    "day_of_week",
]


def feature_vector(history: np.ndarray, day: dt.date) -> list[float]:
    """Features for `day` computed only from the daily values before it."""
    n = len(history)

    def lag(k: int) -> float:
        return float(history[n - k]) if n >= k else 0.0

    last7 = history[-7:] if n else np.zeros(1)
    last14 = history[-14:] if n else np.zeros(1)
    return [
        lag(1),
        lag(7),
        lag(14),
        float(np.mean(last7)),
        float(np.std(last7)),
        float(np.mean(last14)),
        float(day.weekday()),
    ]


def training_rows(
    history: np.ndarray, end_date: dt.date, min_history: int
) -> tuple[list[list[float]], list[float]]:
    """One row per day from `min_history` on; `end_date` is the day after the last value."""
    n = len(history)
    rows: list[list[float]] = []
    targets: list[float] = []
    for t in range(min(min_history, n), n):
        day = end_date - dt.timedelta(days=n - t)
        rows.append(feature_vector(history[:t], day))
        targets.append(float(history[t]))
    return rows, targets
