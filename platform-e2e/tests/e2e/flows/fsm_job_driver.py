"""Runs INSIDE the platform-featurestore job image (mounted at /work by fsm_job_flow).

A read-only inspector: the host venv has neither duckdb nor pyarrow, the image has both.
It reads /work/plan.json, prints one JSON line on stdout and never writes to /data or /features.

ops:
  probe     the analytics export: mtimes, resolved rows / user_keys per listing, engagement facts
  snapshot  the offline dir: every manifest and, per view, the snapshots with their entity ids
"""

from __future__ import annotations

import glob
import json
import os
import sys

import duckdb

PLAN = json.load(open("/work/plan.json"))  # noqa: SIM115
DATA = "/data"
FEATURES = "/features"


def _query(sql: str, params: list | None = None) -> list:
    return duckdb.execute(sql, params or []).fetchall()


def probe() -> dict:
    out: dict = {"files": {}, "resolved": {}, "users": {}, "facts": {}}
    for name in ("tracking_events_resolved", "engagement_facts", "order_facts"):
        path = f"{DATA}/{name}.parquet"
        if os.path.exists(path):
            out["files"][name] = os.path.getmtime(path)
    resolved = f"{DATA}/tracking_events_resolved.parquet"
    if os.path.exists(resolved):
        for lid in PLAN.get("listings", []):
            rows = _query(
                f"SELECT user_key, COUNT(*) FROM read_parquet('{resolved}') "
                "WHERE listing_id = ? GROUP BY 1",
                [lid],
            )
            out["resolved"][lid] = {k: n for k, n in rows}
        for uk in PLAN.get("users", []):
            out["users"][uk] = _query(
                f"SELECT COUNT(*) FROM read_parquet('{resolved}') WHERE user_key = ?", [uk]
            )[0][0]
    facts = f"{DATA}/engagement_facts.parquet"
    if os.path.exists(facts):
        for pair in PLAN.get("facts", []):
            uid, lid = pair
            out["facts"][f"{uid}|{lid}"] = _query(
                f"SELECT COUNT(*) FROM read_parquet('{facts}') WHERE user_id = ? AND listing_id = ?",
                [uid, lid],
            )[0][0]
    return out


def snapshot() -> dict:
    out: dict = {"manifests": [], "snapshots": {}}
    for path in sorted(glob.glob(f"{FEATURES}/runs/*/manifest.json")):
        out["manifests"].append(json.load(open(path)))  # noqa: SIM115
    entity = PLAN.get("entities", {})
    for path in sorted(glob.glob(f"{FEATURES}/*/v*/as_of=*.parquet")):
        view = path.split("/")[-3]
        col = entity.get(view)
        rel = f"read_parquet('{path}')"
        cols = [c[0] for c in _query(f"DESCRIBE SELECT * FROM {rel}")]
        ids = []
        if col and col in cols:
            ids = [r[0] for r in _query(f"SELECT {col} FROM {rel}")]
        out["snapshots"].setdefault(view, []).append(
            {
                "path": os.path.relpath(path, FEATURES),
                "rows": _query(f"SELECT COUNT(*) FROM {rel}")[0][0],
                "columns": cols,
                "ids": ids,
            }
        )
    return out


if __name__ == "__main__":
    print(json.dumps({"probe": probe, "snapshot": snapshot}[PLAN["op"]](), default=str))
    sys.exit(0)
