"""Runs INSIDE the platform-featurestore job image (mounted at /work by ofb_flow).

The host venv has neither duckdb nor pyarrow; the image has both. Reads /work/plan.json and prints
one JSON line on stdout.

ops:
  orders    read-only: the rows of /data/order_facts.parquet for the planned order ids
  mkinputs  writes the three feature inputs into /work/in from a list of synthetic order lines
            (events and facts empty), in the warehouse schemas
"""

from __future__ import annotations

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


def orders() -> dict:
    path = "/data/order_facts.parquet"
    if not os.path.exists(path):
        return {"columns": [], "rows": []}
    rel = f"read_parquet('{path}')"
    cols = [c[0] for c in duckdb.execute(f"DESCRIBE SELECT * FROM {rel}").fetchall()]
    rows = []
    if "buyer_id" in cols:
        for oid in PLAN.get("order_ids", []):
            rows += [
                {"order_id": r[0], "listing_id": r[1], "buyer_id": r[2]}
                for r in duckdb.execute(
                    f"SELECT order_id, listing_id, buyer_id FROM {rel} WHERE order_id = ?", [oid]
                ).fetchall()
            ]
    return {"columns": cols, "rows": rows}


def mkinputs() -> dict:
    out = "/work/in"
    os.makedirs(out, exist_ok=True)
    lines = []
    for i, o in enumerate(PLAN["lines"]):
        lines.append(
            {
                "event_id": f"ofb-{i}-{o['order']}",
                "order_id": o["order"],
                "listing_id": "ofb-listing",
                "variant_id": "",
                "seller_id": "ofb-seller",
                "quantity": 1,
                "unit_price": 1,
                "currency": "VND",
                "occurred_at": datetime.fromisoformat(o["at"]),
                "status": o.get("status", "PAID"),
                "buyer_id": o.get("buyer"),
            }
        )
    pq.write_table(
        pa.Table.from_pylist([], schema=EVENTS), f"{out}/tracking_events_resolved.parquet"
    )
    pq.write_table(pa.Table.from_pylist([], schema=FACTS), f"{out}/engagement_facts.parquet")
    pq.write_table(pa.Table.from_pylist(lines, schema=ORDERS), f"{out}/order_facts.parquet")
    return {"written": len(lines)}


print(json.dumps({"orders": orders, "mkinputs": mkinputs}[PLAN["op"]]()))
