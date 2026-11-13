"""Global multi-quantile LightGBM model for demand forecasting."""

from __future__ import annotations

import datetime as dt

import lightgbm as lgb
import numpy as np

from forecast.features import feature_vector, training_rows

MODEL_VERSION = "lgbm_quantile_v1"
QUANTILES = (0.10, 0.50, 0.90)
SEED = 42


class GlobalQuantileLGBM:
    """One LightGBM model per quantile, trained on all listings pooled."""

    def __init__(self, min_history: int = 14) -> None:
        self.min_history = min_history
        self.models: dict[float, lgb.Booster] = {}

    def fit(self, histories: list[np.ndarray], end_date: dt.date) -> "GlobalQuantileLGBM":
        rows: list[list[float]] = []
        targets: list[float] = []
        for history in histories:
            r, y = training_rows(history, end_date, self.min_history)
            rows.extend(r)
            targets.extend(y)
        x = np.asarray(rows, dtype=float)
        y_arr = np.asarray(targets, dtype=float)
        for q in QUANTILES:
            params = {
                "objective": "quantile",
                "alpha": q,
                "learning_rate": 0.05,
                "num_leaves": 15,
                "min_data_in_leaf": 5,
                "seed": SEED,
                "num_threads": 1,
                "deterministic": True,
                "force_row_wise": True,
                "verbose": -1,
            }
            model = lgb.train(params, lgb.Dataset(x, label=y_arr), num_boost_round=100)
            self.models[q] = model
        return self

    def forecast(
        self, history: np.ndarray, start_date: dt.date, horizon: int
    ) -> dict[str, list[float]]:
        """Recursive multi-step forecast; the p50 is fed back as the next value."""
        values = [float(v) for v in history]
        out: dict[str, list[float]] = {"p10": [], "p50": [], "p90": []}
        for h in range(horizon):
            day = start_date + dt.timedelta(days=h)
            x = np.asarray([feature_vector(np.asarray(values), day)], dtype=float)
            preds = sorted(max(0.0, float(self.models[q].predict(x)[0])) for q in QUANTILES)
            lo, mid, hi = (round(p, 2) for p in preds)
            out["p10"].append(lo)
            out["p50"].append(mid)
            out["p90"].append(hi)
            values.append(mid)
        return out
