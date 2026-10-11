"""Runs INSIDE the platform-featurestore job image (mounted at /work by fsd_job_flow).

A read-only inspector for featurestore-datasets: the host venv has neither duckdb nor pyarrow.
It reads /work/plan.json, prints one JSON line on stdout and never writes to /data or /features.

ops:
  probe     the analytics export: resolved rows per (user_key, listing) and engagement facts
  datasets  the offline dir: every governed dataset file with its manifest, its real SHA-256,
            its columns and row count, and the rows of the requested user keys
"""

from __future__ import annotations

import glob
import hashlib
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
    out: dict = {"resolved": {}, "facts": {}}
    resolved = f"{DATA}/tracking_events_resolved.parquet"
    if os.path.exists(resolved):
        for uk, lid in PLAN.get("pairs", []):
            out["resolved"][f"{uk}|{lid}"] = _query(
                f"SELECT COUNT(*) FROM read_parquet('{resolved}') "
                "WHERE user_key = ? AND listing_id = ?",
                [uk, lid],
            )[0][0]
    facts = f"{DATA}/engagement_facts.parquet"
    if os.path.exists(facts):
        for uid, lid in PLAN.get("pairs", []):
            out["facts"][f"{uid}|{lid}"] = _query(
                f"SELECT COUNT(*) FROM read_parquet('{facts}') WHERE user_id = ? AND listing_id = ?",
                [uid, lid],
            )[0][0]
    return out


def _sha256(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def datasets() -> dict:
    out: dict = {"files": []}
    pattern = f"{FEATURES}/datasets/als_interactions/v1/as_of=*.parquet"
    for path in sorted(glob.glob(pattern)):
        manifest_path = path[: -len(".parquet")] + ".manifest.json"
        rel = f"read_parquet('{path}')"
        users: dict = {}
        for uk in PLAN.get("users", []):
            users[uk] = [
                {"listing_id": r[0], "weight": r[1], "interactions": r[2]}
                for r in _query(
                    f"SELECT listing_id, weight, interactions FROM {rel} WHERE user_key = ?", [uk]
                )
            ]
        manifest = None
        if os.path.exists(manifest_path):
            manifest = json.load(open(manifest_path))  # noqa: SIM115
        out["files"].append(
            {
                "path": os.path.relpath(path, FEATURES),
                "sha256": _sha256(path),
                "columns": [c[0] for c in _query(f"DESCRIBE SELECT * FROM {rel}")],
                "rows": _query(f"SELECT COUNT(*) FROM {rel}")[0][0],
                "manifest": manifest,
                "users": users,
            }
        )
    return out


if __name__ == "__main__":
    print(json.dumps({"probe": probe, "datasets": datasets}[PLAN["op"]](), default=str))
    sys.exit(0)
