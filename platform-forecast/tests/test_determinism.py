from __future__ import annotations

from _selection_fixtures import AS_OF, trend_series

from forecast.config import Settings
from forecast.selection import select_and_forecast

SETTINGS = Settings(horizon_days=14, min_history_days_for_ml=14, backtest_days=14)


def _days(res):
    return {
        (p["seller_id"], p["listing_id"]): p["daily_forecasts"] for p in res["payloads"]
    }


def test_same_input_same_output():
    """FNJ-9"""
    first = select_and_forecast(trend_series(), AS_OF, SETTINGS)
    second = select_and_forecast(trend_series(), AS_OF, SETTINGS)
    assert first["model_version"] == "lgbm_quantile_v1"
    assert len(first["payloads"]) == 8
    assert _days(first) == _days(second)
    assert first["wape"] == second["wape"]
