"""Runs INSIDE the platform-featurestore job image (mounted at /work by fia_flow).

Helper for featurestore-item-attributes. Reads /work/plan.json, prints one JSON line on stdout.

ops:
  probe     read-only: the rows of /data/listing_sellers.parquet (the listing export) for the planned
            listings, and the number of resolved tracking events of the planned users
  mkinputs  write synthetic feature inputs into /work/in in the warehouse schemas: tracking views,
            optional listing_sellers.parquet (rows, optionally without a column), empty facts and orders
  rows      read-only: every row of the newest snapshot of a view under /features
"""

from __future__ import annotations

import glob
import json
import os
from datetime import datetime

import duckdb
import pyarrow as pa
import pyarrow.parquet as pq

PLAN = json.load(open("/work/plan.json"))  # noqa: SIM115

EVENTS = pa.schema(
    [
        ("event_id", pa.string()),
        ("event_type", pa.string()),
        ("listing_id", pa.string()),
        ("occurred_at", pa.timestamp("us")),
        ("ingested_at", pa.timestamp("us")),
        ("principal_type", pa.string()),
        ("user_key", pa.string()),
    ]
)
FACTS = pa.schema(
    [
        ("event_id", pa.string()),
        ("fact", pa.string()),
        ("user_id", pa.string()),
        ("listing_id", pa.string()),
        ("seller_id", pa.string()),
        ("rating", pa.int32()),
        ("occurred_at", pa.timestamp("us")),
        ("ingested_at", pa.timestamp("us")),
    ]
)
ORDERS = pa.schema(
    [
        ("event_id", pa.string()),
        ("order_id", pa.string()),
        ("listing_id", pa.string()),
        ("variant_id", pa.string()),
        ("seller_id", pa.string()),
        ("quantity", pa.int32()),
        ("unit_price", pa.int64()),
        ("currency", pa.string()),
        ("occurred_at", pa.timestamp("us")),
        ("status", pa.string()),
        ("buyer_id", pa.string()),
    ]
)
LISTINGS = pa.schema(
    [
        ("listing_id", pa.string()),
        ("seller_id", pa.string()),
        ("updated_at", pa.timestamp("us")),
        ("category_id", pa.string()),
        ("price", pa.int64()),
    ]
)


def probe() -> dict:
    out: dict = {"listings": {}, "columns": [], "users": {}}
    path = "/data/listing_sellers.parquet"
    if os.path.exists(path):
        rel = f"read_parquet('{path}')"
        out["columns"] = [c[0] for c in duckdb.execute(f"DESCRIBE SELECT * FROM {rel}").fetchall()]
        if "category_id" in out["columns"]:
            for lid in PLAN.get("listings", []):
                rows = duckdb.execute(
                    f"SELECT seller_id, category_id, price FROM {rel} WHERE listing_id = ?", [lid]
                ).fetchall()
                out["listings"][lid] = [
                    {"seller": r[0], "category": r[1], "price": r[2]} for r in rows
                ]
    resolved = "/data/tracking_events_resolved.parquet"
    if os.path.exists(resolved):
        for uk in PLAN.get("users", []):
            out["users"][uk] = duckdb.execute(
                f"SELECT COUNT(*) FROM read_parquet('{resolved}') WHERE user_key = ?", [uk]
            ).fetchone()[0]
    return out


def mkinputs() -> dict:
    out = "/work/in"
    os.makedirs(out, exist_ok=True)
    events = [
        {
            "event_id": f"fia-{i}",
            "event_type": e["type"],
            "listing_id": e["listing"],
            "occurred_at": datetime.fromisoformat(e["at"]),
            "ingested_at": datetime.fromisoformat(e["at"]),
            "principal_type": "USER",
            "user_key": e["user"],
        }
        for i, e in enumerate(PLAN.get("events", []))
    ]
    pq.write_table(
        pa.Table.from_pylist(events, schema=EVENTS), f"{out}/tracking_events_resolved.parquet"
    )
    pq.write_table(pa.Table.from_pylist([], schema=FACTS), f"{out}/engagement_facts.parquet")
    pq.write_table(pa.Table.from_pylist([], schema=ORDERS), f"{out}/order_facts.parquet")
    listings = PLAN.get("listings")
    if listings is not None:
        rows = [
            {
                "listing_id": x["id"],
                "seller_id": x.get("seller", "fia-seller"),
                "updated_at": datetime.fromisoformat(x["at"]),
                "category_id": x.get("category"),
                "price": x.get("price"),
            }
            for x in listings
        ]
        table = pa.Table.from_pylist(rows, schema=LISTINGS)
        for col in PLAN.get("drop_columns", []):
            table = table.drop_columns([col])
        pq.write_table(table, f"{out}/listing_sellers.parquet")
    return {"written": len(events)}


def rows() -> dict:
    view = PLAN["view"]
    paths = sorted(glob.glob(f"/features/{view}/v1/as_of=*.parquet"))
    if not paths:
        return {"found": False, "rows": []}
    return {"found": True, "rows": pq.read_table(paths[-1]).to_pylist()}


print(json.dumps({"probe": probe, "mkinputs": mkinputs, "rows": rows}[PLAN["op"]](), default=str))
