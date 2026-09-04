"""Generate a tiny `tracking_events` Parquet warehouse for local/CI runs.

Writes the canonical team-analytics schema (warehouse.Schema column order) so
`spark.read.parquet(...)` in the job reads it exactly like the DuckDB-exported
Parquet. Uses pandas + pyarrow only (no Spark, no DuckDB) so it runs on a laptop.

    python sample_data/generate_sample.py /data/tracking_events.parquet
"""

from __future__ import annotations

import json
import sys
import uuid
from datetime import datetime, timedelta, timezone

# Canonical column order (team-analytics internal/warehouse/warehouse.go Schema).
COLUMNS = [
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

# A small, deterministic interaction set: several users (mix of authenticated
# principals and anonymous cookies) over a handful of listings, with a spread of
# event types so ALS has weighted signal. Includes an empty-listing row that MUST
# be dropped by the interaction mapper.
_INTERACTIONS = [
    # (principal_id, anonymous_id, listing_id, event_type)
    ("user-1", "", "listing-a", "view"),
    ("user-1", "", "listing-a", "click"),
    ("user-1", "", "listing-b", "view"),
    ("user-1", "", "listing-c", "add_to_cart"),
    ("user-2", "", "listing-a", "view"),
    ("user-2", "", "listing-b", "click"),
    ("user-2", "", "listing-d", "view"),
    ("", "anon-1", "listing-b", "view"),
    ("", "anon-1", "listing-c", "click"),
    ("", "anon-1", "listing-c", "add_to_cart"),
    ("", "anon-2", "listing-a", "impression"),
    ("", "anon-2", "listing-d", "view"),
    ("", "anon-2", "listing-d", "click"),
    ("user-3", "", "listing-c", "view"),
    ("user-3", "", "listing-d", "add_to_cart"),
    ("user-3", "", "listing-a", "view"),
    ("", "", "", "view"),  # dropped: empty listing_id AND empty user
    ("user-2", "", "", "click"),  # dropped: empty listing_id
]


def build_rows() -> list[dict]:
    now = datetime.now(timezone.utc)
    rows = []
    for i, (principal_id, anon, listing_id, etype) in enumerate(_INTERACTIONS):
        rows.append(
            {
                "event_id": str(uuid.uuid4()),
                "event_type": etype,
                "listing_id": listing_id,
                "session_id": f"sess-{i % 5}",
                "anonymous_id": anon,
                "page_path": "/p" if listing_id else "/",
                "referrer": "",
                "position": (i % 10) + 1,
                "search_query": "",
                "occurred_at": now - timedelta(days=i % 7, minutes=i),
                "principal_id": principal_id,
                "principal_type": "user" if principal_id else "",
                "properties": json.dumps({}),
            }
        )
    return rows


def main(dst: str) -> None:
    import pandas as pd  # noqa: PLC0415

    df = pd.DataFrame(build_rows(), columns=COLUMNS)
    df.to_parquet(dst, index=False)
    print(f"wrote {len(df)} rows -> {dst}")


if __name__ == "__main__":
    out = sys.argv[1] if len(sys.argv) > 1 else "/data/tracking_events.parquet"
    main(out)
