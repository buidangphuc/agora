"""Orchestration pipeline for offline probabilistic demand forecasting."""

from __future__ import annotations

import datetime
from typing import Any

import pandas as pd

from forecast.config import Settings
from forecast.extract import extract_daily_sales_dataframe, fill_missing_dates, read_paid_series
from forecast.publisher import publish_meta, publish_to_redis
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


def run_forecast_job(
    settings: Settings, as_of: datetime.datetime, redis_client: Any
) -> dict[str, Any]:
    """Reads the export, forecasts every listing, writes listing keys, then the meta key."""
    series = read_paid_series(settings.input_dir, as_of, settings.history_days)
    generated_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
    base = {
        "generated_at": generated_at,
        "as_of": as_of.isoformat(),
        "horizon_days": settings.horizon_days,
    }
    if series.empty:
        return {
            "model_version": settings.model_version,
            **base,
            "listings": 0,
            "cold_starts": 0,
            "wape": None,
            "baseline_wape": None,
            "status": "empty",
        }

    result = select_and_forecast(series, as_of, settings)
    for payload in result["payloads"]:
        publish_to_redis(
            redis_client=redis_client,
            seller_id=payload["seller_id"],
            listing_id=payload["listing_id"],
            payload=payload,
            prefix=settings.redis_prefix,
            ttl_seconds=settings.redis_ttl_seconds,
        )

    def _num(value: Any) -> float | None:
        return None if value is None else float(value)

    meta = {
        "model_version": result["model_version"],
        **base,
        "listings": result["listings"],
        "cold_starts": result["cold_starts"],
        "wape": _num(result["wape"]),
        "baseline_wape": _num(result["baseline_wape"]),
        "status": "ok",
    }
    publish_meta(redis_client, meta, ttl_seconds=settings.redis_ttl_seconds)
    return meta
