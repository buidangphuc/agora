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


def _render_raw(service: str, env: str | None, sets: tuple[str, ...] = ()) -> str:
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
    for item in sets:
        cmd += ["--set", item]
    run = subprocess.run(cmd, cwd=GITOPS, capture_output=True, text=True, timeout=120)
    assert run.returncode == 0, f"helm template failed: {run.stderr[:600]}"
    return run.stdout


def _render(service: str, env: str | None, sets: tuple[str, ...] = ()) -> list[dict]:
    return [d for d in yaml.safe_load_all(_render_raw(service, env, sets)) if d]


def _probes(docs: list[dict], kind: str) -> dict[str, dict]:
    workload = next(d for d in docs if d["kind"] == kind)
    container = workload["spec"]["template"]["spec"]["containers"][0]
    return {
        name: container[name]
        for name in ("readinessProbe", "livenessProbe", "startupProbe")
        if name in container
    }


def _probe_service(probe: dict) -> str | None:
    return (probe.get("grpc") or {}).get("service")


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
    "app team-search-indexer, app team-domain and app prometheus"
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
        "team-domain",
        "prometheus",
    }, f"ingress pod sources: {sources}"
    assert not others, f"non-pod ingress sources are not allowed for team-ai: {others}"


GOLDEN = GITOPS / "tests" / "golden" / "team-verification.pre-liveness.yaml"
LIVENESS = "probes.livenessService=liveness"


@when(
    "charts/service is rendered for team-verification (grpcPort 50064) with "
    "--set probes.livenessService=liveness"
)
def render_verification_liveness(gitops):
    gitops["probes"] = _probes(_render("team-verification", None, (LIVENESS,)), "Deployment")


@then(
    "the container livenessProbe is a grpc probe with service liveness, and the readinessProbe "
    "and startupProbe have no service key"
)
def liveness_by_name(gitops):
    probes = gitops["probes"]
    assert set(probes) == {"readinessProbe", "livenessProbe", "startupProbe"}, probes
    assert probes["livenessProbe"]["grpc"]["port"] == 50064, probes["livenessProbe"]
    assert _probe_service(probes["livenessProbe"]) == "liveness", probes["livenessProbe"]
    for name in ("readinessProbe", "startupProbe"):
        assert "service" not in probes[name]["grpc"], f"{name} must keep probing overall: {probes[name]}"


@when(
    "charts/service is rendered for team-verification with its service values, without "
    "probes.livenessService"
)
def render_verification_default(gitops):
    gitops["raw"] = _render_raw("team-verification", None)
    gitops["docs"] = [d for d in yaml.safe_load_all(gitops["raw"]) if d]


@then(
    "the output is byte-identical to the golden render "
    "platform-gitops/tests/golden/team-verification.pre-liveness.yaml and no probe has a "
    "service key"
)
def default_unchanged(gitops):
    assert GOLDEN.exists(), f"golden render missing: {GOLDEN}"
    assert gitops["raw"] == GOLDEN.read_text(), "default render differs from the pre-change golden"
    probes = _probes(gitops["docs"], "Deployment")
    assert probes, "no probes rendered"
    for name, probe in probes.items():
        assert _probe_service(probe) is None, f"{name} carries a service key: {probe}"
    control = _probes(_render("team-verification", None, (LIVENESS,)), "Deployment")
    assert _probe_service(control["livenessProbe"]) == "liveness", (
        "positive control: setting the value must change the liveness probe"
    )


@when(
    "charts/service is rendered for team-gateway (healthPath=/healthz, no grpcPort) with "
    "--set probes.livenessService=liveness"
)
def render_gateway_liveness(gitops):
    gitops["raw"] = _render_raw("team-gateway", None, (LIVENESS,))
    gitops["probes"] = _probes([d for d in yaml.safe_load_all(gitops["raw"]) if d], "Deployment")


@then(
    "the container probes are httpGet probes and the rendered output contains no "
    "service: liveness"
)
def http_ignores_value(gitops):
    probes = gitops["probes"]
    assert probes, "no probes rendered for team-gateway"
    for name, probe in probes.items():
        assert "httpGet" in probe, f"{name} is not an httpGet probe: {probe}"
    assert "service: liveness" not in gitops["raw"], "HTTP render leaked service: liveness"
    control = _probes(_render("team-verification", None, (LIVENESS,)), "Deployment")
    assert _probe_service(control["livenessProbe"]) == "liveness", (
        "positive control: the same value must reach a gRPC service's liveness probe"
    )


@when(
    "charts/service is rendered for team-verification with --set rollout.enabled=true "
    "--set probes.livenessService=liveness"
)
def render_verification_rollout(gitops):
    docs = _render("team-verification", None, ("rollout.enabled=true", LIVENESS))
    gitops["kinds"] = {d["kind"] for d in docs}
    gitops["probes"] = _probes(docs, "Rollout")


@then("the Rollout container livenessProbe carries service liveness")
def rollout_liveness(gitops):
    assert "Rollout" in gitops["kinds"], f"no Rollout rendered: {gitops['kinds']}"
    probe = gitops["probes"]["livenessProbe"]
    assert _probe_service(probe) == "liveness", probe
    for name in ("readinessProbe", "startupProbe"):
        assert _probe_service(gitops["probes"][name]) is None, gitops["probes"][name]
