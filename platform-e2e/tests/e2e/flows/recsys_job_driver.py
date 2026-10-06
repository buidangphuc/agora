"""Runs INSIDE the platform-recsys job image (mounted at /work by recsys_job_flow).

It drives the real batch job as a black box against the stack's Redis and Qdrant,
inside an isolated namespace: a dedicated Redis DB and dedicated Qdrant collections.
The live serving data (Redis DB 0 and the default collections) is never touched.

1. It turns /work/plan.json "events" into the tracking_events Parquet the job reads.
2. It starts from a clean namespace (the Redis DB is flushed and the collections
   are dropped).
3. For each entry in "runs" it executes `python -m recsys` with that run's
   environment overrides, records the exit code and the job's own summary (its
   "recsys done: {...}" log line), and snapshots the stores.
4. It writes everything to /work/result.json and cleans the namespace up again.

Only the image's own dependencies are used (pandas, redis, qdrant-client).
"""

from __future__ import annotations

import ast
import json
import os
import subprocess
import sys
from datetime import datetime, timezone

import pandas as pd
from qdrant_client import QdrantClient
from redis import Redis

WORK = "/work"
PLAN = json.load(open(f"{WORK}/plan.json"))  # noqa: SIM115
NS = PLAN["namespace"]
ITEMS = f"{NS}_items"
USERS = f"{NS}_users"
DB = int(PLAN["redis_db"])
SUMMARY_MARK = "recsys done: "


def _write_parquet() -> str:
    rows = PLAN["events"]
    df = pd.DataFrame(
        {
            "event_type": [r["event_type"] for r in rows],
            "principal_id": [r["user"] for r in rows],
            "anonymous_id": ["" for _ in rows],
            "listing_id": [r["listing"] for r in rows],
            "occurred_at": [datetime.fromtimestamp(r["ts"], tz=timezone.utc) for r in rows],
        }
    )
    path = f"{WORK}/tracking_events.parquet"
    df.to_parquet(path, coerce_timestamps="ms", allow_truncated_timestamps=True)
    return path


def _clean(redis: Redis, qdrant: QdrantClient) -> None:
    redis.flushdb()
    for name in (ITEMS, USERS):
        if qdrant.collection_exists(name):
            qdrant.delete_collection(name)


def _points(qdrant: QdrantClient, name: str) -> dict:
    if not qdrant.collection_exists(name):
        return {"count": 0, "model_versions": []}
    points, _ = qdrant.scroll(collection_name=name, limit=10_000, with_payload=True)
    return {
        "count": len(points),
        "model_versions": sorted({(p.payload or {}).get("model_version", "") for p in points}),
    }


def _snapshot(redis: Redis, qdrant: QdrantClient) -> dict:
    models = {}
    for key in redis.scan_iter("recs:model:meta:*"):
        meta = json.loads(redis.get(key))
        models[meta["model_version"]] = {
            "status": meta.get("status"),
            "metrics": meta.get("metrics"),
        }
    return {
        "champion": redis.get("recs:model:champion"),
        "serving_model_version": redis.get("recs:v1:model_version"),
        "popular_cached": redis.exists("recs:v1:popular") == 1,
        "models": models,
        "items": _points(qdrant, ITEMS),
        "users": _points(qdrant, USERS),
    }


def main() -> int:
    parquet = _write_parquet()
    redis_host = os.environ.get("REDIS_HOST", "redis")
    redis = Redis(host=redis_host, port=6379, db=DB, decode_responses=True)
    qdrant = QdrantClient(url=os.environ.get("QDRANT_URL", "http://qdrant:6333"))
    _clean(redis, qdrant)
    base_env = {
        **os.environ,
        "WAREHOUSE_DRIVER": "duckdb",
        "WAREHOUSE_PARQUET_PATH": parquet,
        "REDIS_DATABASE": str(DB),
        "QDRANT_ITEM_COLLECTION": ITEMS,
        "QDRANT_USER_COLLECTION": USERS,
        "SPARK_MASTER": "local[1]",
        "ALS_RANK": "4",
        "ALS_MAX_ITER": "3",
        "TOP_N": "5",
    }
    results = []
    try:
        for run_env in PLAN["runs"]:
            proc = subprocess.run(
                [sys.executable, "-m", "recsys"],
                env={**base_env, **run_env},
                capture_output=True,
                text=True,
                check=False,
            )
            out = proc.stdout + proc.stderr
            summary = None
            for line in out.splitlines():
                if SUMMARY_MARK in line:
                    summary = ast.literal_eval(line.split(SUMMARY_MARK, 1)[1])
            results.append(
                {
                    "exit_code": proc.returncode,
                    "summary": summary,
                    "state": _snapshot(redis, qdrant),
                    "log_tail": out[-3000:] if summary is None else "",
                }
            )
    finally:
        _clean(redis, qdrant)
    json.dump({"runs": results}, open(f"{WORK}/result.json", "w"))  # noqa: SIM115
    return 0


if __name__ == "__main__":
    sys.exit(main())
