"""Time-series feature engineering for demand forecasting."""

from __future__ import annotations

import pandas as pd


def generate_time_features(df: pd.DataFrame) -> pd.DataFrame:
    """Computes lag features, rolling statistics, and calendar features."""
    if df.empty:
        return df

    out = df.copy()
    out["dt"] = pd.to_datetime(out["date"])
    out["day_of_week"] = out["dt"].dt.dayofweek
    out["is_weekend"] = out["day_of_week"].isin([5, 6]).astype(float)
    out["day_of_month"] = out["dt"].dt.day

    # Sort to ensure lag consistency
    out = out.sort_values(["seller_id", "listing_id", "dt"])

    # Compute lags per (seller_id, listing_id)
    grouped = out.groupby(["seller_id", "listing_id"])
    
    out["lag_1"] = grouped["quantity"].shift(1).fillna(0.0)
    out["lag_7"] = grouped["quantity"].shift(7).fillna(0.0)
    out["lag_14"] = grouped["quantity"].shift(14).fillna(0.0)

    # Rolling statistics
    out["rolling_mean_7"] = grouped["quantity"].shift(1).rolling(7, min_periods=1).mean().fillna(0.0)
    out["rolling_std_7"] = grouped["quantity"].shift(1).rolling(7, min_periods=1).std().fillna(0.0)
    out["rolling_mean_14"] = grouped["quantity"].shift(1).rolling(14, min_periods=1).mean().fillna(0.0)
    out["rolling_mean_28"] = grouped["quantity"].shift(1).rolling(28, min_periods=1).mean().fillna(0.0)

    return out.drop(columns=["dt"])
