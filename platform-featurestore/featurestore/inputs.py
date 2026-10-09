"""Point-in-time inputs: DuckDB views over the Parquet exports, filtered by AS_OF (design D2/D3)."""

from __future__ import annotations

from datetime import datetime
from pathlib import Path

import duckdb

from featurestore.settings import ConfigError

FILES = {
    "events": "tracking_events_resolved.parquet",
    "facts": "engagement_facts.parquet",
    "orders": "order_facts.parquet",
}


def _lit(ts: datetime) -> str:
    return f"TIMESTAMP '{ts.strftime('%Y-%m-%d %H:%M:%S')}'"


def input_files(input_dir: Path) -> dict[str, Path]:
    paths = {k: input_dir / v for k, v in FILES.items()}
    missing = [str(p) for p in paths.values() if not p.exists()]
    if missing:
        raise ConfigError("missing input parquet file(s): " + ", ".join(missing))
    return paths


def _retry(fn):
    try:
        return fn()
    except duckdb.IOException:
        return fn()  # a rename may race the read; retry once


def connect(input_dir: Path, as_of: datetime) -> duckdb.DuckDBPyConnection:
    """A connection exposing only the filtered views `events`, `facts`, `orders`."""
    paths = input_files(input_dir)
    con = duckdb.connect(":memory:")
    con.execute("SET TimeZone='UTC'")
    a = _lit(as_of)

    def create():
        con.execute(
            f"CREATE TEMP VIEW events AS SELECT * REPLACE (occurred_at::TIMESTAMP AS occurred_at, "
            f"ingested_at::TIMESTAMP AS ingested_at) FROM read_parquet('{paths['events']}') "
            f"WHERE ingested_at::TIMESTAMP <= {a}"
        )
        con.execute(
            f"CREATE TEMP VIEW facts AS SELECT * REPLACE (occurred_at::TIMESTAMP AS occurred_at, "
            f"ingested_at::TIMESTAMP AS ingested_at) FROM read_parquet('{paths['facts']}') "
            f"WHERE ingested_at::TIMESTAMP <= {a}"
        )
        con.execute(
            f"CREATE TEMP VIEW orders AS SELECT * REPLACE (occurred_at::TIMESTAMP AS occurred_at) "
            f"FROM read_parquet('{paths['orders']}') WHERE occurred_at::TIMESTAMP <= {a}"
        )

    _retry(create)
    return con


def watermark(con: duckdb.DuckDBPyConnection) -> datetime | None:
    row = con.execute(
        "SELECT max(m) FROM (SELECT max(ingested_at) AS m FROM events "
        "UNION ALL SELECT max(ingested_at) FROM facts)"
    ).fetchone()
    return row[0] if row else None


def input_stats(input_dir: Path) -> list[dict]:
    out = []
    for name, p in input_files(input_dir).items():
        st = p.stat()
        out.append({"name": p.name, "input": name, "size": st.st_size, "mtime": st.st_mtime})
    return out
