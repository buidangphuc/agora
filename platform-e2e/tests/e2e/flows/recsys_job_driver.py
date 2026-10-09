"""Runs INSIDE the platform-recsys job image (mounted at /work by recsys_job_flow).

It drives the real batch job as a black box against the stack's Redis and Qdrant,
inside an isolated namespace: a dedicated Redis DB and dedicated Qdrant collections.
The live serving data (Redis DB 0 and the default collections) is never touched.

1. It gives the job a governed dataset (featurestore-datasets): either a small
   `als_interactions` fixture it writes from /work/plan.json "events" (Parquet plus manifest,
   passed as DATASET_PATH), or, when "dataset_mounted" is set, the featurestore job's offline dir
   mounted at /dataset (DATASET_DIR). The job never reads raw tracking events.
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
import hashlib
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


# The tracking-event weights of als_interactions@v1 (spec: featurestore-datasets).
EVENT_WEIGHTS = {
    "impression": 0.5,
    "view": 1.0,
    "click": 2.0,
    "view_cart": 2.5,
    "add_to_cart": 5.0,
    "add_shipping_info": 6.0,
    "add_payment_info": 7.0,
    "begin_checkout": 8.0,
    "purchase": 10.0,
}


def _write_dataset() -> str | None:
    """Write a governed dataset fixture (parquet + manifest) from the plan's events."""
    rows = PLAN["events"]
    if not rows:
        return None
    df = pd.DataFrame(
        {
            "user_key": [r["user"] for r in rows],
            "listing_id": [r["listing"] for r in rows],
            "weight": [EVENT_WEIGHTS.get(r["event_type"], 0.0) for r in rows],
            "occurred_at": [datetime.fromtimestamp(r["ts"], tz=timezone.utc) for r in rows],
        }
    )
    grouped = (
        df.groupby(["user_key", "listing_id"], as_index=False)
        .agg(weight=("weight", "sum"), interactions=("weight", "size"), last=("occurred_at", "max"))
        .rename(columns={"last": "last_occurred_at"})
    )
    grouped = grouped[grouped["weight"] > 0]
    grouped["interactions"] = grouped["interactions"].astype("int64")
    as_of = datetime.now(timezone.utc)
    stamp = as_of.strftime("%Y%m%dT%H%M%SZ")
    directory = f"{WORK}/datasets/als_interactions/v1"
    os.makedirs(directory, exist_ok=True)
    path = f"{directory}/as_of={stamp}.parquet"
    grouped[["user_key", "listing_id", "weight", "interactions", "last_occurred_at"]].to_parquet(
        path, coerce_timestamps="ms", allow_truncated_timestamps=True
    )
    with open(path, "rb") as f:
        file_sha = hashlib.sha256(f.read()).hexdigest()
    manifest = {
        "name": "als_interactions",
        "version": 1,
        "as_of": as_of.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "window_days": 30,
        "rows": int(len(grouped)),
        "users": int(grouped["user_key"].nunique()),
        "items": int(grouped["listing_id"].nunique()),
        "definition_sha256": hashlib.sha256(b"e2e-fixture").hexdigest(),
        "input_watermark": as_of.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "file_sha256": file_sha,
        "file": os.path.basename(path),
    }
    with open(path[: -len(".parquet")] + ".manifest.json", "w") as f:
        json.dump(manifest, f)
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
            "parameters": meta.get("parameters"),
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
    fixture = _write_dataset()
    os.makedirs(f"{WORK}/empty-dataset", exist_ok=True)
    redis_host = os.environ.get("REDIS_HOST", "redis")
    redis = Redis(host=redis_host, port=6379, db=DB, decode_responses=True)
    qdrant = QdrantClient(url=os.environ.get("QDRANT_URL", "http://qdrant:6333"))
    _clean(redis, qdrant)
    base_env = {
        **os.environ,
        "WAREHOUSE_DRIVER": "duckdb",
        "REDIS_DATABASE": str(DB),
        "QDRANT_ITEM_COLLECTION": ITEMS,
        "QDRANT_USER_COLLECTION": USERS,
        "SPARK_MASTER": "local[1]",
        "ALS_RANK": "4",
        "ALS_MAX_ITER": "3",
        "TOP_N": "5",
    }
    if PLAN.get("dataset_mounted"):
        base_env["DATASET_DIR"] = "/dataset/datasets/als_interactions/v1"
        base_env.pop("DATASET_PATH", None)
    elif fixture:
        base_env["DATASET_PATH"] = fixture
    results = []
    try:
        for run_env in PLAN["runs"]:
            env = {**base_env, **{k: v for k, v in run_env.items() if v is not None}}
            for key, value in run_env.items():
                if value is None:
                    env.pop(key, None)
            proc = subprocess.run(
                [sys.executable, "-m", "recsys"],
                env=env,
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
                    "log_tail": out[-3000:],
                }
            )
    finally:
        _clean(redis, qdrant)
    json.dump({"runs": results}, open(f"{WORK}/result.json", "w"))  # noqa: SIM115
    return 0


if __name__ == "__main__":
    sys.exit(main())
