from __future__ import annotations

import datetime as dt

from _selection_fixtures import AS_OF, make_series, seasonal_noisy, trend_series

from forecast.config import Settings
from forecast.selection import select_and_forecast

SETTINGS = Settings(horizon_days=14, min_history_days_for_ml=14, backtest_days=14)


def _check_points(payload):
    days = payload["daily_forecasts"]
    assert len(days) == 14
    assert days[0]["date"] == AS_OF.date().isoformat()
    assert days[-1]["date"] == (AS_OF.date() + dt.timedelta(days=13)).isoformat()
    for d in days:
        assert 0 <= d["p10"] <= d["p50"] <= d["p90"]


def test_lgbm_wins_on_trend():
    """FNJ-11, FNJ-14"""
    res = select_and_forecast(trend_series(), AS_OF, SETTINGS)
    assert res["wape"] is not None
    assert res["wape"] < res["baseline_wape"]
    assert res["model_version"] == "lgbm_quantile_v1"
    assert len(res["payloads"]) == 8
    for p in res["payloads"]:
        assert p["model_version"] == "lgbm_quantile_v1"
        assert p["is_cold_start"] is False
        _check_points(p)


def test_baseline_labelled_when_not_beaten():
    """FNJ-12, FNJ-14"""
    flat = make_series({("S", f"L{i}"): [5.0] * 60 for i in range(4)})
    res = select_and_forecast(flat, AS_OF, SETTINGS)
    assert res["baseline_wape"] is not None
    assert not (res["wape"] < res["baseline_wape"])
    assert res["model_version"] == "seasonal_naive_v1"
    assert len(res["payloads"]) == 4
    for p in res["payloads"]:
        assert p["model_version"] == "seasonal_naive_v1"
        _check_points(p)


def test_baseline_labelled_when_no_listing_eligible():
    """FNJ-12"""
    short = make_series({("S", f"L{i}"): [3.0, 4.0, 5.0, 2.0, 6.0] for i in range(3)})
    res = select_and_forecast(short, AS_OF, SETTINGS)
    assert res["wape"] is None
    assert res["model_version"] == "seasonal_naive_v1"
    assert len(res["payloads"]) == 3
    assert all(p["model_version"] == "seasonal_naive_v1" for p in res["payloads"])


def test_short_history_is_cold_start():
    """FNJ-13"""
    listings = {("S", f"L{i}"): seasonal_noisy(84, i) for i in range(8)}
    listings[("S", "C")] = [2.0, 0.0, 3.0, 1.0, 4.0]
    res = select_and_forecast(make_series(listings), AS_OF, SETTINGS)
    by_listing = {p["listing_id"]: p for p in res["payloads"]}
    cold = by_listing["C"]
    assert cold["is_cold_start"] is True
    assert cold["model_version"] == "seasonal_naive_v1"
    _check_points(cold)
    assert res["cold_starts"] == 1
    for lid, p in by_listing.items():
        if lid != "C":
            assert p["is_cold_start"] is False


def test_label_never_overstates_model():
    """FNJ-14"""
    for series in (
        trend_series(),
        make_series({("S", "A"): [5.0] * 60}),
        make_series({("S", "A"): [1.0, 2.0, 3.0]}),
    ):
        res = select_and_forecast(series, AS_OF, SETTINGS)
        labels = {p["model_version"] for p in res["payloads"] if not p["is_cold_start"]}
        if "lgbm_quantile_v1" in labels:
            assert res["wape"] is not None
            assert res["wape"] < res["baseline_wape"]
            assert res["model_version"] == "lgbm_quantile_v1"


def test_quantile_regressor_is_deleted():
    """FNJ-11"""
    import forecast.models.quantile_lgbm as mod

    assert not hasattr(mod, "QuantileRegressor")
