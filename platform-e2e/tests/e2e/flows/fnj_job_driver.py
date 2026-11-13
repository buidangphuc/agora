"""Runs INSIDE the platform-forecast job image (mounted at /work by fnj_job_flow).

The host venv has neither duckdb nor pyarrow; the image has duckdb. Reads /work/plan.json and prints
one JSON line on stdout.

ops:
  mkexport  writes /work/in/order_facts.parquet in the warehouse schema from synthetic order lines
            ({seller, listing, day, qty, status}); `file: false` writes no file (empty input dir),
            `drop_columns` leaves named columns out of the file
"""

from __future__ import annotations

import json
import os

import duckdb

PLAN = json.load(open("/work/plan.json"))  # noqa: SIM115

COLUMNS = [
    ("event_id", "VARCHAR"),
    ("order_id", "VARCHAR"),
    ("listing_id", "VARCHAR"),
    ("variant_id", "VARCHAR"),
    ("seller_id", "VARCHAR"),
    ("quantity", "INTEGER"),
    ("unit_price", "BIGINT"),
    ("currency", "VARCHAR"),
    ("occurred_at", "TIMESTAMP"),
    ("status", "VARCHAR"),
    ("buyer_id", "VARCHAR"),
]


def mkexport() -> dict:
    out = "/work/in"
    os.makedirs(out, exist_ok=True)
    if PLAN.get("file", True) is False:
        return {"written": 0}
    con = duckdb.connect()
    con.execute(f"CREATE TABLE t ({', '.join(f'{n} {t}' for n, t in COLUMNS)})")
    rows = [
        (
            f"fnj-evt-{i}",
            f"fnj-ord-{i}",
            ln["listing"],
            "",
            ln["seller"],
            ln["qty"],
            1,
            "VND",
            f"{ln['day']} 12:00:00",
            ln.get("status", "PAID"),
            f"fnj-buyer-{i}",
        )
        for i, ln in enumerate(PLAN["lines"])
    ]
    if rows:
        con.executemany("INSERT INTO t VALUES (?,?,?,?,?,?,?,?,CAST(? AS TIMESTAMP),?,?)", rows)
    keep = [n for n, _ in COLUMNS if n not in PLAN.get("drop_columns", [])]
    con.execute(
        f"COPY (SELECT {', '.join(keep)} FROM t) TO '{out}/order_facts.parquet' (FORMAT PARQUET)"
    )
    return {"written": len(rows)}


print(json.dumps({"mkexport": mkexport}[PLAN["op"]]()))
