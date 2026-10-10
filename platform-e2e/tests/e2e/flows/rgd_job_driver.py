"""Runs INSIDE the platform-featurestore job image (mounted at /work by rgd_flow).

Helper for the ranking dataset of recsys-gbdt-trainer. Reads /work/plan.json, prints one JSON line.

ops:
  probe     read-only: how many resolved tracking events the planned users have in the analytics export
  mkinputs  write synthetic feature inputs into /work/in (warehouse schemas): tracking events with position and
            impression_id, empty facts and orders
  rows      read-only: the rows of the newest /features/datasets/rank_training/v1 snapshot for planned impressions
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
        ("position", pa.int32()),
        ("impression_id", pa.string()),
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


def probe() -> dict:
    out: dict = {"users": {}}
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
            "event_id": f"rgd-{i}",
            "event_type": e["type"],
            "listing_id": e["listing"],
            "occurred_at": datetime.fromisoformat(e["at"]),
            "ingested_at": datetime.fromisoformat(e["at"]),
            "principal_type": "USER",
            "user_key": e["user"],
            "position": e.get("position"),
            "impression_id": e.get("impression"),
        }
        for i, e in enumerate(PLAN.get("events", []))
    ]
    pq.write_table(
        pa.Table.from_pylist(events, schema=EVENTS), f"{out}/tracking_events_resolved.parquet"
    )
    pq.write_table(pa.Table.from_pylist([], schema=FACTS), f"{out}/engagement_facts.parquet")
    pq.write_table(pa.Table.from_pylist([], schema=ORDERS), f"{out}/order_facts.parquet")
    return {"written": len(events)}


def rows() -> dict:
    paths = sorted(glob.glob("/features/datasets/rank_training/v1/as_of=*.parquet"))
    if not paths:
        return {"found": False, "rows": []}
    wanted = set(PLAN.get("impressions", []))
    table = pq.read_table(paths[-1]).to_pylist()
    manifest = json.load(open(paths[-1][: -len(".parquet")] + ".manifest.json"))  # noqa: SIM115
    return {
        "found": True,
        "rows": [r for r in table if r["impression_id"] in wanted],
        "manifest": manifest,
    }


print(json.dumps({"probe": probe, "mkinputs": mkinputs, "rows": rows}[PLAN["op"]](), default=str))
