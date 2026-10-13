"""Scenarios: CLI `python -m modelregistry` exit codes and output."""
from __future__ import annotations

import json

import mlflow
import pytest

from conftest import run_cli


def _setup_three(env, model="m", cand_metric=0.100):
    env.add_version(model, {"ndcg@10": 0.100}, alias="champion")   # v1
    env.add_version(model, {"ndcg@10": 0.0}, outcome="rejected")   # v2
    return env.add_version(model, {"ndcg@10": cand_metric}, outcome="rejected")  # v3


def test_force_promote_records_override(env):
    # Scenario: Force moves the alias and records the override
    v3 = _setup_three(env)
    r = run_cli("promote", "m", str(v3), "--force", "--reason", "hotfix for stale data")
    assert r.returncode == 0, r.stderr
    assert env.aliases("m") == {"champion": 3, "previous": 1}
    tags = env.tags("m", 3)
    assert tags["gate.forced"] == "true"
    assert tags["gate.outcome"] == "promoted"
    assert tags["gate.reason"] == "hotfix for stale data"


@pytest.mark.parametrize("extra", [[], ["--reason", ""]])
def test_force_without_reason_refused(env, extra):
    # Scenario: Force without a reason is refused
    _setup_three(env)
    before = env.snapshot("m")
    r = run_cli("promote", "m", "3", "--force", *extra)
    assert r.returncode == 2
    assert "--reason" in r.stderr
    assert env.snapshot("m") == before


def test_unknown_version_exit_2(env):
    # Scenario: Promoting an unknown version fails
    _setup_three(env)
    before = env.snapshot("m")
    r = run_cli("promote", "m", "99", "--force", "--reason", "x")
    assert r.returncode == 2
    assert "99" in r.stderr
    assert env.snapshot("m") == before


def test_unforced_promote_rejected_exits_0_with_json(env):
    # Scenario: Unforced promote that fails the gate exits 0 with the decision
    _setup_three(env, cand_metric=0.100)  # equal to champion, needs +1 %
    r = run_cli("promote", "m", "3")
    assert r.returncode == 0, r.stderr
    out = json.loads(r.stdout)
    assert out["outcome"] == "rejected"
    assert int(out["version"]) == 3
    assert out["reason"]
    assert env.aliases("m") == {"champion": 1}


def test_unforced_promote_passes_applies(env):
    # Scenario: Unforced promote that passes applies it
    _setup_three(env, cand_metric=0.200)
    r = run_cli("promote", "m", "3")
    assert r.returncode == 0, r.stderr
    assert json.loads(r.stdout)["outcome"] == "promoted"
    assert env.aliases("m")["champion"] == 3


def test_rollback_swaps_champion_and_previous(env):
    # Scenario: Rollback swaps the aliases
    env.add_version("m", {"ndcg@10": 0.1}, alias="previous")
    env.add_version("m", {"ndcg@10": 0.2}, alias="champion")
    r = run_cli("rollback", "m")
    assert r.returncode == 0, r.stderr
    assert env.aliases("m") == {"champion": 1, "previous": 2}


def test_rollback_without_previous_exit_2(env):
    # Scenario: Rollback without a previous is refused
    env.add_version("m", {"ndcg@10": 0.1}, alias="champion")
    r = run_cli("rollback", "m")
    assert r.returncode == 2
    assert env.aliases("m") == {"champion": 1}


def test_policy_error_exit_2(env):
    # Scenario: PolicyError exits 2
    _setup_three(env, model="nopolicy")
    r = run_cli("promote", "nopolicy", "3")
    assert r.returncode == 2
    assert "PolicyError" in r.stderr and "nopolicy" in r.stderr


@pytest.mark.parametrize("argv", [["rollback"], ["frobnicate", "m"], []])
def test_usage_error_exit_2(env, argv):
    # Scenario: Unknown subcommand or missing argument exits 2
    r = run_cli(*argv)
    assert r.returncode == 2
    assert "usage" in (r.stderr + r.stdout).lower()


def test_unexpected_exception_exit_1(env, monkeypatch, capsys):
    # Scenario: Unexpected exception exits 1 (client layer raises RuntimeError, in-process)
    def boom(*a, **k):
        raise RuntimeError("boom-unexpected")

    for name in ("get_model_version_by_alias", "get_registered_model", "search_model_versions",
                 "set_registered_model_alias", "get_model_version", "get_run"):
        monkeypatch.setattr(mlflow.MlflowClient, name, boom)
    from modelregistry.cli import main

    try:
        rc = main(["rollback", "m"])
    except SystemExit as e:  # main may either return or exit
        rc = e.code
    assert rc == 1
    assert "boom-unexpected" in capsys.readouterr().err
