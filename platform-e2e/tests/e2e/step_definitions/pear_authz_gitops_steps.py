"""Rendered-manifest assertions on platform-gitops (port-edge-authz-residuals / deploy-runtime).

Runs `helm template` on the real chart with the ApplicationSet's values layering and reads the
YAML it prints. No cluster and no stack involved.
"""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import pytest
import yaml
from pytest_bdd import then, when

GITOPS = Path(__file__).resolve().parents[3].parent / "platform-gitops"
WORKLOADS = ("Deployment", "Rollout")


@pytest.fixture
def gitops():
    return {}


def _render(service: str, env: str | None) -> list[dict]:
    if shutil.which("helm") is None:
        pytest.fail("helm is not installed")
    files = []
    if env:
        files = [
            f"envs/{env}/values.yaml",
            f"envs/services/{service}.yaml",
            f"envs/{env}/services.yaml",
            f"envs/{env}/services/{service}.yaml",
        ]
    else:
        files = [f"envs/services/{service}.yaml"]
    cmd = ["helm", "template", service, "charts/service", "--namespace", "marketplace"]
    for f in files:
        if (GITOPS / f).exists():
            cmd += ["-f", f]
    run = subprocess.run(cmd, cwd=GITOPS, capture_output=True, text=True, timeout=120)
    assert run.returncode == 0, f"helm template failed: {run.stderr[:600]}"
    return [d for d in yaml.safe_load_all(run.stdout) if d]


def _container_env(docs: list[dict]) -> dict[str, str]:
    workload = next(d for d in docs if d["kind"] in WORKLOADS)
    container = workload["spec"]["template"]["spec"]["containers"][0]
    return {e["name"]: e.get("value") for e in container.get("env", [])}


@when(
    "charts/service is rendered for team-gateway against the staging overlay and again against "
    "the prod overlay"
)
def render_gateway(gitops):
    gitops["env"] = {e: _container_env(_render("team-gateway", e)) for e in ("staging", "prod")}


@then('the gateway container has ENV "staging" and "production" respectively')
def gateway_env(gitops):
    got = {e: env.get("ENV") for e, env in gitops["env"].items()}
    assert got == {"staging": "staging", "prod": "production"}, f"rendered ENV by overlay: {got}"


@when("charts/service is rendered for team-ai with its service values")
def render_ai(gitops):
    gitops["docs"] = _render("team-ai", None)


@then(
    "a NetworkPolicy selects app team-ai and its only ingress sources are app team-gateway, "
    "app team-search-indexer and app prometheus"
)
def ai_network_policy(gitops):
    policies = [d for d in gitops["docs"] if d["kind"] == "NetworkPolicy"]
    assert policies, "no NetworkPolicy rendered for team-ai"
    spec = policies[0]["spec"]
    assert spec["podSelector"]["matchLabels"] == {"app": "team-ai"}, spec
    assert "Ingress" in spec["policyTypes"], spec
    sources = {
        peer["podSelector"]["matchLabels"]["app"]
        for rule in spec.get("ingress", [])
        for peer in rule.get("from", [])
        if "podSelector" in peer
    }
    others = [
        peer
        for rule in spec.get("ingress", [])
        for peer in rule.get("from", [])
        if "podSelector" not in peer
    ]
    assert sources == {
        "team-gateway",
        "team-search-indexer",
        "prometheus",
    }, f"ingress pod sources: {sources}"
    assert not others, f"non-pod ingress sources are not allowed for team-ai: {others}"
