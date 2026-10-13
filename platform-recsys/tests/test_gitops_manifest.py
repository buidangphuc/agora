"""Static checks for recsys-auto-train task 1.12: platform-gitops/platform/recsys/cronjob.yaml.

Scenarios RAT-33, RAT-36 and the recs-on-cluster egress requirement (recsys egress gains MLflow).
"""

from __future__ import annotations

import copy
from pathlib import Path

import yaml

GITOPS = Path(__file__).resolve().parents[2] / "platform-gitops"
CRONJOB = GITOPS / "platform/recsys/cronjob.yaml"
WANT_ENV = {
    "ENABLE_GBDT": "true",
    "ENABLE_TWO_TOWER": "true",
    "MLFLOW_TRACKING_URI": "http://mlflow:5050",
}


def load() -> list:
    return [d for d in yaml.safe_load_all(CRONJOB.read_text()) if d]


def of_kind(docs, kind):
    return [d for d in docs if d.get("kind") == kind]


def _mlflow_egress(docs) -> list:
    pol = of_kind(docs, "NetworkPolicy")[0]
    rules = []
    for rule in pol["spec"].get("egress", []):
        targets = [t.get("podSelector", {}).get("matchLabels", {}) for t in rule.get("to", [])]
        ports = {(p.get("protocol"), p.get("port")) for p in rule.get("ports", [])}
        if {"app": "mlflow"} in targets and ("TCP", 5050) in ports:
            rules.append(rule)
    return rules


def check_manifest(docs) -> list:
    """One problem per missing or wrong key (the manifest check RAT-36 refers to)."""
    problems = []
    data = of_kind(docs, "ConfigMap")[0]["data"]
    for key, want in WANT_ENV.items():
        if key not in data:
            problems.append(f"recsys-config is missing {key}")
        elif data[key] != want:
            problems.append(f"recsys-config {key}={data[key]!r}, want {want!r}")
    cron = of_kind(docs, "CronJob")[0]
    if cron["spec"]["schedule"] != "0 3 * * *":
        problems.append("CronJob schedule must be '0 3 * * *'")
    if cron["spec"]["jobTemplate"]["spec"].get("activeDeadlineSeconds") != 3600:
        problems.append("jobTemplate.spec.activeDeadlineSeconds must be 3600")
    if not _mlflow_egress(docs):
        problems.append("NetworkPolicy has no egress to app=mlflow on TCP 5050")
    return problems


def test_cronjob_enables_chain():
    # RAT-33
    assert check_manifest(load()) == []
    cm = of_kind(load(), "ConfigMap")[0]
    assert cm["metadata"]["name"] == "recsys-config"
    assert cm["data"]["SCHEDULE"] == "0 3 * * *"
    cron = of_kind(load(), "CronJob")[0]
    spec = cron["spec"]["jobTemplate"]["spec"]["template"]["spec"]
    train = next(c for c in spec["containers"] if c["name"] == "recsys-train")
    assert {"configMapRef": {"name": "recsys-config"}} in train["envFrom"]


def test_rat33_active_deadline_bounds_job_to_an_hour():
    cron = of_kind(load(), "CronJob")[0]
    assert cron["spec"]["jobTemplate"]["spec"]["activeDeadlineSeconds"] == 3600
    assert cron["spec"]["schedule"] == "0 3 * * *"


def test_rat33_egress_is_dns_plus_exactly_three_stores():
    # recs-on-cluster MODIFIED: egress is DNS plus qdrant, valkey, mlflow(5050); no ingress.
    pol = of_kind(load(), "NetworkPolicy")[0]
    assert "Ingress" in pol["spec"]["policyTypes"] and not pol["spec"].get("ingress")
    assert len(_mlflow_egress(load())) == 1
    apps = set()
    for rule in pol["spec"]["egress"]:
        for t in rule["to"]:
            labels = t.get("podSelector", {}).get("matchLabels", {})
            apps.add(labels.get("app") or labels.get("k8s-app"))
    assert apps == {"kube-dns", "qdrant", "valkey", "mlflow"}
    assert {p["port"] for p in _mlflow_egress(load())[0]["ports"]} == {5050}


def test_rat33_policy_label_matches_mlflow_pods():
    # The label the policy selects must be the one the platform/mlflow pods carry.
    labels = []
    for f in (GITOPS / "platform/mlflow").glob("*.yaml"):
        for d in yaml.safe_load_all(f.read_text()):
            if d and d.get("kind") in {"Deployment", "StatefulSet"} and d["metadata"]["name"] == "mlflow":
                labels.append(d["spec"]["template"]["metadata"]["labels"])
    assert labels and all(lbl.get("app") == "mlflow" for lbl in labels)
    assert _mlflow_egress(load())


def test_manifest_missing_key_fails():
    # RAT-36: baseline must pass, then each mutation is rejected naming the key.
    docs = load()
    assert check_manifest(docs) == []
    for key in WANT_ENV:
        broken = copy.deepcopy(docs)
        del of_kind(broken, "ConfigMap")[0]["data"][key]
        problems = check_manifest(broken)
        assert any(key in p for p in problems), (key, problems)
    off = copy.deepcopy(docs)
    of_kind(off, "ConfigMap")[0]["data"]["ENABLE_GBDT"] = "false"
    assert any("ENABLE_GBDT" in p for p in check_manifest(off))
