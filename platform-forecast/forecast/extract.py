"""Extracts daily order facts from warehouse for forecasting."""

from __future__ import annotations

import datetime
from typing import Any

import pandas as pd


def extract_daily_sales_dataframe(
    raw_facts: list[dict[str, Any]] | pd.DataFrame,
) -> pd.DataFrame:
    """Aggregates raw order facts into daily time series per (seller_id, listing_id, date)."""
    if isinstance(raw_facts, list):
        if not raw_facts:
            return pd.DataFrame(columns=["seller_id", "listing_id", "date", "quantity", "revenue"])
        df = pd.DataFrame(raw_facts)
    else:
        df = raw_facts.copy()

    if df.empty:
        return pd.DataFrame(columns=["seller_id", "listing_id", "date", "quantity", "revenue"])

    # Ensure date format
    if "occurred_at" in df.columns:
        df["date"] = pd.to_datetime(df["occurred_at"]).dt.strftime("%Y-%m-%d")
    elif "date" in df.columns:
        df["date"] = pd.to_datetime(df["date"]).dt.strftime("%Y-%m-%d")

    df["quantity"] = pd.to_numeric(df["quantity"], errors="coerce").fillna(0)
    if "unit_price" in df.columns:
        df["revenue"] = df["quantity"] * pd.to_numeric(df["unit_price"], errors="coerce").fillna(0)
    elif "revenue" not in df.columns:
        df["revenue"] = 0.0

    # Group by (seller_id, listing_id, date)
    daily = (
        df.groupby(["seller_id", "listing_id", "date"], as_index=False)
        .agg({"quantity": "sum", "revenue": "sum"})
        .sort_values(["seller_id", "listing_id", "date"])
    )
    return daily


def fill_missing_dates(
    df: pd.DataFrame,
    start_date: datetime.date | str,
    end_date: datetime.date | str,
) -> pd.DataFrame:
    """Fills missing dates with 0 quantity to make a regular daily time series."""
    if df.empty:
        return df

    all_dates = pd.date_range(start=start_date, end=end_date).strftime("%Y-%m-%d")
    records = []

    for (seller_id, listing_id), group in df.groupby(["seller_id", "listing_id"]):
        group_indexed = group.set_index("date")
        for dt in all_dates:
            if dt in group_indexed.index:
                qty = group_indexed.loc[dt, "quantity"]
                rev = group_indexed.loc[dt, "revenue"]
            else:
                qty = 0.0
                rev = 0.0
            records.append({
                "seller_id": seller_id,
                "listing_id": listing_id,
                "date": dt,
                "quantity": float(qty),
                "revenue": float(rev),
            })

    return pd.DataFrame(records)
