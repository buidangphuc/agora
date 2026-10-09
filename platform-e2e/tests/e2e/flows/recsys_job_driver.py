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

Live plans (recsys-generation-publish) skip the namespace: they publish to the stack's own serving
data (DB 0, the default collections and their aliases), optionally forgetting every generation first
("reset"), and leave what they published in place. A run may name a dataset variant ("@fixture") and
a subcommand ("@command", e.g. rollback).

Only the image's own dependencies are used (pandas, redis, qdrant-client).
"""

from __future__ import annotations

import ast
import hashlib
import json
import os
import subprocess
import sys
import time
from datetime import datetime, timezone

import pandas as pd
from qdrant_client import QdrantClient
from qdrant_client.models import DeleteAlias, DeleteAliasOperation
from redis import Redis

WORK = "/work"
PLAN = json.load(open(f"{WORK}/plan.json"))  # noqa: SIM115
NS = PLAN["namespace"]
# "live" plans (recsys-generation-publish, destructive lane) publish to the stack's serving data
# (Redis DB 0, the default collections / aliases) so team-ai, and the gateway in front of it, read
# what the job published. Default plans stay in the per-worker namespace.
LIVE = bool(PLAN.get("live"))
ITEMS = "item_als_vectors" if LIVE else f"{NS}_items"
USERS = "user_als_vectors" if LIVE else f"{NS}_users"
DB = 0 if LIVE else int(PLAN["redis_db"])
BUYER = PLAN.get("buyer_id") or ""
PREFIX = "recs:v1"
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


def _write_dataset(rows: list[dict], root: str = WORK) -> str | None:
    """Write a governed dataset fixture (parquet + manifest) from events under `root`."""
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
    directory = f"{root}/datasets/als_interactions/v1"
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


# ── Fixture variants (recsys-generation-publish) ──────────────────────────
# A run picks one with the reserved run key "@fixture". Three taste clusters of five items; users
# of a cluster interact with items of their own cluster, the last (held-out) one being an item their
# cluster-mates also have, so the temporal holdout scores above zero. The buyer (plan "buyer_id")
# replaces the first user so Recommend through the gateway has a cached list for the buyer.
#   good_a       4 of the 5 cluster items per user (4 users per cluster)
#   better_b     all 5 cluster items per user (denser history, same users)
#   third_c      4 items, rotated, 5 users per cluster (a different dataset again)
#   regressing   good_a's data; the run is made to fail the metric gate (+10% needed, see below)
#   one_size     every user interacted with the same three items only (degenerate)
# "Better" is engineered as in pipeline_eval_registry: the promotion gate is the lever, not luck. A
# promoting variant runs with PROMOTION_MIN_RELATIVE_IMPROVEMENT=-0.25 (an equal-scoring retrain
# passes), the regressing one with +0.10 (an equal-scoring retrain cannot pass).
_FIXTURE_ENV = {
    "better_b": {"PROMOTION_MIN_RELATIVE_IMPROVEMENT": "-0.25"},
    "third_c": {"PROMOTION_MIN_RELATIVE_IMPROVEMENT": "-0.25"},
    "regressing": {"PROMOTION_MIN_RELATIVE_IMPROVEMENT": "0.10"},
}
_EVENT_CYCLE = ("view", "click", "add_to_cart", "click")


def _fixture_events(name: str) -> list[dict]:
    now = time.time()
    rows: list[dict] = []

    def add(user: str, item: str, step: int, total: int, kind: str) -> None:
        # Oldest first; the final step is the most recent (the temporal holdout).
        rows.append(
            {"user": user, "listing": item, "event_type": kind, "ts": now - (total - step) * 3600}
        )

    if name == "one_size":
        users = [f"rgp-u-{i}" for i in range(10)]
        if BUYER:
            users[0] = BUYER
        for user in users:
            for step in range(3):
                add(user, f"rgp-item-0-{step}", step, 3, _EVENT_CYCLE[step])
        return rows
    per_cluster = 5 if name == "third_c" else 4
    shift = 2 if name == "third_c" else 0
    seen = 5 if name == "better_b" else 4
    for cluster in range(3):
        for i in range(per_cluster):
            user = f"rgp-u{cluster}-{i}"
            if cluster == 0 and i == 0 and BUYER:
                user = BUYER
            for step in range(seen):
                item = f"rgp-item-{cluster}-{(i + shift + step) % 5}"
                add(user, item, step, seen, _EVENT_CYCLE[step % len(_EVENT_CYCLE)])
    return rows


def _fixture_for(name: str) -> str | None:
    base = "good_a" if name == "regressing" else name
    return _write_dataset(_fixture_events(base), f"{WORK}/fixture-{name}")


def _reset_live(redis: Redis, qdrant: QdrantClient) -> None:
    """Forget every generation: pointers, gen keys, registry, aliases and generation collections.

    The unscoped v1 keys and any real (non-generation) collection are left alone, so team-ai's
    fallback and the first publish's legacy migration keep working.
    """
    for pattern in (f"{PREFIX}:gen:*", "recs:model:*"):
        for key in list(redis.scan_iter(pattern)):
            redis.delete(key)
    redis.delete(f"{PREFIX}:serving", f"{PREFIX}:previous")
    aliases = {a.alias_name: a.collection_name for a in qdrant.get_aliases().aliases}
    for alias in (ITEMS, USERS):
        if alias in aliases:
            qdrant.update_collection_aliases(
                change_aliases_operations=[
                    DeleteAliasOperation(delete_alias=DeleteAlias(alias_name=alias))
                ]
            )
    for name in [c.name for c in qdrant.get_collections().collections]:
        if name.startswith((f"{ITEMS}__", f"{USERS}__")):
            qdrant.delete_collection(name)


def _clean(redis: Redis, qdrant: QdrantClient) -> None:
    redis.flushdb()
    for name in (ITEMS, USERS):
        if qdrant.collection_exists(name):
            qdrant.delete_collection(name)


def _aliases(qdrant: QdrantClient) -> dict[str, str]:
    return {a.alias_name: a.collection_name for a in qdrant.get_aliases().aliases}


def _points(qdrant: QdrantClient, name: str) -> dict:
    name = _aliases(qdrant).get(name, name)  # an alias reads as the collection it points at
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
    gens: dict[str, int] = {}
    for key in redis.scan_iter(f"{PREFIX}:gen:*"):
        gen = key[len(f"{PREFIX}:gen:") :].split(":", 1)[0]
        gens[gen] = gens.get(gen, 0) + 1
    aliases = _aliases(qdrant)
    return {
        "champion": redis.get("recs:model:champion"),
        "serving_model_version": redis.get("recs:v1:model_version"),
        # Generation publishing: the pointers, each generation's keys and collections, the aliases.
        "serving": redis.get(f"{PREFIX}:serving"),
        "previous": redis.get(f"{PREFIX}:previous"),
        "gen_keys": gens,
        "collections": sorted(
            c.name
            for c in qdrant.get_collections().collections
            if c.name.startswith((f"{ITEMS}__", f"{USERS}__"))
        ),
        "aliases": {a: c for a, c in aliases.items() if a in (ITEMS, USERS)},
        "popular_cached": redis.exists("recs:v1:popular") == 1,
        "models": models,
        "items": _points(qdrant, ITEMS),
        "users": _points(qdrant, USERS),
    }


def main() -> int:
    fixture = _write_dataset(PLAN["events"])
    os.makedirs(f"{WORK}/empty-dataset", exist_ok=True)
    redis_host = os.environ.get("REDIS_HOST", "redis")
    redis = Redis(host=redis_host, port=6379, db=DB, decode_responses=True)
    qdrant = QdrantClient(url=os.environ.get("QDRANT_URL", "http://qdrant:6333"))
    if LIVE:
        if PLAN.get("reset"):
            _reset_live(redis, qdrant)
    else:
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
        nonce = format(int(time.time() * 1000), "x")
        for index, raw_env in enumerate(PLAN["runs"]):
            # Reserved run keys: "@fixture" picks a dataset variant, "@command" a subcommand.
            run_env = dict(raw_env)
            variant = run_env.pop("@fixture", None)
            command = run_env.pop("@command", None)
            if variant:
                run_env = {**_FIXTURE_ENV.get(variant, {}), **run_env}
                # An explicit version per run: the default one is the run clock to the second.
                slug = variant.replace("_", "-")
                run_env.setdefault("MODEL_VERSION", f"als-e2e-{slug}-{nonce}-{index}")
            env = {**base_env, **{k: v for k, v in run_env.items() if v is not None}}
            for key, value in run_env.items():
                if value is None:
                    env.pop(key, None)
            if variant:
                path = _fixture_for(variant)
                env["DATASET_PATH"] = path or ""
            proc = subprocess.run(
                [sys.executable, "-m", "recsys", *([command] if command else [])],
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
        if not LIVE:
            _clean(redis, qdrant)
    json.dump({"runs": results}, open(f"{WORK}/result.json", "w"))  # noqa: SIM115
    return 0


if __name__ == "__main__":
    sys.exit(main())
