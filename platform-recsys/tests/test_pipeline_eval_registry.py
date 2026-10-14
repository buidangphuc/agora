"""The pipeline evaluates the generation it trained and the registry gate decides (needs PySpark + a JVM).

Ported from the Redis-registry version: the champion is now the ``recsys-als@champion`` alias of the MLflow
registry and the decision is the ``gate.outcome`` tag. "No usable holdout" lives in test_gate_als.py (RAT-06).
"""

from __future__ import annotations

import json

import pytest

pytest.importorskip("pyspark")

from recsys.pipeline import run  # noqa: E402
from tests.dataset_fixture import sample_rows, write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402
from tests.pipeline_fixture import (  # noqa: E402
    ALS,
    newest,
    outcome,
    requires_spark,
    seed_strong,
    settings,
    versions,
)

pytestmark = requires_spark


def test_pipeline_evaluates_and_promotes_initial_model(tmp_path, reg):
    """First run: no champion in the registry -> promoted, becomes the champion, artifacts published."""
    from recsys.evals.holdout import EVAL_PROTOCOL

    write_dataset(tmp_path, sample_rows())
    cfg = settings(tmp_path, "g1", reg, redis_host="localhost", redis_port=6379, gate_max_list_overlap=1.0)
    fake_r, fake_q = FakeRedis(), FakeQdrantClient()

    summary = run(cfg, redis_client=fake_r, qdrant_client=fake_q)

    assert summary["decision"] == "promoted"
    assert "ndcg@10" in summary["metrics"]
    assert summary["qdrant"]["items"] == 3
    assert summary["serving"] == summary["model_version"]
    assert fake_r.store[cfg.serving_key] == summary["model_version"]
    # the registry: one version, champion, promoted, stamped with the protocol and the dataset it trained on
    (v,) = versions(reg, ALS)
    assert reg.aliases(ALS) == {"champion": v} and outcome(reg, ALS, v) == "promoted"
    tags = reg.tags(ALS, v)
    assert tags["lineage.eval_protocol"] == EVAL_PROTOCOL
    manifest = json.loads((tmp_path / "as_of=20261005T020000Z.manifest.json").read_text())
    lineage = " ".join(val for k, val in tags.items() if k.startswith("lineage."))
    assert manifest["file_sha256"] in lineage and manifest["as_of"] in lineage
    assert summary["dataset"] == {
        "name": "als_interactions",
        "version": 1,
        "as_of": "20261005T020000Z",
        "sha256": manifest["file_sha256"],
    }
    # the metrics of the run are the version's metrics
    run_id = reg.client.get_model_version(ALS, str(v)).run_id
    logged = reg.client.get_run(run_id).data.metrics
    assert logged["ndcg_at_10"] == pytest.approx(summary["metrics"]["ndcg@10"])


def test_pipeline_rejects_degraded_candidate_without_loading(tmp_path, reg):
    """A candidate below the champion by the policy is rejected and does not load to the stores."""
    write_dataset(tmp_path, sample_rows())
    seeded = seed_strong(reg, ALS)  # ndcg@10=1.0 under the same protocol
    cfg = settings(tmp_path, "g2", reg, gate_max_list_overlap=1.0)
    fake_r, fake_q = FakeRedis(), FakeQdrantClient()

    summary = run(cfg, redis_client=fake_r, qdrant_client=fake_q)

    assert summary["decision"] == "rejected"
    assert not fake_r.store and not fake_q.collections  # nothing published for the rejected model
    v = newest(reg, ALS)
    assert v != seeded[ALS] and outcome(reg, ALS, v) == "rejected"
    assert "ndcg@10" in reg.tags(ALS, v)["gate.reason"] and "ndcg@10" in summary["reason"]
    assert reg.aliases(ALS) == {"champion": seeded[ALS]}  # still the previous champion


def test_evaluation_model_never_trains_on_the_holdout(tmp_path, monkeypatch):
    """Regression for the leaky protocol: the evaluation model's training triples must
    not contain any held-out (user, listing) pair, and metrics carry the protocol."""
    from pyspark.sql import SparkSession

    import recsys.pipeline as pipeline
    from recsys.config import Settings
    from recsys.evals.holdout import EVAL_PROTOCOL

    parquet_file = write_dataset(tmp_path, sample_rows())
    cfg = Settings(spark_master="local[1]", dataset_dir=str(tmp_path), als_max_iter=2, als_rank=4, top_n=5)
    seen_pairs: list[set] = []
    real_build = pipeline.dataset_triples

    def recording_build(df):
        triples = real_build(df)
        seen_pairs.append({(r["user_key"], r["listing_id"]) for r in triples.collect()})
        return triples

    monkeypatch.setattr(pipeline, "dataset_triples", recording_build)
    spark = SparkSession.builder.master("local[1]").appName("leak-test").getOrCreate()
    try:
        metrics = pipeline.evaluate_generation(spark.read.parquet(str(parquet_file)), cfg)
    finally:
        spark.stop()

    # Targets: each user's most recently discovered listing (see sample_rows).
    heldout = {("u1", "l3"), ("u2", "l3"), ("u3", "l1")}
    (eval_training,) = seen_pairs
    assert not heldout & eval_training, heldout & eval_training
    assert metrics["eval_protocol"] == EVAL_PROTOCOL
    assert metrics["test_events"] == 3
