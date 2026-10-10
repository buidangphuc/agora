"""Runs INSIDE the platform-recsys job image (mounted at /work by mlr_job_flow).

Black-box driver for the ML recsys changes (add-recsys-nearline-signals, add-recsys-drift-monitoring,
wire-two-tower-batch-pipeline). It executes the real entrypoints (`python -m recsys`,
`python -m recsys.nearline`) against the stack's Redis, Qdrant and Kafka inside an isolated namespace: a
dedicated Redis DB and Qdrant collections named after it. Serving data (DB 0, default collections) is never
touched. /work/plan.json holds {"namespace", "redis_db", "steps": [...]}; each step is one of

  {"op": "recsys", "env": {..}, "features": "standard"|"with_blank"|"none"|null, "read": ["/path"]}
      run the batch job on the clustered dataset fixture; null env values unset a variable
  {"op": "nearline", "env": {..}}
      run the nearline consumer in drain mode (earliest offset, unique group) and dump recs:nearline:*
  {"op": "redis_set", "key": k, "value": v, "ex": seconds}
  {"op": "redis_dump", "pattern": "recs:nearline:*"}

and the result of every step goes to /work/result.json.
"""

from __future__ import annotations

import ast
import hashlib
import json
import os
import subprocess
import sys
import time
from datetime import datetime, timedelta, timezone

import pandas as pd
from qdrant_client import QdrantClient
from qdrant_client.models import DeleteAlias, DeleteAliasOperation
from redis import Redis

WORK = "/work"
PLAN = json.load(open(f"{WORK}/plan.json"))  # noqa: SIM115
NS = PLAN["namespace"]
DB = int(PLAN["redis_db"])
ITEMS, USERS, TOWER = f"{NS}_items", f"{NS}_users", f"{NS}_tower"
SUMMARY_MARK = "recsys done: "
PREFIXES = (ITEMS, USERS, TOWER)

ITEM_COLS = ["listing_id", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "review_count",
             "avg_rating", "ctr_7d"]  # fmt: skip
USER_COLS = ["user_key", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "follows_current",
             "paid_orders_30d"]  # fmt: skip


def _dataset(directory: str) -> str:
    """Two taste clusters over six listings (3 users each): a governed als_interactions fixture."""
    now = datetime.now(timezone.utc)
    rows = []
    clusters = [
        ("u1", "abc"),
        ("u2", "abc"),
        ("u3", "abc"),
        ("u4", "def"),
        ("u5", "def"),
        ("u6", "def"),
    ]
    for user, items in clusters:
        for h, ch in enumerate(items):
            rows.append(
                {"user_key": f"mlr-{user}", "listing_id": f"l{ch}", "weight": 3.0 - h, "interactions": 1,
                 "last_occurred_at": now - timedelta(hours=h + 1)}
            )  # fmt: skip
    df = pd.DataFrame(rows).astype({"weight": "float64", "interactions": "int32"})
    os.makedirs(directory, exist_ok=True)
    path = f"{directory}/as_of=20261005T020000Z.parquet"
    df.to_parquet(path, index=False, coerce_timestamps="ms", allow_truncated_timestamps=True)
    sha = hashlib.sha256(open(path, "rb").read()).hexdigest()  # noqa: SIM115
    manifest = {"name": "als_interactions", "version": 1, "as_of": "20261005T020000Z", "window_days": 30,
                "rows": len(df), "users": int(df["user_key"].nunique()), "items": int(df["listing_id"].nunique()),
                "definition_sha256": "d" * 64, "input_watermark": "2026-10-05T02:00:00Z",
                "file_sha256": sha, "file": os.path.basename(path)}  # fmt: skip
    json.dump(manifest, open(path[: -len(".parquet")] + ".manifest.json", "w"))  # noqa: SIM115
    return path


def _features(kind: str | None, root: str) -> dict[str, str]:
    """item_popularity@v1 / user_activity@v2 snapshot fixtures; returns the env that points at them."""
    item_dir, user_dir = f"{root}/item_popularity/v1", f"{root}/user_activity/v2"
    os.makedirs(item_dir, exist_ok=True)
    os.makedirs(user_dir, exist_ok=True)
    if kind in ("standard", "with_blank"):
        items = [
            [f"l{c}", 10 * (i + 1), i, i // 2, i, 0, None, 0.1 * i] for i, c in enumerate("abcdef")
        ]
        items.append(
            ["mlr-cold", 0, 0, 0, 2, 1, 4.0, 0.0]
        )  # favourited and reviewed, no interactions
        if kind == "with_blank":
            items.append(
                ["mlr-blank", 0, 0, 0, 0, 0, None, 0.0]
            )  # no signal at all: embeds to zero untrained
        pd.DataFrame(items, columns=ITEM_COLS).to_parquet(
            f"{item_dir}/as_of=20261005T000000Z.parquet", index=False
        )
        users = [[f"mlr-u{n}", 5 * n, n, 0, 0, 0, n % 2] for n in range(1, 7)]
        pd.DataFrame(users, columns=USER_COLS).to_parquet(
            f"{user_dir}/as_of=20261005T000000Z.parquet", index=False
        )
    return {"ITEM_FEATURES_DIR": item_dir, "USER_FEATURES_DIR": user_dir}


def _clean(redis: Redis, qdrant: QdrantClient) -> None:
    redis.flushdb()
    aliases = {a.alias_name: a.collection_name for a in qdrant.get_aliases().aliases}
    for alias in (ITEMS, USERS):
        if alias in aliases:
            qdrant.update_collection_aliases(
                change_aliases_operations=[
                    DeleteAliasOperation(delete_alias=DeleteAlias(alias_name=alias))
                ]
            )
    for c in qdrant.get_collections().collections:
        if c.name.startswith(PREFIXES):
            qdrant.delete_collection(c.name)


def _collections(qdrant: QdrantClient) -> dict[str, list[dict]]:
    out = {}
    for c in sorted(qdrant.get_collections().collections, key=lambda c: c.name):
        if c.name.startswith(PREFIXES):
            points, _ = qdrant.scroll(
                collection_name=c.name, limit=10_000, with_payload=True, with_vectors=True
            )
            out[c.name] = [
                {"listing_id": (p.payload or {}).get("listing_id"), "user_key": (p.payload or {}).get("user_key"),
                 "model_version": (p.payload or {}).get("model_version"), "vector": list(p.vector)}
                for p in points
            ]  # fmt: skip
    return out


def _dump(redis: Redis, pattern: str) -> dict:
    out: dict = {}
    for key in sorted(redis.scan_iter(pattern)):
        if ":seen:" in key or ":session:" in key:
            continue
        kind = redis.type(key)
        if kind == "zset":
            out[key] = {
                "type": kind,
                "members": redis.zrevrange(key, 0, -1, withscores=True),
                "ttl": redis.ttl(key),
            }
        elif kind == "hash":
            out[key] = {"type": kind, "fields": redis.hgetall(key), "ttl": redis.ttl(key)}
        else:
            out[key] = {"type": kind, "value": redis.get(key), "ttl": redis.ttl(key)}
    return out


def _models(redis: Redis) -> dict:
    return {
        json.loads(redis.get(k))["model_version"]: json.loads(redis.get(k))
        for k in redis.scan_iter("recs:model:meta:*")
    }


def _run(argv: list[str], env: dict) -> tuple[int, str]:
    proc = subprocess.run(argv, env=env, capture_output=True, text=True, check=False)
    return proc.returncode, proc.stdout + proc.stderr


def main() -> int:
    redis = Redis(
        host=os.environ.get("REDIS_HOST", "redis"), port=6379, db=DB, decode_responses=True
    )
    qdrant = QdrantClient(url=os.environ.get("QDRANT_URL", "http://qdrant:6333"))
    _clean(redis, qdrant)
    dataset = _dataset(f"{WORK}/dataset")
    base = {
        **os.environ,
        "REDIS_DATABASE": str(DB),
        "QDRANT_ITEM_COLLECTION": ITEMS,
        "QDRANT_USER_COLLECTION": USERS,
        "QDRANT_TWO_TOWER_COLLECTION": TOWER,
        "SPARK_MASTER": "local[1]",
        "ALS_RANK": "4",
        "ALS_MAX_ITER": "3",
        "TOP_N": "2",
        "TWO_TOWER_DIM": "8",
        "DATASET_PATH": dataset,
        "PROMOTION_FORCE": "true",
        # a six-listing catalogue gives every user the same top-2: keep the structural gate out of the way
        "GATE_MAX_LIST_OVERLAP": "1.0",
        "GATE_MIN_ITEM_COVERAGE": "0",
        "GATE_MIN_USER_COVERAGE": "0",
    }
    nonce = format(int(time.time() * 1000), "x")
    results = []
    try:
        for index, step in enumerate(PLAN["steps"]):
            op = step["op"]
            env = dict(base)
            for k, v in (step.get("env") or {}).items():
                env.pop(k, None) if v is None else env.__setitem__(k, v)
            if op == "recsys":
                env.update(_features(step.get("features"), f"{WORK}/feat-{index}"))
                env.setdefault("MODEL_VERSION", f"als-mlr-{nonce}-{index}")
                code, out = _run([sys.executable, "-m", "recsys"], env)
                summary = None
                for line in out.splitlines():
                    if SUMMARY_MARK in line:
                        summary = ast.literal_eval(line.split(SUMMARY_MARK, 1)[1])
                files = {
                    p: open(p).read() for p in step.get("read", []) if os.path.exists(p)
                }  # noqa: SIM115
                results.append({
                    "op": op, "exit_code": code, "summary": summary, "log_tail": out[-3000:],
                    "models": _models(redis), "champion": redis.get("recs:model:champion"),
                    "serving": redis.get("recs:v1:serving"), "previous": redis.get("recs:v1:previous"),
                    "collections": _collections(qdrant), "files": files, "nearline": _dump(redis, "recs:nearline:*"),
                    "model_version": env["MODEL_VERSION"],
                })  # fmt: skip
            elif op == "nearline":
                env.update(
                    {"KAFKA_BROKERS": "redpanda:9092", "NEARLINE_START_OFFSET": "earliest",
                     "NEARLINE_CONSUMER_GROUP": f"mlr-e2e-{nonce}-{index}", "NEARLINE_IDLE_EXIT_SECONDS": "20"}
                )  # fmt: skip
                code, out = _run([sys.executable, "-m", "recsys.nearline"], env)
                results.append({"op": op, "exit_code": code, "log_tail": out[-3000:],
                                "nearline": _dump(redis, "recs:nearline:*")})  # fmt: skip
            elif op == "redis_set":
                redis.set(step["key"], step["value"], ex=step.get("ex"))
                results.append({"op": op, "nearline": _dump(redis, "recs:nearline:*")})
            elif op == "redis_zadd":
                redis.zadd(step["key"], {step["member"]: step["score"]})
                redis.expire(step["key"], step.get("ex", 3600))
                results.append({"op": op, "nearline": _dump(redis, "recs:nearline:*")})
            else:
                raise ValueError(f"unknown op {op}")
    finally:
        json.dump({"steps": results}, open(f"{WORK}/result.json", "w"))  # noqa: SIM115
        _clean(redis, qdrant)
    return 0


if __name__ == "__main__":
    sys.exit(main())
