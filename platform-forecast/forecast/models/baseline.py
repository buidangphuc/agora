"""Seasonal-Naive and EWMA baseline quantile forecasters."""

from __future__ import annotations

import datetime
from typing import Any

import numpy as np
import pandas as pd


class SeasonalNaiveQuantileForecaster:
    """Seasonal-Naive baseline: predicts future day t as day (t - 7) with empirical residual variance."""

    def __init__(self, season_length: int = 7) -> None:
        self.season_length = season_length

    def forecast_series(
        self,
        series: pd.Series,
        horizon: int = 28,
        quantiles: list[float] | None = None,
    ) -> dict[str, list[float]]:
        """Forecasts p10, p50, p90 for horizon steps."""
        quantiles = quantiles or [0.10, 0.50, 0.90]
        values = series.values
        if len(values) == 0:
            zeros = [0.0] * horizon
            return {f"p{int(q*100)}": zeros for q in quantiles}

        # Use recent 28 days to estimate residual variance and seasonal cycle
        recent_window = values[-min(len(values), 28) :]
        if len(recent_window) < self.season_length:
            mean_val = float(np.mean(recent_window))
            std_val = float(np.std(recent_window)) if len(recent_window) > 1 else max(1.0, mean_val * 0.3)
            base_pattern = [mean_val] * horizon
        else:
            base_pattern = []
            for h in range(horizon):
                lag_idx = -(self.season_length - (h % self.season_length))
                base_pattern.append(float(recent_window[lag_idx]))
            std_val = float(np.std(recent_window)) if len(recent_window) > 1 else 1.0

        std_val = max(0.5, std_val)
        out: dict[str, list[float]] = {}
        for q in quantiles:
            # Normal distribution z-score approximation
            if q == 0.10:
                z = -1.28
            elif q == 0.50:
                z = 0.0
            elif q == 0.90:
                z = 1.28
            else:
                z = 0.0
            q_preds = [max(0.0, round(float(b + z * std_val), 2)) for b in base_pattern]
            out[f"p{int(q*100)}"] = q_preds

        return out
