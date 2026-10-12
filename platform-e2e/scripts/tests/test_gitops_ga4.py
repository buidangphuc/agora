"""Static tests for the GA4 GitOps wiring (add-ga4-export-import, task gitops-ga4 / ga4import-in-consumer).

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


def _consumer() -> dict:
    return _by_kind("Deployment", "team-analytics")["spec"]["template"]["spec"]["containers"][0]


def _env(c: dict) -> dict:
    return {e["name"]: e.get("value") for e in c.get("env", [])}


# --- scenario: The job is a no-op when unconfigured (import runs in the consumer, no CronJob) ----


def test_job_noop_when_unconfigured__no_cronjob_in_team_analytics_kustomize_output():
    crons = [d["metadata"]["name"] for d in _render() if d["kind"] in ("CronJob", "Job")]
    assert crons == [], f"import runs inside the consumer; found {crons}"


def test_job_noop_when_unconfigured__ga4_settings_declared_on_consumer_deployment():
    env = _env(_consumer())
    for k in ("GA4_EXPORT_SOURCE", "GA4_EXPORT_DATASET", "GA4_IMPORT_AT", "GA4_RECONCILE_MAX_DIFF_RATIO"):
        assert k in env, f"{k} must be declared on the consumer Deployment"
    assert env["GA4_EXPORT_DATASET"] in (None, ""), "dataset must be empty in the base render (no-op)"
    assert env["GA4_RECONCILE_MAX_DIFF_RATIO"] == "0.35"
    assert env["GA4_IMPORT_AT"] == "03:30"


@pytest.mark.parametrize("path", [
    "envs/local/values.yaml", "envs/staging/values.yaml", "envs/staging/services.yaml",
])
def test_job_noop_when_unconfigured__local_and_staging_never_set_dataset(path):
    text = (GITOPS / path).read_text()
    for line in text.splitlines():
        if "GA4_EXPORT_DATASET" in line:
            val = line.split("GA4_EXPORT_DATASET", 1)[1].strip(" :=\"'{},value")
            assert val == "", f"{path} sets GA4_EXPORT_DATASET: {line.strip()}"


# --- scenario: The internal trigger is not exposed through the gateway ----------------------


def test_trigger_not_exposed__no_ingress_or_public_service_for_team_analytics():
    docs = _render()
    assert not [d for d in docs if d["kind"] in ("Ingress", "HTTPRoute", "IngressRoute")]
    svc = _by_kind("Service", "team-analytics")
    assert svc["spec"].get("type", "ClusterIP") == "ClusterIP"
    assert not any(p.get("nodePort") for p in svc["spec"]["ports"])
    for path in sorted((GITOPS / "envs").rglob("*.y*ml")):
        for d in yaml.safe_load_all(path.read_text()):
            if not isinstance(d, dict):
                continue
            ing = d.get("ingress") or {}
            if isinstance(ing, dict) and ing.get("enabled") and (
                d.get("name") == "team-analytics" or path.stem == "team-analytics"
            ):
                raise AssertionError(f"{path} enables an ingress for team-analytics")


def test_trigger_not_exposed__no_gateway_route_maps_internal_ga4import():
    hits = []
    for p in (ROOT / "team-gateway").rglob("*"):
        if p.suffix in (".go", ".yaml", ".yml", ".json") and p.is_file() and "_test.go" not in p.name:
            if "/internal/ga4import" in p.read_text(errors="ignore"):
                hits.append(str(p.relative_to(ROOT)))
    assert hits == [], f"gateway must not route the internal trigger: {hits}"


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
