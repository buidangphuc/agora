"""Runs INSIDE the platform-recsys job image (mounted at /work by rgt_job_flow).

Black-box driver for recsys-gbdt-trainer: it executes the real `python -m recsys` against the stack's Redis and
Qdrant in an isolated namespace (a dedicated Redis DB and Qdrant collections named after it; serving data is
never touched). /work/plan.json holds {"namespace", "redis_db", "steps": [...]}; a step is
  {"env": {..}, "features": "rank"|"no_rank_dataset", "rows": true, "score": [[8 feature values], ..]}
`env` null values unset a variable. Fixture ("rank"): a governed als_interactions dataset, a rank_training@v1 dataset
of 300 impression lists over 24 listings whose clicks depend on item price (cheap items are clicked), and one
item_popularity@v1 and one item_attributes@v1 snapshot older than every impression. `rows` makes the run write
every training row (GBDT_ROWS_PATH) and returns it; `score` asks the image to score those vectors with the
artifact of the serving generation (recsys.ranker.gbdt.score_vector).
"""

from __future__ import annotations

import ast
import hashlib
import json
import os
import random
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

N_ITEMS, N_LISTS, SHOWN = 24, 300, 8
POP_COLS = ["listing_id", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "review_count",
            "avg_rating", "ctr_7d"]  # fmt: skip


def _listing(k: int) -> str:
    return f"gb-{N_ITEMS - 1 - k:02d}"  # ids run against price, so an id tie-break cannot help the baseline


def _als_dataset(directory: str) -> str:
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
            rows.append({"user_key": f"rgt-{user}", "listing_id": f"l{ch}", "weight": 3.0 - h, "interactions": 1,
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


def _rank_dataset(directory: str) -> None:
    rng = random.Random(7)
    start = datetime(2026, 9, 10, 8, 0, 0)
    rows = []
    for n in range(N_LISTS):
        at = start + timedelta(minutes=n * 90)
        for pos, k in enumerate(rng.sample(range(N_ITEMS), SHOWN), start=1):
            p = 0.75 - 0.65 * k / (N_ITEMS - 1)  # cheap listings (small k) are clicked more
            label = 0
            if rng.random() < p:
                label = 2 if rng.random() < 0.25 else 1
            rows.append({"user_key": f"rgt-u{n % 40}", "impression_id": f"rgt-imp-{n:04d}",
                         "listing_id": _listing(k), "position": pos, "label": label,
                         "occurred_at": at + timedelta(seconds=pos)})  # fmt: skip
    df = pd.DataFrame(rows).astype({"position": "int64", "label": "int64"})
    os.makedirs(directory, exist_ok=True)
    path = f"{directory}/as_of=20261005T020000Z.parquet"
    df.to_parquet(path, index=False, coerce_timestamps="ms", allow_truncated_timestamps=True)
    manifest = {"name": "rank_training", "version": 1, "as_of": "20261005T020000Z", "window_days": 30,
                "rows": len(df), "users": int(df["user_key"].nunique()), "items": int(df["listing_id"].nunique()),
                "definition_sha256": "r" * 64, "input_watermark": "2026-10-05T02:00:00Z",
                "file_sha256": hashlib.sha256(open(path, "rb").read()).hexdigest(),  # noqa: SIM115
                "file": os.path.basename(path)}  # fmt: skip
    json.dump(manifest, open(path[: -len(".parquet")] + ".manifest.json", "w"))  # noqa: SIM115


def _features(kind: str, root: str) -> dict[str, str]:
    dirs = {"RANK_DATASET_DIR": f"{root}/rank", "ITEM_FEATURES_DIR": f"{root}/item_popularity/v1",
            "ITEM_ATTRIBUTES_DIR": f"{root}/item_attributes/v1"}  # fmt: skip
    for d in dirs.values():
        os.makedirs(d, exist_ok=True)
    if kind == "rank":
        _rank_dataset(dirs["RANK_DATASET_DIR"])
        pop = [[_listing(k), 100, 5, 1, 1, 3, 4.0, 0.05] for k in range(N_ITEMS)]
        pd.DataFrame(pop, columns=POP_COLS).to_parquet(
            f"{dirs['ITEM_FEATURES_DIR']}/as_of=20260901T000000Z.parquet", index=False
        )
        attrs = [[_listing(k), "rgt-seller", "cat-x", 20_000 * (k + 1)] for k in range(N_ITEMS)]
        pd.DataFrame(attrs, columns=["listing_id", "seller_id", "category_id", "price"]).to_parquet(
            f"{dirs['ITEM_ATTRIBUTES_DIR']}/as_of=20260901T000000Z.parquet", index=False
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


def _models(redis: Redis) -> dict:
    return {
        json.loads(redis.get(k))["model_version"]: json.loads(redis.get(k))
        for k in redis.scan_iter("recs:model:meta:*")
    }


def _rankers(redis: Redis) -> dict:
    out = {}
    for key in sorted(redis.scan_iter("recs:v1:gen:*:ranker")):
        out[key] = {"ttl": redis.ttl(key), "doc": json.loads(redis.get(key))}
    return out


def main() -> int:
    sys.path.insert(0, "/app")
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
        "ENABLE_GBDT": "true",
        "GBDT_TREES": "25",
        # No item has this many impressions, so ctr stays the snapshot's constant and only the price carries signal.
        "GBDT_CTR_MIN_IMPRESSIONS": "1000000",
        "DATASET_PATH": _als_dataset(f"{WORK}/dataset"),
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
            env.update(_features(step.get("features", "rank"), f"{WORK}/feat-{index}"))
            rows_path = f"{WORK}/rows-{index}.parquet"
            if step.get("rows"):
                env["GBDT_ROWS_PATH"] = rows_path
            env.setdefault("MODEL_VERSION", f"als-rgt-{nonce}-{index}")
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
            result = {"exit_code": proc.returncode, "summary": summary, "log_tail": out[-3000:],
                      "models": _models(redis), "rankers": _rankers(redis),
                      "gbdt_champion": redis.get("recs:model:champion:gbdt"),
                      "serving": redis.get("recs:v1:serving"), "previous": redis.get("recs:v1:previous"),
                      "model_version": env["MODEL_VERSION"]}  # fmt: skip
            if step.get("rows") and os.path.exists(rows_path):
                result["rows"] = (
                    pd.read_parquet(rows_path).astype({"occurred_at": "string"}).to_dict("records")
                )
            if step.get("score") and result["serving"]:
                from recsys.ranker.gbdt import score_vector  # noqa: PLC0415

                art = (
                    result["rankers"].get(f"recs:v1:gen:{result['serving']}:ranker", {}).get("doc")
                )
                if art:
                    result["reference_scores"] = [score_vector(art, v) for v in step["score"]]
            results.append(result)
    finally:
        json.dump({"steps": results}, open(f"{WORK}/result.json", "w"), default=str)
        _clean(redis, qdrant)
    return 0


if __name__ == "__main__":
    sys.exit(main())
