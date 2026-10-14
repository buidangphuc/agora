"""Static compose, manifest and workflow checks for platform-finetune (add-platform-finetune).

Requirements covered: "The training image and compose job run the pipeline against agora infra",
"The weekly CronJob trains unattended on the GPU pool" and "CI runs the offline tests without a GPU
or dataset". No cluster and no GPU: `docker compose config`, `kubectl kustomize` renders, YAML parsing
and a CPU-host `docker build`.
"""

from __future__ import annotations

import ipaddress
import json
import shutil

import pytest
import yaml

from tests.e2e.static.pft_support import FINETUNE, GITOPS, ROOT, run

MANIFEST_DIR = "platform/finetune"
WORKFLOW = ROOT / ".github" / "workflows" / "services-ci.yml"


# --- helpers -----------------------------------------------------------------------------------


def _need(tool: str) -> None:
    if shutil.which(tool) is None:
        pytest.fail(f"{tool} is not installed")


def _compose(*args: str) -> str:
    _need("docker")
    res = run(["docker", "compose", "-f", "docker-compose.yaml", *args], timeout=180)
    assert res.returncode == 0, f"docker compose {' '.join(args)}: {res.stderr[:400]}"
    return res.stdout


@pytest.fixture(scope="module")
def rendered() -> list[dict]:
    _need("kubectl")
    assert (GITOPS / MANIFEST_DIR).is_dir(), f"platform-gitops/{MANIFEST_DIR} does not exist"
    res = run(["kubectl", "kustomize", MANIFEST_DIR], cwd=GITOPS, timeout=120)
    assert res.returncode == 0, f"kustomize render failed: {res.stderr[:400]}"
    return [d for d in yaml.safe_load_all(res.stdout) if d]


def _one(docs: list[dict], kind: str) -> dict:
    found = [d for d in docs if d["kind"] == kind]
    assert len(found) == 1, f"expected exactly one {kind}, rendered {len(found)}"
    return found[0]


def _pod_spec(cron: dict) -> dict:
    return cron["spec"]["jobTemplate"]["spec"]["template"]["spec"]


def _env_of(container: dict, cm_data: dict) -> dict:
    env = {e["name"]: e.get("value") for e in container.get("env", [])}
    for src in container.get("envFrom", []):
        if "configMapRef" in src:
            env.update(cm_data)
    return env


# --- scenario: The service is in the jobs profile only -----------------------------------------


def test_compose_service_in_jobs_profile():
    without = set(_compose("config", "--services").split())
    with_jobs = set(_compose("--profile", "jobs", "config", "--services").split())
    assert "platform-finetune" not in without, "platform-finetune starts without the jobs profile"
    assert "platform-finetune" in with_jobs, "platform-finetune is not defined under profile jobs"
    cfg = json.loads(_compose("--profile", "jobs", "config", "--format", "json"))
    svc = cfg["services"]["platform-finetune"]
    assert svc.get("profiles") == ["jobs"], f"profiles = {svc.get('profiles')}"
    env = svc.get("environment") or {}
    if isinstance(env, list):
        env = dict(e.split("=", 1) for e in env)
    assert env.get("MLFLOW_TRACKING_URI") == "http://mlflow:5050"
    for key in ("MLFLOW_S3_ENDPOINT_URL", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"):
        assert env.get(key), f"MinIO setting {key} missing from the compose service"


# --- scenario: The image builds without a GPU --------------------------------------------------


def test_image_builds_cpu_host():
    assert (FINETUNE / "Dockerfile").is_file(), "platform-finetune/Dockerfile does not exist"
    _need("docker")
    tag = "platform-finetune:e2e-cpu"
    build = run(["docker", "build", "-t", tag, "platform-finetune"], timeout=1800)
    assert build.returncode == 0, f"docker build failed: {build.stderr[-600:]}"
    info = run(["docker", "image", "inspect", tag, "--format", "{{json .Config}}"])
    assert info.returncode == 0, info.stderr[:300]
    cfg = json.loads(info.stdout)
    start = " ".join([*(cfg.get("Entrypoint") or []), *(cfg.get("Cmd") or [])])
    assert start.endswith("python main.py run"), f"image entrypoint/cmd is {start!r}"


# --- scenario: The manifest is weekly, non-overlapping and bounded -----------------------------


def test_cronjob_weekly_forbid(rendered):
    cron = _one(rendered, "CronJob")
    spec = cron["spec"]
    assert spec["schedule"] == "0 4 * * 0"
    assert spec["concurrencyPolicy"] == "Forbid"
    assert spec["jobTemplate"]["spec"]["backoffLimit"] == 1
    assert _pod_spec(cron)["restartPolicy"] == "Never"
    cm_data = {}
    for d in rendered:
        if d["kind"] == "ConfigMap":
            cm_data.update(d.get("data") or {})
    env = _env_of(_pod_spec(cron)["containers"][0], cm_data)
    assert env.get("MLFLOW_TRACKING_URI") == "http://mlflow:5050", env.get("MLFLOW_TRACKING_URI")
    argo = ROOT / "platform-gitops" / "argocd" / "apps" / "finetune.yaml"
    assert argo.is_file(), "ArgoCD Application argocd/apps/finetune.yaml does not exist"
    app = yaml.safe_load(argo.read_text("utf-8"))
    assert app["kind"] == "Application" and app["spec"]["source"]["path"] == MANIFEST_DIR


# --- scenario: The job lands on a GPU node -----------------------------------------------------


def test_cronjob_gpu_pool(rendered):
    pod = _pod_spec(_one(rendered, "CronJob"))
    selector = pod.get("nodeSelector") or {}
    assert selector, "no nodeSelector"
    assert any("gpu" in f"{k}={v}".lower() for k, v in selector.items()), f"selector {selector}"
    tolerations = pod.get("tolerations") or []
    assert any(
        "gpu" in str(t.get("key", "")).lower() for t in tolerations
    ), f"tolerations {tolerations}"
    limits = pod["containers"][0]["resources"]["limits"]
    assert str(limits.get("nvidia.com/gpu")) == "1", f"limits {limits}"


# --- scenario: The pod is isolated and unreachable ---------------------------------------------


def _classify(rule: dict) -> str:
    ports = {(p.get("protocol", "TCP"), p.get("port")) for p in rule.get("ports", [])}
    peers = rule.get("to", [])
    if ("UDP", 53) in ports:
        return "dns"
    if any(p.get("ipBlock", {}).get("cidr") == "0.0.0.0/0" for p in peers):
        assert ports == {("TCP", 443)}, f"internet egress must be TCP 443 only, got {ports}"
        excepts = [c for p in peers for c in p["ipBlock"].get("except", [])]
        assert excepts, "TCP 443 egress to 0.0.0.0/0 has no cluster CIDR in except"
        for c in excepts:
            ipaddress.ip_network(c)
        return "https"
    labels = " ".join(
        f"{k}={v}"
        for p in peers
        for k, v in (p.get("podSelector", {}).get("matchLabels") or {}).items()
    )
    for name in ("mlflow", "minio"):
        if name in labels:
            return name
    raise AssertionError(f"unexpected egress rule: {rule}")


def test_networkpolicy_egress_and_no_service(rendered):
    pol = _one(rendered, "NetworkPolicy")["spec"]
    assert not pol.get("ingress"), "NetworkPolicy must have no ingress rule"
    assert "Egress" in pol["policyTypes"]
    kinds = sorted(_classify(r) for r in pol.get("egress", []))
    assert kinds == ["dns", "https", "minio", "mlflow"], f"egress allows {kinds}"
    present = {d["kind"] for d in rendered}
    assert not present & {"Service", "Ingress"}, f"unexpected kinds rendered: {present}"


# --- scenarios: The CI job exists and is bounded / A missing dependency fails the job ----------


def test_ci_job_offline():
    wf = yaml.safe_load(WORKFLOW.read_text("utf-8"))
    job = (wf.get("jobs") or {}).get("platform-finetune")
    assert job, "job platform-finetune is not defined in services-ci.yml"
    assert job.get("timeout-minutes") == 10
    runs_on = str(job.get("runs-on", "")).lower()
    assert "gpu" not in runs_on and runs_on.startswith("ubuntu"), f"runs-on {runs_on!r}"
    default_wd = ((job.get("defaults") or {}).get("run") or {}).get("working-directory")
    steps = job.get("steps") or []
    pytest_steps = [s for s in steps if "pytest" in str(s.get("run", ""))]
    assert pytest_steps, "no step runs pytest"
    for s in pytest_steps:
        assert (s.get("working-directory") or default_wd) == "platform-finetune"
        assert "tests" in s["run"]
    # a missing dependency must fail the job: nothing may swallow pytest's exit status or skip tests
    assert not job.get("continue-on-error")
    for s in steps:
        assert not s.get("continue-on-error"), f"step {s.get('name')} continues on error"
        cmd = str(s.get("run", ""))
        assert "|| true" not in cmd and "--continue-on-collection-errors" not in cmd
    install = " ".join(str(s.get("run", "")) for s in steps if "install" in str(s.get("run", "")))
    assert "dev" in install, "CPU dev extras are not installed"
