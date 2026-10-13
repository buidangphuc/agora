"""recsys-als policy (ADR-0014 numbers), packaged policies.yaml: RAT-01..06, RAT-35, RAT-37..39."""

from __future__ import annotations

import ast
from pathlib import Path

import pytest
from modelregistry import apply

from recsys.pipeline import run
from tests.dataset_fixture import write_dataset
from tests.fakes import FakeQdrantClient, FakeRedis
from tests.pipeline_fixture import (
    als_rows,
    newest,
    no_holdout_rows,
    one_size_rows,
    outcome,
    requires_spark,
    run_main,
    seed_strong,
    seed_weak,
    settings,
    versions,
)

M = "recsys-als"


def _champ(reg):
    return reg.add_version(M, {"ndcg@10": 0.100, "coverage@10": 0.50})


def test_als_promotable_then_applied_after_publish(reg):
    # RAT-01
    old = _champ(reg)
    d = reg.evaluate(M, {"ndcg@10": 0.102, "coverage@10": 0.45})
    assert d.outcome == "promotable"
    assert reg.aliases(M) == {"champion": old}  # evaluate moves no alias
    apply(M, d.version)
    assert reg.tags(M, d.version)["gate.outcome"] == "promoted"
    assert reg.aliases(M) == {"champion": d.version, "previous": old}


def test_als_below_threshold_rejected(reg):
    # RAT-02
    old = _champ(reg)
    d = reg.evaluate(M, {"ndcg@10": 0.1005, "coverage@10": 0.50})
    assert d.outcome == "rejected"
    tags = reg.tags(M, d.version)
    assert tags["gate.outcome"] == "rejected" and "ndcg@10" in tags["gate.reason"]
    assert reg.aliases(M) == {"champion": old}


def test_als_coverage_guard(reg):
    # RAT-03
    _champ(reg)
    d = reg.evaluate(M, {"ndcg@10": 0.150, "coverage@10": 0.39})
    assert d.outcome == "rejected"
    assert "coverage@10" in reg.tags(M, d.version)["gate.reason"]


def test_als_boundary_values_pass(reg):
    # RAT-04
    _champ(reg)
    d = reg.evaluate(M, {"ndcg@10": 0.101, "coverage@10": 0.40})
    assert d.outcome == "promotable"
    assert reg.tags(M, d.version)["gate.outcome"] == "promotable"


def test_als_first_candidate_without_champion_is_promotable(reg):
    # RAT-01 (no champion: judged on floors only; recsys-als has none)
    d = reg.evaluate(M, {"ndcg@10": 0.01, "coverage@10": 0.01})
    assert d.outcome == "promotable"


@pytest.mark.parametrize("bad", [{"ndcg@10": 0.2}, {"coverage@10": 0.5}])
def test_als_missing_metric_rejected(reg, bad):
    # RAT-02 (an incomplete candidate is rejected, not accepted)
    _champ(reg)
    assert reg.evaluate(M, bad).outcome == "rejected"


# --- RAT-37 / RAT-38 / RAT-39: the gate's edge decisions (library behaviour the job relies on) -----
def test_first_candidate_without_champion(reg):
    # RAT-37
    d = reg.evaluate(M, {"ndcg@10": 0.05, "coverage@10": 0.2})
    assert d.outcome == "promotable"
    assert "no champion" in reg.tags(M, d.version)["gate.reason"].lower()


def test_protocol_change_floors_only(reg):
    # RAT-39: not compared, judged on the floors only (recsys-als has none), reason names both protocols
    reg.add_version(M, {"ndcg@10": 0.75, "coverage@10": 0.8}, protocol="leave-last-new-item-v0")
    d = reg.evaluate(M, {"ndcg@10": 0.05, "coverage@10": 0.3}, protocol="leave-last-new-item-v1")
    assert d.outcome == "promotable"
    assert reg.tags(M, d.version)["gate.reason"].startswith(
        "eval_protocol changed: leave-last-new-item-v0 -> leave-last-new-item-v1"
    )


def test_nan_metric_rejected_by_the_library(reg):
    # RAT-38 (library half): no exception, a rejected version whose reason names the metric
    _champ(reg)
    d = reg.evaluate(M, {"ndcg@10": 0.2, "coverage@10": float("nan")})
    assert d.outcome == "rejected"
    assert "coverage@10" in reg.tags(M, d.version)["gate.reason"]


# --- the nightly job on top of the gate (real Spark: skipped without a JVM) ------------------------
@requires_spark
def test_structural_reject_uses_register_rejected(tmp_path, reg, monkeypatch):
    # RAT-05: degenerate candidate -> registered as rejected, evaluate never called, nothing published
    import modelregistry

    write_dataset(tmp_path, one_size_rows())
    seeded = seed_weak(reg, M)
    monkeypatch.setattr(
        modelregistry, "evaluate", lambda *a, **k: pytest.fail("the metric gate must not be called")
    )
    r, q = FakeRedis(), FakeQdrantClient()

    summary = run(settings(tmp_path, "g2-degenerate", reg), redis_client=r, qdrant_client=q)

    assert summary["decision"] == "rejected" and summary["gate"] == "structural"
    new = [v for v in versions(reg, M) if v != seeded[M]]
    assert len(new) == 1
    tags = reg.tags(M, new[0])
    assert tags["gate.outcome"] == "rejected" and tags["gate.reason"] == summary["reason"]
    assert "list overlap" in tags["gate.reason"] or "item coverage" in tags["gate.reason"]
    assert reg.aliases(M) == {"champion": seeded[M]}
    assert not r.store and not q.collections


@requires_spark
def test_no_holdout_not_a_candidate(tmp_path, reg, monkeypatch):
    # RAT-06: no usable holdout -> nothing registered, no alias moves, nothing published, exits 0
    write_dataset(tmp_path, no_holdout_rows())
    seeded = seed_weak(reg, M)
    r, q = FakeRedis(), FakeQdrantClient()
    cfg = settings(tmp_path, "g2", reg, gate_max_list_overlap=1.0)

    assert run_main(monkeypatch, cfg, r, q) == 0
    summary = run(cfg, redis_client=r, qdrant_client=q)

    assert summary["decision"] == "skipped" and "no usable holdout" in summary["reason"]
    assert versions(reg, M) == [seeded[M]]
    assert reg.aliases(M) == {"champion": seeded[M]}
    assert not r.store and not q.collections


@requires_spark
def test_nan_guard_metric_rejected(tmp_path, reg, monkeypatch):
    # RAT-38: a NaN coverage@10 on the candidate -> rejected, reason names it, no exception, exit 0
    import recsys.pipeline as pipeline

    write_dataset(tmp_path, als_rows())
    seeded = seed_weak(reg, M)
    real = pipeline.evaluate_generation
    monkeypatch.setattr(
        pipeline, "evaluate_generation", lambda ev, s: {**real(ev, s), "coverage@10": float("nan")}
    )
    r, q = FakeRedis(), FakeQdrantClient()

    assert run_main(monkeypatch, settings(tmp_path, "g2", reg), r, q) == 0

    v = newest(reg, M)
    assert v != seeded[M] and outcome(reg, M, v) == "rejected"
    assert "coverage@10" in reg.tags(M, v)["gate.reason"]
    assert reg.aliases(M) == {"champion": seeded[M]}
    assert not r.store and not q.collections


@requires_spark
def test_nightly_job_cannot_force(tmp_path, reg, monkeypatch):
    # RAT-35: the removed PROMOTION_FORCE is denied: still rejected, no gate.forced tag, champion unchanged
    write_dataset(tmp_path, als_rows())
    seeded = seed_strong(reg, M)
    monkeypatch.setenv("PROMOTION_FORCE", "true")
    r, q = FakeRedis(), FakeQdrantClient()

    summary = run(settings(tmp_path, "g2", reg), redis_client=r, qdrant_client=q)

    v = newest(reg, M)
    assert summary["decision"] == "rejected" and v != seeded[M]
    tags = reg.tags(M, v)
    assert tags["gate.outcome"] == "rejected" and not any("forced" in k for k in tags)
    assert reg.aliases(M) == {"champion": seeded[M]}
    assert not r.store and not q.collections


def test_no_code_path_forces_a_promotion():
    # RAT-35 / task 1.11: nothing under recsys/ calls promote(...), passes force=..., or reads PROMOTION_FORCE
    root = Path(__file__).resolve().parents[1] / "recsys"
    offenders = []
    for path in root.rglob("*.py"):
        text = path.read_text(encoding="utf-8")
        if "PROMOTION_FORCE" in text:
            offenders.append(f"{path.name}: PROMOTION_FORCE")
        for node in ast.walk(ast.parse(text)):
            if isinstance(node, ast.Call):
                fn = node.func
                name = fn.attr if isinstance(fn, ast.Attribute) else getattr(fn, "id", "")
                if name == "promote":
                    offenders.append(f"{path.name}:{node.lineno} promote()")
                if any(kw.arg == "force" for kw in node.keywords):
                    offenders.append(f"{path.name}:{node.lineno} force=")
    assert not offenders, offenders
