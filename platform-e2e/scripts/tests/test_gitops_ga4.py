"""Static tests for the GA4 GitOps wiring (add-ga4-export-import, task gitops-ga4).

Scenarios: "The job is a no-op when unconfigured" (ga4-export-import) and
"Divergence beyond the threshold raises the alert" (tracking-data-quality).
No cluster: `kubectl kustomize` renders, YAML parsing, `promtool test rules`.
"""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import pytest
import yaml

ROOT = Path(__file__).resolve().parents[3]
GITOPS = ROOT / "platform-gitops"
ALERTS = GITOPS / "platform/monitoring/alerts.yaml"
PROM = GITOPS / "platform/monitoring/prometheus.yaml"


def _render() -> list[dict]:
    if shutil.which("kubectl") is None:
        pytest.fail("kubectl is not installed")
    run = subprocess.run(["kubectl", "kustomize", "platform/team-analytics"], cwd=GITOPS,
                         capture_output=True, text=True, timeout=120)  # fmt: skip
    assert run.returncode == 0, run.stderr[:400]
    return [d for d in yaml.safe_load_all(run.stdout) if d]


def _by_kind(kind: str, name: str) -> dict:
    for d in _render():
        if d["kind"] == kind and d["metadata"]["name"] == name:
            return d
    raise AssertionError(f"{kind}/{name} not rendered")


def _cron() -> dict:
    crons = [d for d in _render() if d["kind"] == "CronJob" and "ga4" in d["metadata"]["name"]]
    assert len(crons) == 1, "expected exactly one ga4 import CronJob in platform/team-analytics"
    return crons[0]


def _container(cron: dict) -> dict:
    cs = cron["spec"]["jobTemplate"]["spec"]["template"]["spec"]["containers"]
    assert len(cs) == 1
    return cs[0]


def _env(c: dict) -> dict:
    return {e["name"]: e.get("value") for e in c.get("env", [])}


# --- scenario: The job is a no-op when unconfigured -------------------------------------


def test_job_noop_when_unconfigured__cronjob_runs_ga4import_daily_with_team_analytics_image():
    cron = _cron()
    c = _container(cron)
    consumer = _by_kind("Deployment", "team-analytics")["spec"]["template"]["spec"]["containers"][0]
    assert c["image"] == consumer["image"], "must reuse the team-analytics image (same tag)"
    assert (c.get("command") or []) + (c.get("args") or []) == ["/ga4import"] or c.get("command") == ["/ga4import"]
    sched = cron["spec"]["schedule"].split()
    assert len(sched) == 5 and sched[2:] == ["*", "*", "*"], "daily schedule"
    assert cron["spec"]["concurrencyPolicy"] == "Forbid"
    assert cron["spec"]["jobTemplate"]["spec"]["template"]["spec"]["restartPolicy"] == "Never"


def test_job_noop_when_unconfigured__dataset_unset_and_settings_declared():
    env = _env(_container(_cron()))
    assert env.get("GA4_EXPORT_DATASET") in (None, ""), "dataset must be empty in the base render (no-op)"
    assert "GA4_EXPORT_DATASET" in env, "setting must be declared so ops can fill it"
    assert "GA4_EXPORT_SOURCE" in env
    assert env.get("GA4_RECONCILE_MAX_DIFF_RATIO") == "0.35"


@pytest.mark.parametrize("path", [
    "envs/local/values.yaml", "envs/staging/values.yaml", "envs/staging/services.yaml",
])
def test_job_noop_when_unconfigured__local_and_staging_never_set_dataset(path):
    text = (GITOPS / path).read_text()
    for line in text.splitlines():
        if "GA4_EXPORT_DATASET" in line:
            val = line.split("GA4_EXPORT_DATASET", 1)[1].strip(" :=\"'{},value")
            assert val == "", f"{path} sets GA4_EXPORT_DATASET: {line.strip()}"


# --- consumer exposes metrics ------------------------------------------------------------


def test_reconcile_gauge_scrape__consumer_exposes_9109_and_prometheus_scrapes_it():
    dep = _by_kind("Deployment", "team-analytics")
    c = dep["spec"]["template"]["spec"]["containers"][0]
    assert any(p.get("containerPort") == 9109 for p in c.get("ports", [])), "containerPort 9109"
    svc = _by_kind("Service", "team-analytics")
    assert any(p["port"] == 9109 for p in svc["spec"]["ports"]), "Service must expose 9109"
    cm = yaml.safe_load(PROM.read_text().split("\n---")[0])
    cfg = yaml.safe_load(cm["data"]["prometheus.yml"])
    targets = [t for j in cfg["scrape_configs"] for sc in j.get("static_configs", []) for t in sc["targets"]]
    assert "team-analytics.marketplace.svc:9109" in targets


# --- scenario: Divergence beyond the threshold raises the alert -------------------------


def _rule_file(tmp: Path) -> Path:
    cm = yaml.safe_load(ALERTS.read_text())
    out = tmp / "rules.yml"
    out.write_text(cm["data"]["alerts.yml"])
    return out


def _promtool(tmp: Path, value: str) -> subprocess.CompletedProcess:
    """Run a promtool test that expects NO alert; if one fires, promtool prints it."""
    if shutil.which("promtool") is None:
        pytest.fail("promtool is not installed")
    _rule_file(tmp)
    (tmp / "t.yml").write_text(f"""rule_files: [rules.yml]
evaluation_interval: 1m
tests:
  - interval: 1m
    input_series:
      - series: 'ga4_reconcile_diff_ratio{{event_type="view"}}'
        values: '{value}x240'
    alert_rule_test:
      - eval_time: 3h
        alertname: Ga4ReconcileDivergence
        exp_alerts: []
""")
    return subprocess.run(["promtool", "test", "rules", "t.yml"], cwd=tmp, capture_output=True, text=True)


def _firing_output(tmp: Path, value: str) -> str:
    r = _promtool(tmp, value)
    assert r.returncode != 0, "no alert fired"
    return r.stdout + r.stderr


def test_divergence_alert__rule_exists_with_runbook_and_severity():
    rules = yaml.safe_load(yaml.safe_load(ALERTS.read_text())["data"]["alerts.yml"])
    found = [r for g in rules["groups"] for r in g["rules"] if r.get("alert") == "Ga4ReconcileDivergence"]
    assert len(found) == 1
    assert found[0]["for"], "must have a for: duration"
    assert found[0]["annotations"].get("summary") and found[0]["annotations"].get("runbook")
    assert found[0]["labels"].get("severity")


def test_divergence_alert__fires_when_ratio_beyond_threshold(tmp_path):
    out = _firing_output(tmp_path, "0.5")
    assert "Ga4ReconcileDivergence" in out and "event_type" in out, (
        f"alert must be firing at ratio 0.5 (got: {out[:400]})"
    )


def test_divergence_alert__does_not_fire_at_threshold(tmp_path):
    # "Divergence at the threshold does not alert": ratio 0.35 -> no alert.
    r = _promtool(tmp_path, "0.35")
    assert r.returncode == 0, r.stdout + r.stderr


def test_divergence_alert__fires_just_above_threshold(tmp_path):
    out = _firing_output(tmp_path, "0.36")
    assert "Ga4ReconcileDivergence" in out and "event_type" in out
