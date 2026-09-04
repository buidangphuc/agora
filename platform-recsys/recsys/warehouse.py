"""Warehouse reader seam — the ONLY driver-specific code.

Reads the canonical `tracking_events` table produced by team-analytics
(warehouse.Schema) over a rolling interaction window:

- local  (WAREHOUSE_DRIVER=duckdb):   spark.read.parquet(<exported parquet>)
- prod   (WAREHOUSE_DRIVER=bigquery): the BigQuery table via the Spark BigQuery
  connector.

Everything downstream operates on the same DataFrame of tracking rows, so the
driver switch is contained entirely here.
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone

from .config import DRIVER_BIGQUERY, DRIVER_DUCKDB, Settings

# The canonical column list both team-analytics adapters agree on
# (internal/warehouse/warehouse.go Schema). Kept here as the read contract.
TRACKING_COLUMNS = [
    "event_id",
    "event_type",
    "listing_id",
    "session_id",
    "anonymous_id",
    "page_path",
    "referrer",
    "position",
    "search_query",
    "occurred_at",
    "principal_id",
    "principal_type",
    "properties",
]


def _window_start(settings: Settings) -> datetime:
    return datetime.now(timezone.utc) - timedelta(days=settings.interaction_window_days)


def read_tracking_events(spark, settings: Settings):
    """Return a DataFrame of `tracking_events` rows within the rolling window."""
    from pyspark.sql import functions as F  # noqa: PLC0415

    if settings.warehouse_driver == DRIVER_DUCKDB:
        df = spark.read.parquet(settings.warehouse_parquet_path)
    elif settings.warehouse_driver == DRIVER_BIGQUERY:
        table = f"{settings.bigquery_project}.{settings.bigquery_dataset}.{settings.bigquery_table}"
        df = spark.read.format("bigquery").option("table", table).load()
    else:  # pragma: no cover - guarded by Settings.validate()
        raise ValueError(f"unsupported WAREHOUSE_DRIVER: {settings.warehouse_driver!r}")

    # Keep only the canonical columns that are actually present (parquet export
    # carries them all; be defensive so a schema addition upstream never breaks
    # the read). Then bound to the rolling window by occurred_at.
    present = [c for c in TRACKING_COLUMNS if c in df.columns]
    df = df.select(*present)
    if "occurred_at" in df.columns:
        start = _window_start(settings)
        df = df.filter(F.col("occurred_at") >= F.lit(start))
    return df
