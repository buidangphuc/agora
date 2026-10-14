"""Scenarios: exit codes of `python main.py run` (D4) with a local file-store registry.

Each test runs the real entry point in a subprocess; only the heavy stages are stubbed by
tests/cli_driver.py (see its docstring). The registry is a local MLflow file store, so the
suite needs no network. Process-time bounds (15 s) are not asserted here: interpreter and
torch import time would make them flaky; the 12 s deadline is covered by the library suite.
"""

import json
import os
import subprocess
import sys

from conftest import TESTS_DIR

DRIVER = TESTS_DIR / "cli_driver.py"


def run_job(tmp_path, reg, metrics, mode="ok", tracking_uri=None, policies=None, run_id=None):
    env = dict(os.environ)
    env.update({
        "FT_TEST_ROOT": str(tmp_path / "artifacts"),
        "FT_TEST_MODE": mode,
        "FT_TEST_RUN_ID": run_id or reg.new_run(),
        "FT_TEST_METRICS": json.dumps(metrics),
        "MLFLOW_TRACKING_URI": tracking_uri or reg.uri,
        "MLFLOW_HTTP_REQUEST_TIMEOUT": "5",
    })
    env.pop("MLFLOW_REGISTRY_URI", None)
    env.pop("MODELREGISTRY_POLICIES", None)
    if policies:
        env["MODELREGISTRY_POLICIES"] = str(policies)
    (tmp_path / "artifacts").mkdir(exist_ok=True)
    return subprocess.run([sys.executable, str(DRIVER)], capture_output=True, text=True, env=env,
                          timeout=240, check=False)


def started(tmp_path):
    log = tmp_path / "artifacts" / "stages.log"
    return log.read_text().split() if log.exists() else []


def test_below_floor_run_exits_0(reg, tmp_path):
    # Scenario: A below-floor run through the CLI exits 0
    r = run_job(tmp_path, reg, {"macro_f1": 0.40, "accuracy": 0.4})
    assert r.returncode == 0, r.stderr[-2000:]
    assert reg.tags(1)["gate.outcome"] == "rejected"
    assert "GateFailed" not in r.stdout + r.stderr
    assert started(tmp_path) == ["ingest", "validate", "transform", "train", "evaluate"]


def test_preflight_unreachable_exits_1_before_training(reg, tmp_path, closed_port_url):
    # Scenario: The pre-flight stops the job before training
    r = run_job(tmp_path, reg, {"macro_f1": 0.9}, tracking_uri=closed_port_url)
    assert r.returncode == 1, r.stderr[-2000:]
    assert started(tmp_path) == []  # neither ingest nor train started


def test_stage_exception_exits_1(reg, tmp_path):
    # Scenario: An unexpected stage exception exits 1
    reg.add_version({"macro_f1": 0.8}, alias="champion")
    before = reg.snapshot()
    r = run_job(tmp_path, reg, {"macro_f1": 0.9}, mode="raise")
    assert r.returncode == 1, r.stderr[-2000:]
    assert started(tmp_path)[-1] == "train"
    assert reg.snapshot() == before


def test_rejected_exits_0_outage_exits_1(reg, tmp_path, closed_port_url):
    # Scenario: A rejected run completes and an outage run fails by exit code
    reg.add_version({"macro_f1": 0.8}, alias="champion")
    rejected = run_job(tmp_path, reg, {"macro_f1": 0.30})
    assert rejected.returncode == 0, rejected.stderr[-2000:]
    after_rejected = reg.snapshot()
    assert after_rejected["aliases"] == {"champion": 1}
    assert after_rejected["tags"][2]["gate.outcome"] == "rejected"

    outage = run_job(tmp_path, reg, {"macro_f1": 0.95}, tracking_uri=closed_port_url)
    assert outage.returncode == 1, outage.stderr[-2000:]
    assert reg.snapshot() == after_rejected


def test_missing_policy_file_exits_2(reg, tmp_path):
    # Scenario: A malformed policy path is a config error
    r = run_job(tmp_path, reg, {"macro_f1": 0.9}, policies=tmp_path / "does-not-exist.yaml")
    assert r.returncode == 2, r.stderr[-2000:]
    assert "PolicyError" in r.stdout + r.stderr
    assert reg.versions() == []
