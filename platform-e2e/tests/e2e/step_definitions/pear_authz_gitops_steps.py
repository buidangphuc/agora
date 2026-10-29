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


GOLDEN = GITOPS / "tests" / "golden" / "team-verification.with-liveness.yaml"
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
    "charts/service is rendered for team-verification with its service values and "
    "--set migrate.enabled=false"
)
def render_verification_default(gitops):
    gitops["raw"] = _render_raw("team-verification", None, ("migrate.enabled=false",))
    gitops["docs"] = [d for d in yaml.safe_load_all(gitops["raw"]) if d]


@then(
    "the output is byte-identical to the golden render "
    "platform-gitops/tests/golden/team-verification.with-liveness.yaml and the liveness probe "
    "alone carries service liveness"
)
def default_matches_golden(gitops):
    assert GOLDEN.exists(), f"golden render missing: {GOLDEN}"
    assert gitops["raw"] == GOLDEN.read_text(), "migrate-disabled render differs from the with-liveness golden"
    assert "kind: Job" not in gitops["raw"], "migrate.enabled=false must render no Job"
    probes = _probes(gitops["docs"], "Deployment")
    assert _probe_service(probes["livenessProbe"]) == "liveness", probes["livenessProbe"]
    for name in ("readinessProbe", "startupProbe"):
        assert _probe_service(probes[name]) is None, f"{name} carries a service key: {probes[name]}"


@when(
    "charts/service is rendered for team-frontend (healthPath=/, no grpcPort) with "
    "--set probes.livenessService=liveness"
)
def render_frontend_liveness(gitops):
    gitops["raw"] = _render_raw("team-frontend", None, (LIVENESS,))
    gitops["probes"] = _probes([d for d in yaml.safe_load_all(gitops["raw"]) if d], "Deployment")


@then(
    "the container probes are httpGet probes and the rendered output contains no "
    "service: liveness"
)
def http_ignores_value(gitops):
    probes = gitops["probes"]
    assert probes, "no probes rendered for team-frontend"
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


GO_SERVICES = (
    "team-chat",
    "team-verification",
    "team-search",
    "team-audit",
    "team-notification",
    "team-promotion",
    "team-payment",
    "team-engagement",
    "team-domain",
    "team-gateway",
    "team-order",
    "team-identity",
    "team-sharing",
    "team-referral",
)
STRICT_ENV = {"staging": "staging", "prod": "production"}
BANNED = ("GRPC_REFLECTION_ENABLED", "MOCK_DATA")


def _names(obj) -> set[str]:
    out: set[str] = set()
    if isinstance(obj, dict):
        for key, value in obj.items():
            out.add(str(key))
            if key == "name" and isinstance(value, str):
                out.add(value)
            out |= _names(value)
    elif isinstance(obj, list):
        for value in obj:
            out |= _names(value)
    return out


@when(
    "charts/service is rendered for each of the fourteen chart-based Go services against its "
    "baseline values"
)
def render_every_go_service(gitops):
    gitops["probes_by_svc"] = {
        svc: _probes(_render(svc, None), "Deployment") for svc in GO_SERVICES
    }


@then(
    "each Deployment livenessProbe grpc service is liveness and each readinessProbe and "
    "startupProbe grpc has no service key"
)
def every_service_liveness(gitops):
    assert len(gitops["probes_by_svc"]) == 14, sorted(gitops["probes_by_svc"])
    for svc, probes in gitops["probes_by_svc"].items():
        assert set(probes) == {"readinessProbe", "livenessProbe", "startupProbe"}, (svc, probes)
        assert _probe_service(probes["livenessProbe"]) == "liveness", (svc, probes["livenessProbe"])
        for name in ("readinessProbe", "startupProbe"):
            assert "grpc" in probes[name], (svc, name, probes[name])
            assert "service" not in probes[name]["grpc"], (svc, name, probes[name])


@when("charts/service is rendered for team-gateway against its baseline values")
def render_gateway_baseline(gitops):
    gitops["raw"] = _render_raw("team-gateway", None)
    gitops["probes"] = _probes([d for d in yaml.safe_load_all(gitops["raw"]) if d], "Deployment")


@then(
    "the container has grpc probes on port 8080 and the output contains no httpGet probe and no "
    "healthPath"
)
def gateway_grpc_only(gitops):
    probes = gitops["probes"]
    assert set(probes) == {"readinessProbe", "livenessProbe", "startupProbe"}, probes
    for name, probe in probes.items():
        assert probe.get("grpc", {}).get("port") == 8080, (name, probe)
        assert "httpGet" not in probe, (name, probe)
    assert "httpGet" not in gitops["raw"], "gateway render contains an httpGet probe"
    assert "healthPath" not in gitops["raw"], "gateway render mentions healthPath"


@when(
    "each of the fourteen chart-based Go services is rendered against the staging overlay and "
    "the prod overlay"
)
def render_every_go_service_envs(gitops):
    gitops["env_by_svc"] = {
        svc: {env: _container_env(_render(svc, env)).get("ENV") for env in STRICT_ENV}
        for svc in GO_SERVICES
    }


@then('every container has ENV "staging" and "production" respectively')
def every_service_env(gitops):
    assert len(gitops["env_by_svc"]) == 14, sorted(gitops["env_by_svc"])
    for svc, got in gitops["env_by_svc"].items():
        assert got == STRICT_ENV, f"{svc} renders ENV {got}"


@when("charts/service is rendered for team-order against the staging overlay")
def render_order_staging(gitops):
    gitops["docs"] = _render("team-order", "staging")


@then(
    "exactly one migrate Job team-order-migrate is rendered with hook PreSync and delete policy "
    "BeforeHookCreation, running the Deployment image with args migrate up and ENV staging"
)
def order_migrate_job(gitops):
    docs = gitops["docs"]
    found = [d for d in docs if d["kind"] == "Job"]
    assert [j["metadata"]["name"] for j in found] == ["team-order-migrate"], found
    job = found[0]
    annotations = job["metadata"]["annotations"]
    assert annotations["argocd.argoproj.io/hook"] == "PreSync", annotations
    assert annotations["argocd.argoproj.io/hook-delete-policy"] == "BeforeHookCreation", annotations
    job_container = job["spec"]["template"]["spec"]["containers"][0]
    workload = next(d for d in docs if d["kind"] in WORKLOADS)
    deployment_container = workload["spec"]["template"]["spec"]["containers"][0]
    assert job_container["args"] == ["migrate", "up"], job_container
    assert job_container["image"] == deployment_container["image"], (job_container, deployment_container)
    job_env = {e["name"]: e.get("value") for e in job_container.get("env", [])}
    assert job_env.get("ENV") == "staging", job_env


@when("charts/service is rendered for team-order with --set strategy.type=Recreate")
def render_order_recreate(gitops):
    gitops["recreate"] = _render("team-order", None, ("strategy.type=Recreate",))
    gitops["default"] = _render("team-order", None)


@then("the Deployment strategy type is Recreate and the default render of team-order carries no strategy")
def recreate_strategy(gitops):
    recreate = next(d for d in gitops["recreate"] if d["kind"] == "Deployment")
    assert recreate["spec"]["strategy"]["type"] == "Recreate", recreate["spec"].get("strategy")
    default = next(d for d in gitops["default"] if d["kind"] == "Deployment")
    assert default["spec"].get("strategy", {}).get("type") != "Recreate", default["spec"].get("strategy")


@when(
    "charts/service is rendered for team-verification with --set migrate.enabled=true and again "
    "with --set migrate.enabled=false"
)
def render_verification_migrate_toggle(gitops):
    gitops["enabled"] = _render("team-verification", None, ("migrate.enabled=true",))
    gitops["disabled"] = _render("team-verification", None, ("migrate.enabled=false",))


@then("the first render contains one Job and the second contains none")
def migrate_toggle(gitops):
    assert len([d for d in gitops["enabled"] if d["kind"] == "Job"]) == 1, "enabled render lacks one Job"
    assert [d for d in gitops["disabled"] if d["kind"] == "Job"] == [], "disabled render still has a Job"


@when(
    "every file under platform-gitops/envs/staging and envs/prod and every shared baseline is "
    "scanned"
)
def scan_overlays(gitops):
    files = [GITOPS / "charts" / "service" / "values.yaml"]
    files += sorted((GITOPS / "envs" / "services").glob("*.yaml"))
    for env in ("staging", "prod"):
        files += sorted(p for p in (GITOPS / "envs" / env).rglob("*.yaml") if p.is_file())
    assert len(files) > 20, f"too few files scanned: {len(files)}"
    gitops["hits"] = {}
    for path in files:
        present = set()
        for doc in yaml.safe_load_all(path.read_text()):
            present |= _names(doc)
        found = sorted(present & set(BANNED))
        if found:
            gitops["hits"][str(path.relative_to(GITOPS))] = found


@then("none of them names GRPC_REFLECTION_ENABLED or MOCK_DATA")
def none_banned(gitops):
    assert gitops["hits"] == {}, f"deployed manifests name banned keys: {gitops['hits']}"
