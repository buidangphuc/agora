"""Backtest-driven choice between the global LightGBM model and the seasonal-naive baseline."""

from __future__ import annotations

import datetime as dt
from typing import Any

import numpy as np
import pandas as pd

from forecast.config import Settings
from forecast.evals.backtest import compute_wape
from forecast.models.baseline import BASELINE_VERSION, SeasonalNaiveQuantileForecaster
from forecast.models.quantile_lgbm import MODEL_VERSION, GlobalQuantileLGBM
from forecast.publisher import format_forecast_payload


def _backtest(
    histories: dict[tuple[str, str], np.ndarray],
    as_of_date: dt.date,
    settings: Settings,
) -> tuple[float | None, float | None]:
    holdout = settings.backtest_days
    min_hist = settings.min_history_days_for_ml
    eligible = {k: v for k, v in histories.items() if len(v) - holdout >= min_hist}
    if not eligible:
        return None, None
    train_end = as_of_date - dt.timedelta(days=holdout)
    model = GlobalQuantileLGBM(min_hist).fit([v[:-holdout] for v in eligible.values()], train_end)
    baseline = SeasonalNaiveQuantileForecaster(season_length=7)
    actuals: list[float] = []
    lgbm_pred: list[float] = []
    base_pred: list[float] = []
    for hist in eligible.values():
        past = hist[:-holdout]
        actuals.extend(float(v) for v in hist[-holdout:])
        lgbm_pred.extend(model.forecast(past, train_end, holdout)["p50"])
        base_pred.extend(baseline.forecast_series(pd.Series(past), horizon=holdout)["p50"])
    return compute_wape(actuals, lgbm_pred), compute_wape(actuals, base_pred)


def select_and_forecast(
    series: pd.DataFrame, as_of: dt.datetime, settings: Settings
) -> dict[str, Any]:
    as_of_date = as_of.date()
    horizon = settings.horizon_days
    min_hist = settings.min_history_days_for_ml

    histories: dict[tuple[str, str], np.ndarray] = {}
    for (seller, listing), group in series.groupby(["seller_id", "listing_id"], sort=True):
        ordered = group.sort_values("date")
        histories[(str(seller), str(listing))] = ordered["units"].to_numpy(dtype=float)

    warm = {k: v for k, v in histories.items() if len(v) >= min_hist}
    lgbm_wape, baseline_wape = _backtest(warm, as_of_date, settings)
    use_lgbm = lgbm_wape is not None and baseline_wape is not None and lgbm_wape < baseline_wape

    model = GlobalQuantileLGBM(min_hist).fit(list(warm.values()), as_of_date) if use_lgbm else None
    baseline = SeasonalNaiveQuantileForecaster(season_length=7)

    payloads: list[dict[str, Any]] = []
    cold_starts = 0
    for (seller, listing), hist in histories.items():
        cold = len(hist) < min_hist
        cold_starts += int(cold)
        if model is not None and not cold:
            preds = model.forecast(hist, as_of_date, horizon)
            version = MODEL_VERSION
        else:
            preds = baseline.forecast_series(pd.Series(hist), horizon=horizon)
            version = BASELINE_VERSION
        payloads.append(
            format_forecast_payload(
                seller_id=seller,
                listing_id=listing,
                start_date=as_of_date,
                quantiles_dict=preds,
                model_version=version,
                is_cold_start=cold,
            )
        )

    return {
        "model_version": MODEL_VERSION if use_lgbm else BASELINE_VERSION,
        "wape": lgbm_wape if use_lgbm else baseline_wape,
        "baseline_wape": baseline_wape,
        "cold_starts": cold_starts,
        "listings": len(payloads),
        "payloads": payloads,
    }
