"""Publishes forecast artifacts to Redis."""

from __future__ import annotations

import datetime
import json
from typing import Any


def format_forecast_payload(
    seller_id: str,
    listing_id: str,
    start_date: datetime.date | str,
    quantiles_dict: dict[str, list[float]],
    model_version: str,
    is_cold_start: bool = False,
) -> dict[str, Any]:
    """Formats daily forecast records for Redis storage."""
    p10_list = quantiles_dict.get("p10", [])
    p50_list = quantiles_dict.get("p50", [])
    p90_list = quantiles_dict.get("p90", [])
    
    n_days = len(p50_list)
    start_dt = pd.to_datetime(start_date)
    dates = pd.date_range(start=start_dt, periods=n_days).strftime("%Y-%m-%d").tolist()

    daily_forecasts = []
    for i in range(n_days):
        daily_forecasts.append({
            "date": dates[i],
            "p10": float(p10_list[i]) if i < len(p10_list) else 0.0,
            "p50": float(p50_list[i]) if i < len(p50_list) else 0.0,
            "p90": float(p90_list[i]) if i < len(p90_list) else 0.0,
        })

    return {
        "seller_id": seller_id,
        "listing_id": listing_id,
        "model_version": model_version,
        "generated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "is_cold_start": is_cold_start,
        "daily_forecasts": daily_forecasts,
    }


def publish_to_redis(
    redis_client: Any,
    seller_id: str,
    listing_id: str,
    payload: dict[str, Any],
    prefix: str = "fc:v1:seller",
    ttl_seconds: int = 172800,
) -> str:
    """Writes forecast JSON payload to Redis."""
    key = f"{prefix}:{seller_id}:{listing_id}"
    data = json.dumps(payload)
    if redis_client is not None:
        redis_client.setex(key, ttl_seconds, data)
    return key


import pandas as pd  # noqa: E402
