"""Helpers for pipeline-level tests (real Spark in local mode; skipped on a host without a JVM).

State is prepared and judged through the MLflow file store of the ``reg`` fixture and the in-memory
Redis/Qdrant fakes of ``tests/fakes.py``; nothing here asserts that a mock was called.

INTERFACE ASSUMPTIONS made by the group-B tests (see the report of the test author):
  - ``recsys.pipeline.run(settings=None, redis_client=None, qdrant_client=None) -> dict`` (no ``registry``
    argument). Summary keys kept from the old pipeline: ``decision`` (promoted|rejected|skipped), ``gate``
    ("structural" for a degenerate candidate), ``reason``, ``model_version``, ``serving``, ``previous``,
    ``qdrant``, ``metrics``, ``drift``, ``two_tower_items`` and the sub-summaries ``gbdt`` / ``two_tower``
    (each with ``decision``: promoted|rejected|skipped; ``gbdt.ranker_key``).
  - the ALS trainer is looked up as ``recsys.pipeline.train_als``; ``evaluate_generation`` stays in
    ``recsys.pipeline``; the stages are looked up as ``recsys.ranker.stage.run_stage`` and
    ``recsys.two_tower.stage.run_stage``.
  - the candidate version carries ``lineage.eval_protocol`` (ALS: ``recsys.evals.holdout.EVAL_PROTOCOL``,
    GBDT: ``recsys.ranker.contract.EVAL_PROTOCOL``, two-tower: ``recsys.two_tower.stage.EVAL_PROTOCOL``) and
    ``lineage.model_version`` (the generation id) so a rollback can find the version that built a generation.
"""

from __future__ import annotations

import shutil
import subprocess
from datetime import datetime, timedelta, timezone
from pathlib import Path

import pandas as pd
import pytest

from recsys.config import Settings

ALS = "recsys-als"
GBDT = "recsys-gbdt"
TWO_TOWER = "recsys-two-tower"

WEAK = {
    ALS: {"ndcg@10": 0.0001, "coverage@10": 0.01},
    GBDT: {"ndcg@10": 0.0001, "recall@10": 0.0001},
    TWO_TOWER: {"recall@10": 0.0001, "coverage@10": 0.01},
}
STRONG = {
    ALS: {"ndcg@10": 1.0, "coverage@10": 1.0},
    GBDT: {"ndcg@10": 0.9999, "recall@10": 0.9999},
    TWO_TOWER: {"recall@10": 1.0, "coverage@10": 1.0},
}


def _java_ok() -> bool:
    java = shutil.which("java")
    if not java:
        return False
    try:
        return subprocess.run([java, "-version"], capture_output=True, timeout=20).returncode == 0
    except (OSError, subprocess.SubprocessError):
        return False


requires_spark = pytest.mark.skipif(not _java_ok(), reason="Spark needs a Java runtime (runs in the image)")


def protocol_of(model: str) -> str:
    """The evaluation protocol the candidate of ``model`` is stamped with."""
    if model == ALS:
        from recsys.evals.holdout import EVAL_PROTOCOL

        return EVAL_PROTOCOL
    if model == GBDT:
        from recsys.ranker.contract import EVAL_PROTOCOL as gbdt_protocol

        return gbdt_protocol
    from recsys.two_tower import stage

    assert hasattr(stage, "EVAL_PROTOCOL"), "interface: recsys.two_tower.stage.EVAL_PROTOCOL"
    return stage.EVAL_PROTOCOL


def seed_weak(reg, *models: str) -> dict[str, int]:
    """A champion any candidate beats (no protocol: the candidate is judged on the floors only)."""
    return {m: reg.add_version(m, WEAK[m], protocol=None) for m in models}


def seed_strong(reg, *models: str) -> dict[str, int]:
    """A champion no candidate can reach, measured under the candidate's own protocol."""
    return {m: reg.add_version(m, STRONG[m], protocol=protocol_of(m)) for m in models}


def versions(reg, model: str) -> list[int]:
    return sorted(int(v.version) for v in reg.client.search_model_versions(f"name='{model}'"))


def registered_models(reg) -> set[str]:
    return {m.name for m in reg.client.search_registered_models()}


def outcome(reg, model: str, version: int) -> str:
    return reg.tags(model, version)["gate.outcome"]


def newest(reg, model: str) -> int:
    return versions(reg, model)[-1]


def als_rows(weight_scale: float = 1.0) -> list[dict]:
    """Two taste clusters over six listings; ``weight_scale`` rescales every pair's weight."""
    now = datetime.now(timezone.utc)
    rows = []
    for u, items in [
        ("u1", "abc"),
        ("u2", "abc"),
        ("u3", "abc"),
        ("u4", "def"),
        ("u5", "def"),
        ("u6", "def"),
    ]:
        for h, ch in enumerate(items):
            weight = (3.0 - h) * weight_scale + 0.01 * int(u[1:])
            rows.append(
                {"user_key": u, "listing_id": f"l{ch}", "weight": weight, "interactions": 1,
                 "last_occurred_at": now - timedelta(hours=h + 1)}
            )  # fmt: skip
    return rows


def one_size_rows() -> list[dict]:
    """Every user interacted with the same three items only."""
    now = datetime.now(timezone.utc)
    return [
        {"user_key": f"u{u}", "listing_id": lid, "weight": 1.0, "interactions": 1,
         "last_occurred_at": now - timedelta(hours=h + 1)}
        for u in range(1, 5)
        for h, lid in enumerate(["l1", "l2", "l3"])
    ]  # fmt: skip


def no_holdout_rows() -> list[dict]:
    """One event per user: the temporal split leaves no test event."""
    now = datetime.now(timezone.utc)
    return [
        {"user_key": u, "listing_id": lid, "weight": 1.0, "interactions": 1, "last_occurred_at": now}
        for u, lid in [("u1", "l1"), ("u2", "l2"), ("u3", "l3")]
    ]  # fmt: skip


def settings(data_dir, version: str, reg, **kw) -> Settings:
    base = dict(
        spark_master="local[1]",
        dataset_dir=str(data_dir),
        als_max_iter=3,
        als_rank=4,
        top_n=2,
        model_version=version,
        mlflow_tracking_uri=reg.tracking_uri,
    )  # fmt: skip
    return Settings(**{**base, **kw})


USER_COLS = ["user_key", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "follows_current",
             "paid_orders_30d"]  # fmt: skip
ITEM_COLS = ["listing_id", "views_7d", "clicks_7d", "add_to_cart_7d", "favorites_current", "review_count",
             "avg_rating", "ctr_7d"]  # fmt: skip


def tower_features(root: Path, items: list[list], users: list[list] | None = None) -> dict[str, str]:
    """item_popularity / user_activity snapshots for the two-tower stage; returns Settings kwargs."""
    d = root / "tt-feat"
    (d / "i").mkdir(parents=True, exist_ok=True)
    (d / "u").mkdir(parents=True, exist_ok=True)
    pd.DataFrame(items, columns=ITEM_COLS).to_parquet(d / "i" / "as_of=20261005T000000Z.parquet", index=False)
    users = users if users is not None else [[f"u{n}", 5 * n, n, 0, 0, 0, n % 2] for n in range(1, 13)]
    pd.DataFrame(users, columns=USER_COLS).to_parquet(d / "u" / "as_of=20261005T000000Z.parquet", index=False)
    return {"item_features_dir": str(d / "i"), "user_features_dir": str(d / "u")}


def cold_item_features(root: Path, extra: tuple = ()) -> dict[str, str]:
    """Items la..lf plus a never-interacted ``l-cold`` (the old two-tower pipeline fixture)."""
    items = [[f"l{c}", 10 * (i + 1), i, i // 2, i, 0, None, 0.1 * i] for i, c in enumerate("abcdef")]
    items.append(["l-cold", 0, 0, 0, 2, 1, 4.0, 0.0])
    items.extend(extra)
    return tower_features(root, items)


def three_model_world(root: Path, monkeypatch) -> dict[str, str]:
    """One governed world in which ALS, GBDT and two-tower can all pass.

    The catalogue is 8 listings (``gb-00..gb-07``), so a top-10 retrieval returns the whole catalogue:
    the two-tower coverage floor holds whatever the towers learn. The ALS dataset and the ranking dataset
    use the same listings; ranking impressions come from tests/ranker_fixture.
    """
    from tests import ranker_fixture as fx
    from tests.dataset_fixture import write_dataset

    monkeypatch.setattr(fx, "ITEMS", 8)
    kw = fx.write_all(root)
    now = datetime.now(timezone.utc)
    rows = []
    for u in range(12):
        cluster = range(0, 4) if u < 6 else range(4, 8)
        for h, k in enumerate(list(cluster)[:3] if u % 2 == 0 else list(cluster)[1:]):
            rows.append({"user_key": f"u{u + 1}", "listing_id": fx.listing_id(k), "weight": 3.0 - h,
                         "interactions": 1, "last_occurred_at": now - timedelta(hours=h + 1)})  # fmt: skip
    write_dataset(root / "als", rows)
    items = [[fx.listing_id(k), 100, 5, 1, 1, 3, 4.0, 0.05] for k in range(8)]
    kw.update(tower_features(root, items))
    kw["item_features_dir"] = str(root / "pop")  # one popularity snapshot serves both stages
    kw["dataset_dir"] = str(root / "als")
    return kw


def run_main(monkeypatch, settings: Settings, redis_client, qdrant_client) -> int:
    """``python -m recsys`` with the stores injected: returns the process exit code."""
    from recsys import __main__ as entry
    from recsys.load import qdrant as qdrant_mod
    from recsys.load import redis_cache

    monkeypatch.setattr(redis_cache, "connect", lambda s: redis_client)
    monkeypatch.setattr(qdrant_mod, "_connect", lambda s: qdrant_client)
    monkeypatch.setattr(entry, "load_settings", lambda: settings)
    return entry.main([])


class SparkStarted(BaseException):
    """Raised by a Spark entry point that must not be reached (not an Exception: no handler maps it)."""


def forbid_spark(monkeypatch) -> None:
    def boom(*_a, **_k):
        raise SparkStarted("Spark was started")

    monkeypatch.setattr("recsys.spark.build_spark", boom)
    monkeypatch.setattr("recsys.pipeline.build_spark", boom, raising=False)


def gens(redis) -> set[str]:
    return {k.split(":")[3] for k in redis.store if k.startswith("recs:v1:gen:")}


def pointers(redis) -> dict[str, str]:
    return {k: v for k, v in redis.store.items() if k.startswith("recs:v1:") and ":gen:" not in k}


def aliases_of(reg, *models: str) -> dict[str, dict[str, int]]:
    return {m: reg.aliases(m) for m in models}
