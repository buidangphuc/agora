"""Extracts daily order facts from warehouse for forecasting."""

from __future__ import annotations

import datetime
from pathlib import Path
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


class ConfigError(Exception):
    """Input data or configuration is unusable (exit code 2)."""


_REQUIRED_COLUMNS = ("seller_id", "listing_id", "quantity", "occurred_at", "status")


def read_paid_series(
    input_dir: Any, as_of: datetime.datetime, history_days: int
) -> pd.DataFrame:
    """Per-listing daily paid units from first sale to the last complete day before as_of."""
    import duckdb

    path = Path(input_dir) / "order_facts.parquet"
    if not path.is_file():
        raise ConfigError(f"missing input file: {path.name} in {input_dir}")

    last_day = as_of.date() - datetime.timedelta(days=1)
    first_day = as_of.date() - datetime.timedelta(days=history_days)
    con = duckdb.connect()
    try:
        try:
            cols = {
                r[0]
                for r in con.execute(
                    "DESCRIBE SELECT * FROM read_parquet(?)", [str(path)]
                ).fetchall()
            }
        except duckdb.Error as exc:
            raise ConfigError(f"unreadable {path.name}: {exc}") from exc
        missing = [c for c in _REQUIRED_COLUMNS if c not in cols]
        if missing:
            raise ConfigError(f"{path.name} missing columns: {', '.join(missing)}")
        daily = con.execute(
            """
            SELECT seller_id, listing_id, CAST(occurred_at AS DATE) AS date,
                   CAST(SUM(quantity) AS BIGINT) AS units
            FROM read_parquet(?)
            WHERE status = 'PAID'
              AND CAST(occurred_at AS DATE) BETWEEN ? AND ?
            GROUP BY 1, 2, 3
            """,
            [str(path), first_day, last_day],
        ).df()
    finally:
        con.close()

    columns = ["seller_id", "listing_id", "date", "units"]
    if daily.empty:
        return pd.DataFrame(columns=columns)
    daily["date"] = pd.to_datetime(daily["date"])
    end = pd.Timestamp(last_day)
    frames = []
    for (seller, listing), g in daily.groupby(["seller_id", "listing_id"]):
        idx = pd.date_range(g["date"].min(), end)
        s = g.set_index("date")["units"].reindex(idx, fill_value=0)
        frames.append(
            pd.DataFrame(
                {"seller_id": seller, "listing_id": listing, "date": idx, "units": s.to_numpy()}
            )
        )
    return pd.concat(frames, ignore_index=True)[columns]
