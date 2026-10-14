"""Scenarios: the gate fails closed when the registry is unavailable."""
from __future__ import annotations

import time

import mlflow as _mlflow
import mlflow.environment_variables as mev
import pytest
from mlflow.exceptions import MlflowException

from conftest import run_cli
from modelregistry import (
    GateError, RegistryUnavailable, apply, check_reachable, evaluate, promote, rollback,
    seed, submit,
)

DEADLINE_WITH_SLACK = 13.5  # 12 s deadline plus process scheduling slack


def _ops(env):
    c = env.candidate({"ndcg@10": 0.2})
    args = (c["run_id"], c["metrics"], c["artifact_path"], c["lineage"])
    return {
        "evaluate": lambda: evaluate("m", *args),
        "apply": lambda: apply("m", 1),
        "submit": lambda: submit("m", *args),
        "promote": lambda: promote("m", 1, force=True, reason="x"),
        "rollback": lambda: rollback("m"),
        "seed": lambda: seed("m", {"ndcg@10": 0.1}, {"data_version": "d", "eval_protocol": "p"}, "r"),
    }


def test_evaluate_raises_when_unreachable(env, monkeypatch, closed_port_url):
    # Scenario: Unreachable registry raises and decides nothing
    ops = _ops(env)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url)
    t0 = time.monotonic()
    with pytest.raises(RegistryUnavailable):
        ops["evaluate"]()
    assert time.monotonic() - t0 <= DEADLINE_WITH_SLACK


@pytest.mark.parametrize("op", ["apply", "submit", "promote", "rollback", "seed"])
def test_every_operation_fails_closed(env, monkeypatch, closed_port_url, op):
    # Scenario: Unreachable registry raises and decides nothing (all operations)
    ops = _ops(env)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url)
    t0 = time.monotonic()
    with pytest.raises(RegistryUnavailable):
        ops[op]()
    assert time.monotonic() - t0 <= DEADLINE_WITH_SLACK


def test_cli_exit_1_when_unreachable(env, monkeypatch, closed_port_url):
    # Scenario: CLI exits 1 when the registry is down
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url)
    r = run_cli("rollback", "m", timeout=60)
    assert r.returncode == 1
    assert "RegistryUnavailable" in r.stderr


def test_hanging_server_times_out_within_deadline(env, monkeypatch, hang_server):
    # Scenario: A hanging registry times out within the deadline
    ops = _ops(env)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", hang_server.url)
    t0 = time.monotonic()
    with pytest.raises(RegistryUnavailable):
        ops["evaluate"]()
    assert time.monotonic() - t0 <= DEADLINE_WITH_SLACK


def test_total_deadline_across_calls(env, monkeypatch, once_then_stall_server):
    # Scenario: First call answers, then the registry stalls
    ops = _ops(env)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", once_then_stall_server.url)
    t0 = time.monotonic()
    with pytest.raises(RegistryUnavailable):
        ops["submit"]()
    assert time.monotonic() - t0 <= DEADLINE_WITH_SLACK


def test_timeout_is_capped(env, monkeypatch, hang_server):
    # Scenario: Timeout is capped at 10 s per call
    monkeypatch.setenv("MLFLOW_HTTP_REQUEST_TIMEOUT", "300")
    check_reachable()  # against the file store; builds the client
    assert mev.MLFLOW_HTTP_REQUEST_TIMEOUT.get() == 10
    # behavioural proof against a hanging server: the 300 s value is not honoured
    monkeypatch.setenv("MLFLOW_HTTP_REQUEST_TIMEOUT", "300")
    monkeypatch.setenv("MLFLOW_TRACKING_URI", hang_server.url)
    t0 = time.monotonic()
    with pytest.raises(RegistryUnavailable):
        check_reachable()
    assert time.monotonic() - t0 <= DEADLINE_WITH_SLACK


def test_smaller_timeout_env_is_kept(env, monkeypatch):
    # Scenario: Timeout is capped at 10 s per call (only larger values are ignored)
    monkeypatch.setenv("MLFLOW_HTTP_REQUEST_TIMEOUT", "3")
    check_reachable()
    assert mev.MLFLOW_HTTP_REQUEST_TIMEOUT.get() == 3


def test_reachability_check(env, monkeypatch, closed_port_url):
    # Scenario: Reachability check
    check_reachable()  # live file store: returns without error
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url)
    t0 = time.monotonic()
    with pytest.raises(RegistryUnavailable):
        check_reachable()
    assert time.monotonic() - t0 <= DEADLINE_WITH_SLACK


def test_outage_mid_evaluate_keeps_champion(env, monkeypatch):
    # Scenario: Outage mid-evaluate leaves no champion change and no promoted version
    env.add_version("m", {"ndcg@10": 0.100}, alias="champion")
    c = env.candidate({"ndcg@10": 0.2})
    real = _mlflow.MlflowClient.set_model_version_tag

    def flaky(self, name, version, key, value, *a, **k):
        if key.startswith("gate."):
            raise MlflowException("API request to http://x/api failed with exception "
                                  "HTTPConnectionPool: Max retries exceeded (connection refused)")
        return real(self, name, version, key, value, *a, **k)

    monkeypatch.setattr(_mlflow.MlflowClient, "set_model_version_tag", flaky)
    with pytest.raises(RegistryUnavailable):
        evaluate("m", c["run_id"], c["metrics"], c["artifact_path"], c["lineage"])
    monkeypatch.setattr(_mlflow.MlflowClient, "set_model_version_tag", real)  # registry is back

    assert env.aliases("m") == {"champion": 1}
    new = [v for v in env.versions("m") if v != 1]
    for v in new:
        assert env.tags("m", v).get("gate.outcome") != "promoted"
        if env.tags("m", v).get("gate.outcome") is None:
            with pytest.raises(GateError):
                apply("m", v)
