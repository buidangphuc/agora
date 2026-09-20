"""Unit and pipeline integration tests for platform-forecast."""

import datetime
from unittest.mock import MagicMock

import pandas as pd
import pytest

from forecast.config import Settings
from forecast.evals.backtest import (
    compute_interval_coverage,
    compute_pinball_loss,
    compute_wape,
)
from forecast.extract import extract_daily_sales_dataframe, fill_missing_dates
from forecast.models.baseline import SeasonalNaiveQuantileForecaster
from forecast.models.quantile_lgbm import QuantileRegressor
from forecast.pipeline import run_demand_forecast_pipeline


def test_eval_metrics():
    actuals = [10, 20, 30, 40]
    preds = [12, 18, 33, 38]
    wape = compute_wape(actuals, preds)
    assert 0.0 < wape < 0.2

    p50_loss = compute_pinball_loss(actuals, preds, 0.5)
    assert p50_loss >= 0.0

    coverage = compute_interval_coverage([10, 20, 30], [5, 15, 25], [15, 25, 35])
    assert coverage == 1.0


def test_baseline_and_ml_models():
    series = pd.Series([5, 8, 12, 6, 9, 15, 10, 7, 8, 14, 5, 11, 16, 12])
    forecaster = SeasonalNaiveQuantileForecaster(season_length=7)
    res = forecaster.forecast_series(series, horizon=14)
    assert "p10" in res and "p50" in res and "p90" in res
    assert len(res["p50"]) == 14
    assert res["p10"][0] <= res["p50"][0] <= res["p90"][0]


def test_full_pipeline_run():
    # Synthetic order facts over 20 days
    base_date = datetime.date(2026, 9, 1)
    facts = []
    for d in range(20):
        dt = base_date + datetime.timedelta(days=d)
        facts.append({
            "seller_id": "seller-01",
            "listing_id": "item-01",
            "date": dt.strftime("%Y-%m-%d"),
            "quantity": 5 + (d % 3),
            "unit_price": 100000,
        })
        facts.append({
            "seller_id": "seller-01",
            "listing_id": "item-02",
            "date": dt.strftime("%Y-%m-%d"),
            "quantity": 10 + (d % 5),
            "unit_price": 50000,
        })

    mock_redis = MagicMock()
    settings = Settings(horizon_days=14, min_history_days_for_ml=10)

    result = run_demand_forecast_pipeline(facts, redis_client=mock_redis, settings=settings)
    assert result["status"] == "success"
    assert result["forecast_count"] == 2
    assert result["horizon_days"] == 14
    assert len(result["published_keys"]) == 2
    assert mock_redis.setex.call_count == 2
