"""Static checks for change add-model-registry (task 4, gitops): platform/mlflow + ArgoCD app.

kustomize is not required: the 'render' is the kustomization's resources, loaded in order.
Run: platform-e2e/.venv/bin/python -m pytest platform-gitops/tests/test_mlflow_manifests.py
"""
import re
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parent.parent
MLFLOW_DIR = ROOT / "platform/mlflow"
APP = ROOT / "argocd/apps/mlflow.yaml"
SENSITIVE = re.compile(r"password|passwd|secret|access_?key", re.I)
REF_KINDS = {"ExternalSecret", "Secret"}


def render(directory: Path) -> list:
    """Approximate `kustomize build`: every doc of every resource listed in kustomization.yaml."""
    kfile = directory / "kustomization.yaml"
    assert kfile.exists(), f"{kfile} does not exist"
    kust = yaml.safe_load(kfile.read_text())
    docs = []
    for res in kust.get("resources", []):
        path = directory / res
        assert path.is_file(), f"kustomization lists missing resource {res}"
        docs += [d for d in yaml.safe_load_all(path.read_text()) if d]
    return docs


def of_kind(docs, kind):
    return [d for d in docs if d.get("kind") == kind]


def pod_specs(docs):
    for d in docs:
        if d.get("kind") in {"Deployment", "StatefulSet"}:
            yield d, d["spec"]["template"]["spec"]


def test_no_ingress_clusterip_only():
    """Scenario: No ingress is rendered."""
    docs = render(MLFLOW_DIR)
    assert of_kind(docs, "Ingress") == []
    assert of_kind(docs, "HTTPRoute") == []
    # the postgres-mlflow backend must be part of the render, headless/ClusterIP only
    assert any(d["metadata"]["name"] == "postgres-mlflow" for d in of_kind(docs, "StatefulSet"))
    for svc in of_kind(docs, "Service"):
        assert svc["spec"].get("type", "ClusterIP") == "ClusterIP", svc["metadata"]["name"]
    mlflow_svcs = [s for s in of_kind(docs, "Service")
                   if any(p.get("port") == 5050 for p in s["spec"].get("ports", []))]
    assert len(mlflow_svcs) == 1, "exactly one Service exposes port 5050"


def test_credentials_from_vault_refs():
    """Scenario: Manifests without a literal secret."""
    docs = render(MLFLOW_DIR)
    assert of_kind(docs, "ExternalSecret"), "credentials must come from an ExternalSecret (Vault)"
    offenders = []
    for d in docs:
        name = f'{d["kind"]}/{d["metadata"]["name"]}'
        if d["kind"] == "Secret":  # a committed Secret carries literals
            offenders.append(f"{name}: literal Secret manifest")
        if d["kind"] == "ConfigMap":
            for k, v in (d.get("data") or {}).items():
                if SENSITIVE.search(k) and v:
                    offenders.append(f"{name}: data.{k}")
    for d, spec in pod_specs(docs):
        for c in spec["containers"]:
            for e in c.get("env", []):
                if "valueFrom" in e:
                    assert "secretKeyRef" in e["valueFrom"] or "configMapKeyRef" in e["valueFrom"]
                    continue
                val = str(e.get("value", ""))
                if SENSITIVE.search(e["name"]) and val and not val.startswith("$("):
                    offenders.append(f'{d["metadata"]["name"]}: env {e["name"]} literal')
                if re.search(r"://[^/\s:@]+:[^/\s@$]+@", val):  # user:pass@host URI
                    offenders.append(f'{d["metadata"]["name"]}: env {e["name"]} embeds credentials')
    assert not offenders, offenders


def test_argocd_app_source_path():
    """Scenario: ArgoCD application points at the manifests."""
    assert APP.exists(), f"{APP} does not exist"
    app = yaml.safe_load(APP.read_text())
    assert app["kind"] == "Application"
    assert app["spec"]["source"]["path"] == "platform/mlflow"
    assert app["spec"]["destination"].get("namespace")
