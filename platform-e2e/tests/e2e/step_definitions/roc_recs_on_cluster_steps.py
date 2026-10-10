"""Static assertions on platform-gitops (recs-on-cluster). No cluster: raw-manifest parsing plus
`helm template` of charts/service for team-ai."""

from __future__ import annotations

import re
import shutil
import subprocess
from pathlib import Path

import pytest
import yaml
from pytest_bdd import then, when

ROOT = Path(__file__).resolve().parents[3].parent
GITOPS = ROOT / "platform-gitops"
INFRA = "platform/infra"


@pytest.fixture
def roc():
    return {}


def _docs(rel: str) -> list[dict]:
    return [d for d in yaml.safe_load_all((GITOPS / rel).read_text()) if d]


def _one(docs, kind, name):
    return next(d for d in docs if d["kind"] == kind and d["metadata"]["name"] == name)


def _pod(workload):
    return workload["spec"]["template"]["spec"]


def _services() -> dict[str, dict]:
    """Every Service defined by the raw manifests of the cluster, by name."""
    out = {}
    for f in (GITOPS / "platform").rglob("*.yaml"):
        for d in yaml.safe_load_all(f.read_text()):
            if d and d.get("kind") == "Service":
                out[d["metadata"]["name"]] = d
    return out


def _peers(rules) -> list[dict]:
    return [peer for rule in rules for peer in rule.get("from", rule.get("to", []))]


def _apps() -> list[dict]:
    return [yaml.safe_load(p.read_text()) for p in sorted((GITOPS / "argocd/apps").glob("*.yaml"))]


def _synced_paths() -> set[str]:
    paths = set()
    for app in _apps():
        spec = app["spec"]
        for src in [spec.get("source", {}), *spec.get("sources", [])]:
            if src.get("path"):
                paths.add(src["path"])
    return paths


# ── Qdrant ────────────────────────────────────────────────────────────────────────────────────


@when("the Qdrant manifest and the compose infra file are read")
def read_qdrant_compose(roc):
    roc["q"] = _docs(f"{INFRA}/qdrant.yaml")
    compose = yaml.safe_load((ROOT / "platform-core/infra/docker-compose.yaml").read_text())
    roc["compose_image"] = compose["services"]["qdrant"]["image"]


@then("the Qdrant StatefulSet image tag equals the compose qdrant image tag and is not latest")
def qdrant_pinned(roc):
    image = _pod(_one(roc["q"], "StatefulSet", "qdrant"))["containers"][0]["image"]
    name, _, tag = image.partition(":")
    c_name, _, c_tag = roc["compose_image"].partition(":")
    assert name == c_name == "qdrant/qdrant", (image, roc["compose_image"])
    assert tag and tag != "latest" and tag == c_tag, (tag, c_tag)


@when("the Qdrant manifest is rendered")
def render_qdrant(roc):
    roc["q"] = _docs(f"{INFRA}/qdrant.yaml")


@then(
    "it has a volume claim template mounted at /qdrant/storage, resource requests and limits, and a "
    "non-root, read-only-root-filesystem security context with all capabilities dropped, and a "
    "Service qdrant exposes 6333 and 6334"
)
def qdrant_hardened(roc):
    sts = _one(roc["q"], "StatefulSet", "qdrant")
    pod = _pod(sts)
    c = pod["containers"][0]
    claims = {t["metadata"]["name"] for t in sts["spec"]["volumeClaimTemplates"]}
    storage = [m for m in c["volumeMounts"] if m["mountPath"] == "/qdrant/storage"]
    assert len(storage) == 1 and storage[0]["name"] in claims, (storage, claims)
    for side in ("requests", "limits"):
        assert {"cpu", "memory"} <= set(c["resources"][side]), c["resources"]
    assert pod["securityContext"]["runAsNonRoot"] is True
    assert pod["securityContext"]["runAsUser"] != 0
    sc = c["securityContext"]
    assert sc["readOnlyRootFilesystem"] is True and sc["allowPrivilegeEscalation"] is False
    assert sc["capabilities"]["drop"] == ["ALL"]
    svc = _one(roc["q"], "Service", "qdrant")
    assert {p["port"] for p in svc["spec"]["ports"]} == {6333, 6334}
    assert svc["spec"]["selector"] == sts["spec"]["selector"]["matchLabels"]


@when("the Qdrant NetworkPolicy is rendered")
def render_qdrant_np(roc):
    roc["np"] = _one(_docs(f"{INFRA}/qdrant.yaml"), "NetworkPolicy", "qdrant-isolation")


@then("its only ingress sources are app team-ai and app platform-recsys")
def qdrant_np(roc):
    spec = roc["np"]["spec"]
    assert spec["podSelector"]["matchLabels"] == {"app": "qdrant"}
    assert "Ingress" in spec["policyTypes"]
    peers = _peers(spec["ingress"])
    assert all("podSelector" in p and len(p) == 1 for p in peers), peers
    assert {p["podSelector"]["matchLabels"]["app"] for p in peers} == {"team-ai", "platform-recsys"}


# ── team-ai ───────────────────────────────────────────────────────────────────────────────────


@when("charts/service is rendered for team-ai with its recs service values")
def render_ai(roc):
    if shutil.which("helm") is None:
        pytest.fail("helm is not installed")
    cmd = [
        "helm", "template", "team-ai", "charts/service", "--namespace", "marketplace",
        "-f", "envs/services/team-ai.yaml",
    ]  # fmt: skip
    run = subprocess.run(cmd, cwd=GITOPS, capture_output=True, text=True, timeout=120)
    assert run.returncode == 0, f"helm template failed: {run.stderr[:600]}"
    roc["ai"] = [d for d in yaml.safe_load_all(run.stdout) if d]


def _env(docs) -> dict[str, str]:
    wl = next(d for d in docs if d["kind"] in ("Deployment", "Rollout"))
    return {e["name"]: e.get("value") for e in _pod(wl)["containers"][0].get("env", [])}


def _inline_env() -> dict[str, str]:
    app = yaml.safe_load((GITOPS / "argocd/apps/team-ai.yaml").read_text())
    chart = next(s for s in app["spec"]["sources"] if s.get("path") == "charts/service")
    return {k: str(v) for k, v in yaml.safe_load(chart["helm"]["values"])["env"].items()}


@then(
    "the container env enables the qdrant backend on Service qdrant and the recsys collection, reads "
    "featurestore features from valkey DB 2 and nearline signals from valkey DB 0, and the cache "
    "prefix, schema and Redis host match the recsys job config"
)
def ai_env(roc):
    env = _env(roc["ai"])
    recsys = _one(_docs("platform/recsys/cronjob.yaml"), "ConfigMap", "recsys-config")["data"]
    services = _services()
    assert env["RECS_ENABLED"] == "true" and env["RECS_BACKEND"] == "qdrant"
    q = re.fullmatch(r"http://([a-z0-9-]+):(\d+)", env["RECS_QDRANT_URL"])
    assert q and q[1] == "qdrant" and "qdrant" in services, env["RECS_QDRANT_URL"]
    assert int(q[2]) in {p["port"] for p in services["qdrant"]["spec"]["ports"]}
    assert env["RECS_QDRANT_COLLECTION"] == recsys["QDRANT_ITEM_COLLECTION"]
    for key, db in (("RECS_FEATURESTORE_REDIS_URL", "2"), ("RECS_NEARLINE_REDIS_URL", "0")):
        m = re.fullmatch(r"redis://([a-z0-9-]+):(\d+)/(\d+)", env[key])
        assert m and m[1] == "valkey" and m[3] == db, (key, env[key])
        assert m[1] in services
    fs = _one(_docs("platform/featurestore/featurestore.yaml"), "ConfigMap", "featurestore-config")
    assert env["RECS_FEATURESTORE_REDIS_URL"] == fs["data"]["FEATURESTORE_REDIS_URL"]
    assert env["RECS_CACHE_PREFIX"] == recsys["RECS_CACHE_PREFIX"]
    assert env["RECS_CACHE_SCHEMA_VERSION"] == recsys["RECS_SCHEMA_VERSION"]
    assert env["REDIS_ENABLED"] == "true" and env["REDIS_HOST"] == recsys["REDIS_HOST"] == "valkey"
    # No secret literals: only the recs/redis non-secret keys were added.
    assert not [k for k in env if re.search(r"PASSWORD|TOKEN|SECRET|API_KEY", k) and env[k]]
    # The local cluster renders the inline block of the Application, so it must carry the same.
    inline = _inline_env()
    drift = {
        k: (env[k], inline.get(k))
        for k in env
        if k.startswith(("RECS_", "REDIS_")) and env[k] != inline.get(k)
    }
    assert not drift, f"inline local block differs from envs/services/team-ai.yaml: {drift}"


@then(
    "its NetworkPolicy ingress sources are exactly team-gateway, team-search-indexer and prometheus"
)
def ai_allow_list(roc):
    policies = [d for d in roc["ai"] if d["kind"] == "NetworkPolicy"]
    assert len(policies) == 1
    spec = policies[0]["spec"]
    assert spec["podSelector"]["matchLabels"] == {"app": "team-ai"}
    peers = _peers(spec["ingress"])
    assert all("podSelector" in p for p in peers), peers
    assert {p["podSelector"]["matchLabels"]["app"] for p in peers} == {
        "team-gateway",
        "team-search-indexer",
        "prometheus",
    }


# ── nearline ──────────────────────────────────────────────────────────────────────────────────


@when("the nearline manifest is rendered")
def render_nearline(roc):
    roc["nl"] = _docs("platform/recsys/nearline.yaml")


@then(
    "the Deployment has one replica, runs python -m recsys.nearline from the platform-recsys image, "
    "and its env names the redpanda broker, topic analytics.events and Redis host valkey, which "
    "exist as Services in the cluster"
)
def nearline_wired(roc):
    dep = _one(roc["nl"], "Deployment", "platform-recsys-nearline")
    assert dep["spec"]["replicas"] == 1
    pod = _pod(dep)
    c = pod["containers"][0]
    assert c["command"] == ["python", "-m", "recsys.nearline"]
    assert c["image"].rsplit("/", 1)[-1].split(":")[0] == "platform-recsys"
    env = dict(_one(roc["nl"], "ConfigMap", "recsys-nearline-config")["data"])
    assert c["envFrom"] == [{"configMapRef": {"name": "recsys-nearline-config"}}]
    env.update({e["name"]: e["value"] for e in c.get("env", [])})  # literals only (no secretKeyRef)
    services = _services()
    broker, _, port = env["KAFKA_BROKERS"].partition(":")
    assert broker == "redpanda" and broker in services
    assert int(port) in {p["port"] for p in services["redpanda"]["spec"]["ports"]}
    assert env["KAFKA_ANALYTICS_TOPIC"] == "analytics.events"
    assert env["REDIS_HOST"] == "valkey" and "valkey" in services
    assert env.get("REDIS_DATABASE", "0") == "0"
    assert {"cpu", "memory"} <= set(c["resources"]["requests"]) and c["resources"]["limits"]
    assert pod["securityContext"]["runAsNonRoot"] is True
    assert c["securityContext"]["readOnlyRootFilesystem"] is True
    assert not any("secretKeyRef" in e.get("valueFrom", {}) for e in c.get("env", []))
    assert not [k for k in env if re.search(r"PASSWORD|TOKEN|SECRET", k) and env[k]]


@when("the recsys and nearline NetworkPolicies are rendered")
def render_np(roc):
    roc["rx_np"] = _one(_docs("platform/recsys/cronjob.yaml"), "NetworkPolicy", "platform-recsys")
    roc["nl_np"] = _one(
        _docs("platform/recsys/nearline.yaml"), "NetworkPolicy", "platform-recsys-nearline"
    )


def _egress(np) -> set[tuple[str, tuple[int, ...]]]:
    out = set()
    for rule in np["spec"]["egress"]:
        ports = tuple(sorted(p["port"] for p in rule["ports"]))
        for peer in rule["to"]:
            sel = peer["podSelector"]["matchLabels"]
            if "namespaceSelector" in peer:
                assert sel == {"k8s-app": "kube-dns"} and peer["namespaceSelector"][
                    "matchLabels"
                ] == {"kubernetes.io/metadata.name": "kube-system"}, peer
                out.add(("dns", ports))
            else:
                out.add((sel["app"], ports))
    return out


@then(
    "each allows no ingress, and its egress targets are DNS plus exactly the store pods and ports named above"
)
def egress_limited(roc):
    for key, app, expected in (
        ("rx_np", "platform-recsys", {("qdrant", (6333, 6334)), ("valkey", (6379,))}),
        ("nl_np", "platform-recsys-nearline", {("redpanda", (9092,)), ("valkey", (6379,))}),
    ):
        np = roc[key]
        spec = np["spec"]
        assert spec["podSelector"]["matchLabels"] == {"app": app}
        assert set(spec["policyTypes"]) == {"Ingress", "Egress"}
        assert not spec.get("ingress"), "no ingress rule may be defined"
        assert _egress(np) == {("dns", (53, 53)), *expected}, _egress(np)
    # The recsys CronJob pod really carries the label its policy selects.
    cron = _one(_docs("platform/recsys/cronjob.yaml"), "CronJob", "platform-recsys")
    labels = cron["spec"]["jobTemplate"]["spec"]["template"]["metadata"]["labels"]
    assert labels["app"] == "platform-recsys"


# ── images ────────────────────────────────────────────────────────────────────────────────────


@when("the image push script and the local values are read")
def read_push(roc):
    script = (ROOT / "deploy/images/push-images.sh").read_text()
    block = script.split('MAP="', 1)[1].split('"', 1)[0]
    roc["push"] = dict(line.split() for line in block.splitlines() if line.strip())
    roc["values"] = yaml.safe_load((GITOPS / "envs/local/values.yaml").read_text())


@then(
    "the push list names platform-featurestore and platform-recsys, the local values carry both "
    "image keys, and every localhost:5001 image the recsys and featurestore manifests use is on "
    "the push list"
)
def push_list(roc):
    push, values = roc["push"], roc["values"]
    assert push["platform-featurestore"] == "platform-featurestore:local"
    assert push["platform-recsys"] == "platform-recsys:local"
    assert {"platform-featurestore", "platform-recsys"} <= set(values["images"])
    used = set()
    for rel in (
        "platform/recsys/cronjob.yaml",
        "platform/recsys/nearline.yaml",
        "platform/featurestore/featurestore.yaml",
    ):
        used |= set(re.findall(r"localhost:5001/([a-z0-9-]+):local", (GITOPS / rel).read_text()))
    assert used == {"platform-recsys", "platform-featurestore"}, used
    assert used <= set(push), used - set(push)


# ── ArgoCD ────────────────────────────────────────────────────────────────────────────────────


@when("the manifest paths and the Applications are read")
def read_apps(roc):
    roc["paths"] = _synced_paths()


@then(
    "an Application syncs the directory holding the Qdrant manifest and one syncs the directory "
    "holding the nearline manifest"
)
def apps_sync(roc):
    for rel in (f"{INFRA}/qdrant.yaml", "platform/recsys/nearline.yaml"):
        assert (GITOPS / rel).is_file()
        parent = str(Path(rel).parent)
        syncing = [
            a["metadata"]["name"]
            for a in _apps()
            if parent
            in {s.get("path") for s in [a["spec"].get("source", {}), *a["spec"].get("sources", [])]}
            and a["spec"].get("source", {}).get("directory", {}).get("recurse")
        ]
        assert syncing, f"no Application syncs {parent}: {sorted(roc['paths'])}"
