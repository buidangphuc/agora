"""Point-in-time inputs: DuckDB views over the Parquet exports, filtered by AS_OF (design D2/D3)."""

from __future__ import annotations

import sys
from datetime import datetime
from pathlib import Path

import duckdb
import pyarrow.parquet as pq

from featurestore.settings import ConfigError

FILES = {
    "events": "tracking_events_resolved.parquet",
    "facts": "engagement_facts.parquet",
    "orders": "order_facts.parquet",
}


# Optional input (featurestore-item-attributes): the listing -> seller, category, price table.
LISTINGS_FILE = "listing_sellers.parquet"


def _lit(ts: datetime) -> str:
    return f"TIMESTAMP '{ts.strftime('%Y-%m-%d %H:%M:%S')}'"


def input_files(input_dir: Path) -> dict[str, Path]:
    paths = {k: input_dir / v for k, v in FILES.items()}
    missing = [str(p) for p in paths.values() if not p.exists()]
    if missing:
        raise ConfigError("missing input parquet file(s): " + ", ".join(missing))
    return paths


def _require_buyer_column(path: Path) -> None:
    """order_facts must carry buyer_id (order-facts-buyer); fail loudly instead of computing zeros."""
    if "buyer_id" not in pq.read_schema(path).names:
        raise ConfigError(
            f"{path.name} has no buyer_id column (written by an older team-analytics); "
            "wait for the next export cycle after team-analytics is upgraded"
        )


def listings_file(input_dir: Path) -> Path | None:
    """The listing export, or None when team-analytics has not written one (an older exporter)."""
    p = input_dir / LISTINGS_FILE
    return p if p.exists() else None


def _require_listing_columns(path: Path) -> None:
    """A listing export without category_id/price is an older exporter: fail naming the column."""
    names = pq.read_schema(path).names
    for col in ("category_id", "price"):
        if col not in names:
            raise ConfigError(
                f"{path.name} has no {col} column (written by an older team-analytics); "
                "wait for the next export cycle after team-analytics is upgraded"
            )


def _retry(fn):
    try:
        return fn()
    except duckdb.IOException:
        return fn()  # a rename may race the read; retry once


def connect(input_dir: Path, as_of: datetime) -> duckdb.DuckDBPyConnection:
    """A connection exposing only the filtered tables `events`, `facts`, `orders`, `listings`.

    The inputs are copied into temp tables filtered by AS_OF, then external access is
    switched off and the configuration locked, so no definition can read a Parquet file
    (or anything else) directly and bypass the point-in-time rule.
    """
    paths = input_files(input_dir)
    _require_buyer_column(paths["orders"])
    listings = listings_file(input_dir)
    if listings is not None:
        _require_listing_columns(listings)
    con = duckdb.connect(":memory:")
    con.execute("SET TimeZone='UTC'")
    a = _lit(as_of)

    def create():
        con.execute(
            f"CREATE TEMP TABLE events AS SELECT * REPLACE (occurred_at::TIMESTAMP AS occurred_at, "
            f"ingested_at::TIMESTAMP AS ingested_at) FROM read_parquet('{paths['events']}') "
            f"WHERE ingested_at::TIMESTAMP <= {a}"
        )
        con.execute(
            f"CREATE TEMP TABLE facts AS SELECT * REPLACE (occurred_at::TIMESTAMP AS occurred_at, "
            f"ingested_at::TIMESTAMP AS ingested_at) FROM read_parquet('{paths['facts']}') "
            f"WHERE ingested_at::TIMESTAMP <= {a}"
        )
        con.execute(
            f"CREATE TEMP TABLE orders AS SELECT * REPLACE (occurred_at::TIMESTAMP AS occurred_at) "
            f"FROM read_parquet('{paths['orders']}') WHERE occurred_at::TIMESTAMP <= {a}"
        )

    def create_listings():
        # Point in time: the table keeps one row per listing (its latest change), so a listing
        # changed after AS_OF is left out rather than leaking a later value.
        if listings is None:
            con.execute(
                "CREATE TEMP TABLE listings (listing_id VARCHAR, seller_id VARCHAR, category_id VARCHAR, "
                "price BIGINT, updated_at TIMESTAMP)"
            )
            return
        con.execute(
            "CREATE TEMP TABLE listings AS SELECT listing_id, seller_id, category_id, price::BIGINT AS price, "
            f"updated_at::TIMESTAMP AS updated_at FROM read_parquet('{listings}') "
            f"WHERE updated_at::TIMESTAMP <= {a}"
        )

    _retry(create)
    _retry(create_listings)
    if listings is None:
        print(
            f"warning: {LISTINGS_FILE} not found in {input_dir}: item_attributes and user_preferences are empty",
            file=sys.stderr,
        )
    con.execute("SET enable_external_access = false")
    con.execute("SET lock_configuration = true")
    return con


def watermark(con: duckdb.DuckDBPyConnection) -> datetime | None:
    row = con.execute(
        "SELECT max(m) FROM (SELECT max(ingested_at) AS m FROM events "
        "UNION ALL SELECT max(ingested_at) FROM facts)"
    ).fetchone()
    return row[0] if row else None


def input_stats(input_dir: Path) -> list[dict]:
    out = []
    paths = dict(input_files(input_dir))
    listings = listings_file(input_dir)
    if listings is not None:
        paths["listings"] = listings
    for name, p in paths.items():
        st = p.stat()
        out.append({"name": p.name, "input": name, "size": st.st_size, "mtime": st.st_mtime})
    return out
