"""Runs INSIDE the platform-recsys job image (mounted at /work by recsys_job.py).

Black-box driver for the recsys-auto-train scenarios. It acts on the stack's LIVE serving data
(Redis DB 0, the default Qdrant collections and aliases) because the scenarios assert what team-ai
`Recommend` answers. It reuses `_write_dataset`, `_fixture_events` and `_reset_live` of
tests/e2e/flows/recsys_job_driver.py (copied to /work/driver.py by the host side).

/work/plan.json carries the driver's own keys (namespace, redis_db, live=true, buyer_id, events,
runs: only read when driver.py is imported) plus "actions", a list of:

    {"op": "reset"}                              forget every generation and the Redis registry
    {"op": "job", "dataset": "good_a|better_b|keyed|none", "age_hours": 0, "env": {...},
     "command": null}                            run `python -m recsys [command]` once

Per action it reports the exit code, the wall-clock seconds of the job process, and its full output.
It writes /work/result.json. Only the image's own dependencies are used (pandas, redis, qdrant).
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import time
from datetime import datetime, timedelta, timezone

sys.path.insert(0, "/work")
import driver  # noqa: E402  (the copy of recsys_job_driver.py)
from qdrant_client import QdrantClient  # noqa: E402
from redis import Redis  # noqa: E402

WORK = "/work"
PLAN = json.load(open(f"{WORK}/plan.json"))  # noqa: SIM115


def _keyed_events() -> list[dict]:
    """RAT-25: fifty users u-0001 .. u-0050 in the three taste clusters of `better_b`.

    Same shape as driver._fixture_events("better_b"): each user interacts with all five items of
    its cluster, the last (most recent) one being an item its cluster-mates have too.
    """
    now = time.time()
    cycle = ("view", "click", "add_to_cart", "click")
    rows: list[dict] = []
    for n in range(50):
        cluster, slot = n % 3, n // 3
        for step in range(5):
            rows.append(
                {
                    "user": f"u-{n + 1:04d}",
                    "listing": f"rgp-item-{cluster}-{(slot + step) % 5}",
                    "event_type": cycle[step % len(cycle)],
                    "ts": now - (5 - step) * 3600,
                }
            )
    return rows


def _events(dataset: str) -> list[dict]:
    if dataset == "keyed":
        return _keyed_events()
    return driver._fixture_events(dataset)


def _backdate(directory: str, age_hours: float) -> None:
    """Make the newest snapshot `age_hours` old: file name stamp, manifest and mtime."""
    when = datetime.now(timezone.utc) - timedelta(hours=age_hours)
    stamp = when.strftime("%Y%m%dT%H%M%SZ")
    iso = when.strftime("%Y-%m-%dT%H:%M:%SZ")
    manifest_path = next(
        os.path.join(directory, f) for f in os.listdir(directory) if f.endswith(".manifest.json")
    )
    manifest = json.load(open(manifest_path))  # noqa: SIM115
    old = os.path.join(directory, manifest["file"])
    new_name = re.sub(r"as_of=[0-9TZ]+", f"as_of={stamp}", manifest["file"])
    new = os.path.join(directory, new_name)
    os.rename(old, new)
    os.remove(manifest_path)
    manifest.update({"file": new_name, "as_of": iso, "input_watermark": iso})
    with open(new[: -len(".parquet")] + ".manifest.json", "w") as f:
        json.dump(manifest, f)
    ts = when.timestamp()
    for name in os.listdir(directory):
        os.utime(os.path.join(directory, name), (ts, ts))


def _job_env(index: int, action: dict) -> dict:
    env = {
        **os.environ,
        "WAREHOUSE_DRIVER": "duckdb",
        "REDIS_DATABASE": "0",
        "QDRANT_ITEM_COLLECTION": "item_als_vectors",
        "QDRANT_USER_COLLECTION": "user_als_vectors",
        "SPARK_MASTER": "local[1]",
        "ALS_RANK": "4",
        "ALS_MAX_ITER": "3",
        "TOP_N": "5",
    }
    env.pop("DATASET_PATH", None)
    dataset = action.get("dataset", "good_a")
    if dataset != "none":
        root = f"{WORK}/ds-{index}"
        driver._write_dataset(_events(dataset), root)
        directory = f"{root}/datasets/als_interactions/v1"
        if action.get("age_hours"):
            _backdate(directory, float(action["age_hours"]))
        env["DATASET_DIR"] = directory
    for key, value in (action.get("env") or {}).items():
        if value is None:
            env.pop(key, None)
        else:
            env[key] = str(value)
    return env


def main() -> int:
    redis = Redis(
        host=os.environ.get("REDIS_HOST", "redis"), port=6379, db=0, decode_responses=True
    )
    qdrant = QdrantClient(url=os.environ.get("QDRANT_URL", "http://qdrant:6333"))
    results = []
    for index, action in enumerate(PLAN["actions"]):
        if action["op"] == "reset":
            driver._reset_live(redis, qdrant)
            results.append({"op": "reset"})
            continue
        env = _job_env(index, action)
        argv = [
            sys.executable,
            "-m",
            "recsys",
            *([action["command"]] if action.get("command") else []),
        ]
        started = time.monotonic()
        proc = subprocess.run(argv, env=env, capture_output=True, text=True, check=False)
        results.append(
            {
                "op": "job",
                "exit_code": proc.returncode,
                "duration_s": time.monotonic() - started,
                "log": proc.stdout + proc.stderr,
            }
        )
    json.dump({"results": results}, open(f"{WORK}/result.json", "w"))  # noqa: SIM115
    return 0


if __name__ == "__main__":
    sys.exit(main())
