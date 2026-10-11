"""Runs INSIDE the platform-recsys job image (mounted at /work by fir_job_flow).

Black-box driver for featurestore-item-attributes on the recsys side: it executes the real
`python -m recsys` against the stack's Redis and Qdrant in an isolated namespace (a dedicated Redis
DB and Qdrant collections named after it; serving data is never touched). /work/plan.json holds
{"namespace", "redis_db", "steps": [{"env": {..}, "features": "attrs"|"no_attrs"|"none"}]}.

Feature fixtures (item_popularity@v1 / user_activity@v2 always, except "none"):
  attrs     + item_attributes@v1 (the six interacted listings in two categories, and two listings that exist
            only here: fir-cold-a and fir-cold-b, different categories, same price) and user_preferences@v1
  no_attrs  popularity and activity snapshots only
  none      no snapshot at all
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
CAT_X, CAT_Y = "fir-cat-x", "fir-cat-y"


def _dataset(directory: str) -> str:
    """Two taste clusters over six listings (3 users each): a governed als_interactions fixture."""
    now = datetime.now(timezone.utc)
    rows = []
    for user, items in [
        ("u1", "abc"),
        ("u2", "abc"),
        ("u3", "abc"),
        ("u4", "def"),
        ("u5", "def"),
        ("u6", "def"),
    ]:
        for h, ch in enumerate(items):
            rows.append({"user_key": f"fir-{user}", "listing_id": f"l{ch}", "weight": 3.0 - h, "interactions": 1,
                         "last_occurred_at": now - timedelta(hours=h + 1)})  # fmt: skip
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


def _features(kind: str, root: str) -> dict[str, str]:
    dirs = {"ITEM_FEATURES_DIR": f"{root}/item_popularity/v1", "USER_FEATURES_DIR": f"{root}/user_activity/v2",
            "ITEM_ATTRIBUTES_DIR": f"{root}/item_attributes/v1",
            "USER_PREFERENCES_DIR": f"{root}/user_preferences/v1"}  # fmt: skip
    for d in dirs.values():
        os.makedirs(d, exist_ok=True)
    stamp = "as_of=20261005T000000Z.parquet"
    if kind in ("attrs", "no_attrs"):
        items = [
            [f"l{c}", 10 * (i + 1), i, i // 2, i, 0, None, 0.1 * i] for i, c in enumerate("abcdef")
        ]
        pd.DataFrame(items, columns=ITEM_COLS).to_parquet(
            f"{dirs['ITEM_FEATURES_DIR']}/{stamp}", index=False
        )
        users = [[f"fir-u{n}", 5 * n, n, 0, 0, 0, n % 2] for n in range(1, 7)]
        pd.DataFrame(users, columns=USER_COLS).to_parquet(
            f"{dirs['USER_FEATURES_DIR']}/{stamp}", index=False
        )
    if kind == "attrs":
        attrs = [[f"l{c}", "fir-seller", CAT_X if c in "abc" else CAT_Y, 80_000 + 1_000 * i]
                 for i, c in enumerate("abcdef")]  # fmt: skip
        attrs += [
            ["fir-cold-a", "fir-seller", CAT_X, 50_000],
            ["fir-cold-b", "fir-seller", CAT_Y, 50_000],
        ]
        pd.DataFrame(attrs, columns=["listing_id", "seller_id", "category_id", "price"]).to_parquet(
            f"{dirs['ITEM_ATTRIBUTES_DIR']}/{stamp}", index=False
        )
        prefs = [[f"fir-u{n}", CAT_X if n <= 3 else CAT_Y] for n in range(1, 7)]
        pd.DataFrame(prefs, columns=["user_key", "preferred_categories"]).to_parquet(
            f"{dirs['USER_PREFERENCES_DIR']}/{stamp}", index=False
        )
    return dirs


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
            out[c.name] = [{"listing_id": (p.payload or {}).get("listing_id"),
                            "model_version": (p.payload or {}).get("model_version"),
                            "vector": list(p.vector)} for p in points]  # fmt: skip
    return out


def _models(redis: Redis) -> dict:
    return {
        json.loads(redis.get(k))["model_version"]: json.loads(redis.get(k))
        for k in redis.scan_iter("recs:model:meta:*")
    }


def main() -> int:
    redis = Redis(
        host=os.environ.get("REDIS_HOST", "redis"), port=6379, db=DB, decode_responses=True
    )
    qdrant = QdrantClient(url=os.environ.get("QDRANT_URL", "http://qdrant:6333"))
    _clean(redis, qdrant)
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
        "ENABLE_TWO_TOWER": "true",
        "DATASET_PATH": _dataset(f"{WORK}/dataset"),
        "PROMOTION_FORCE": "true",
        "GATE_MAX_LIST_OVERLAP": "1.0",
        "GATE_MIN_ITEM_COVERAGE": "0",
        "GATE_MIN_USER_COVERAGE": "0",
    }
    nonce = format(int(time.time() * 1000), "x")
    results = []
    try:
        for index, step in enumerate(PLAN["steps"]):
            env = dict(base)
            for k, v in (step.get("env") or {}).items():
                env.pop(k, None) if v is None else env.__setitem__(k, v)
            env.update(_features(step.get("features", "attrs"), f"{WORK}/feat-{index}"))
            env.setdefault("MODEL_VERSION", f"als-fir-{nonce}-{index}")
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
            results.append({"exit_code": proc.returncode, "summary": summary, "log_tail": out[-3000:],
                            "models": _models(redis), "collections": _collections(qdrant),
                            "model_version": env["MODEL_VERSION"]})  # fmt: skip
    finally:
        json.dump({"steps": results}, open(f"{WORK}/result.json", "w"))  # noqa: SIM115
        _clean(redis, qdrant)
    return 0


if __name__ == "__main__":
    sys.exit(main())
