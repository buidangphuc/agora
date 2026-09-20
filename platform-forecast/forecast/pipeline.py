"""Orchestration pipeline for offline probabilistic demand forecasting."""

from __future__ import annotations

import datetime
from typing import Any

import pandas as pd

from forecast.config import Settings
from forecast.extract import extract_daily_sales_dataframe, fill_missing_dates
from forecast.features import generate_time_features
from forecast.models.baseline import SeasonalNaiveQuantileForecaster
from forecast.models.quantile_lgbm import QuantileRegressor
from forecast.publisher import format_forecast_payload, publish_to_redis


def run_demand_forecast_pipeline(
    raw_facts: list[dict[str, Any]] | pd.DataFrame,
    redis_client: Any | None = None,
    settings: Settings | None = None,
) -> dict[str, Any]:
    """Executes the full forecasting pipeline across all sellers and listings."""
    settings = settings or Settings()
    daily_df = extract_daily_sales_dataframe(raw_facts)
    if daily_df.empty:
        return {"status": "empty", "forecast_count": 0, "model_version": settings.model_version}

    min_date = daily_df["date"].min()
    max_date = daily_df["date"].max()
    forecast_start_date = (pd.to_datetime(max_date) + pd.Timedelta(days=1)).strftime("%Y-%m-%d")

    # Regularize daily series
    regular_df = fill_missing_dates(daily_df, min_date, max_date)
    features_df = generate_time_features(regular_df)

    baseline_model = SeasonalNaiveQuantileForecaster(season_length=7)
    ml_model = QuantileRegressor(quantiles=settings.quantiles)

    published_keys: list[str] = []
    published_payloads: list[dict[str, Any]] = []

    for (seller_id, listing_id), group in features_df.groupby(["seller_id", "listing_id"]):
        n_obs = len(group)
        is_cold_start = n_obs < settings.min_history_days_for_ml

        if is_cold_start:
            # Baseline forecast
            series = group["quantity"]
            q_preds = baseline_model.forecast_series(series, horizon=settings.horizon_days, quantiles=settings.quantiles)
            model_ver = f"{settings.model_version}_baseline"
        else:
            # ML Quantile forecast
            q_preds = ml_model.predict_quantiles(group, horizon=settings.horizon_days)
            model_ver = settings.model_version

        payload = format_forecast_payload(
            seller_id=seller_id,
            listing_id=listing_id,
            start_date=forecast_start_date,
            quantiles_dict=q_preds,
            model_version=model_ver,
            is_cold_start=is_cold_start,
        )

        key = publish_to_redis(
            redis_client=redis_client,
            seller_id=seller_id,
            listing_id=listing_id,
            payload=payload,
            prefix=settings.redis_prefix,
            ttl_seconds=settings.redis_ttl_seconds,
        )
        published_keys.append(key)
        published_payloads.append(payload)

    return {
        "status": "success",
        "forecast_count": len(published_keys),
        "forecast_start_date": forecast_start_date,
        "horizon_days": settings.horizon_days,
        "published_keys": published_keys,
        "payloads": published_payloads,
    }
