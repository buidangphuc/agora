"""Scenarios: seed bypasses the gate for an empty model only."""
from __future__ import annotations

import pytest

from modelregistry import GateError, evaluate, seed

LINEAGE = {"data_version": "d1", "eval_protocol": "p1"}


def test_seed_creates_first_champion(env):
    # Scenario: Seed creates the first champion
    seed("m", {"ndcg@10": 0.1}, LINEAGE, "migrate from redis")
    assert env.versions("m") == [1]
    assert env.aliases("m") == {"champion": 1}
    tags = env.tags("m", 1)
    assert tags["gate.outcome"] == "promoted"
    assert tags["gate.reason"] == "seed: migrate from redis"
    assert tags["lineage.eval_protocol"] == "p1"
    assert env.run_metrics(env.run_id("m", 1))["ndcg@10"] == pytest.approx(0.1)


def test_seed_does_not_read_policy(env):
    # Scenario: Seed creates the first champion (without reading the policy)
    seed("model-without-policy", {"x": 1.0}, LINEAGE, "r")
    assert env.aliases("model-without-policy") == {"champion": 1}


def test_seed_metrics_only(env):
    # Scenario: Seed without an artifact is allowed
    seed("m", {"ndcg@10": 0.1}, LINEAGE, "r", artifact_path=None)
    assert env.model_files(env.run_id("m", 1)) == []


def test_seed_with_artifact_logs_under_model(env):
    # Scenario: Seed creates the first champion (artifact variant)
    seed("m", {"ndcg@10": 0.1}, LINEAGE, "r", artifact_path=str(env.artifact_dir()))
    assert any(p.endswith("weights.bin") for p in env.model_files(env.run_id("m", 1)))


def test_seed_refused_when_versions_exist(env):
    # Scenario: Seed refused when versions exist (including a rejected one)
    env.add_version("m", {"ndcg@10": 0.1}, outcome="rejected")
    before = env.snapshot("m")
    runs_before = len(env.client.search_runs(["0"]))
    with pytest.raises(GateError):
        seed("m", {"ndcg@10": 0.9}, LINEAGE, "r")
    assert env.snapshot("m") == before
    assert env.versions("m") == [1]
    assert len(env.client.search_runs(["0"])) == runs_before


def test_evaluate_after_seed_compares(env):
    # Scenario: The next evaluate compares against the seeded champion
    seed("m", {"ndcg@10": 0.1}, LINEAGE, "r")
    c = env.candidate({"ndcg@10": 0.2}, protocol="p1")
    d = evaluate("m", c["run_id"], c["metrics"], c["artifact_path"], c["lineage"])
    assert d.outcome == "promotable"
    assert not d.reason.startswith("eval_protocol changed")
    assert "improvement" in d.reason.lower()
