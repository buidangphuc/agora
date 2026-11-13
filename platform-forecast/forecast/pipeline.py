"""Orchestration pipeline for offline probabilistic demand forecasting."""

from __future__ import annotations

import datetime
from typing import Any

import pandas as pd

from forecast.config import Settings
from forecast.extract import extract_daily_sales_dataframe, fill_missing_dates
from forecast.publisher import publish_to_redis
from forecast.selection import select_and_forecast


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
    forecast_start = pd.to_datetime(max_date) + pd.Timedelta(days=1)
    forecast_start_date = forecast_start.strftime("%Y-%m-%d")

    regular_df = fill_missing_dates(daily_df, min_date, max_date)
    series = regular_df.rename(columns={"quantity": "units"})[
        ["seller_id", "listing_id", "date", "units"]
    ]
    result = select_and_forecast(series, forecast_start.to_pydatetime(), settings)

    published_keys: list[str] = []
    published_payloads: list[dict[str, Any]] = result["payloads"]
    for payload in published_payloads:
        key = publish_to_redis(
            redis_client=redis_client,
            seller_id=payload["seller_id"],
            listing_id=payload["listing_id"],
            payload=payload,
            prefix=settings.redis_prefix,
            ttl_seconds=settings.redis_ttl_seconds,
        )
        published_keys.append(key)

    return {
        "status": "success",
        "forecast_count": len(published_keys),
        "forecast_start_date": forecast_start_date,
        "horizon_days": settings.horizon_days,
        "published_keys": published_keys,
        "payloads": published_payloads,
    }
