"""Static assertions on the local cluster image wiring (local-cluster-image-wiring). No cluster, no docker:
`kubectl kustomize` renders, YAML parsing, and text checks on the deploy scripts."""

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
REG = "localhost:5001"
RAW_APPS = ("recsys", "featurestore", "team-analytics", "search-indexer", "modelserve")


@pytest.fixture
def lcw():
    return {}


def _kustomization(app: str) -> dict:
    return yaml.safe_load((GITOPS / "platform" / app / "kustomization.yaml").read_text())


def _push_map() -> list[tuple[str, str, str]]:
    rows = (ROOT / "deploy/images/services.map").read_text().splitlines()
    return [
        tuple(ln.split()) for ln in rows if ln.strip() and not ln.lstrip().startswith("#")
    ]  # key, repo, compose image


def _app_for(path: str) -> dict:
    for p in sorted((GITOPS / "argocd/apps").glob("*.yaml")):
        app = yaml.safe_load(p.read_text())
        if app["spec"].get("source", {}).get("path") == path:
            return app
    raise AssertionError(f"no Application syncs {path}")


# ── hardcoded tags ────────────────────────────────────────────────────────────────────────────


@when("the manifests of the raw-manifest apps are rendered with kustomize")
def render_raw(lcw):
    if shutil.which("kubectl") is None:
        pytest.fail("kubectl is not installed")
    lcw["rendered"] = {}
    for app in RAW_APPS:
        run = subprocess.run(
            ["kubectl", "kustomize", f"platform/{app}"],
            cwd=GITOPS, capture_output=True, text=True, timeout=120,
        )  # fmt: skip
        assert run.returncode == 0, f"kustomize {app} failed: {run.stderr[:400]}"
        lcw["rendered"][app] = run.stdout


@then(
    "no manifest file carries a localhost:5001 image with a tag, and every rendered image is "
    "localhost:5001/<repo> with the newTag of its kustomization"
)
def no_hardcoded_tag(lcw):
    for app in RAW_APPS:
        for f in (GITOPS / "platform" / app).glob("*.yaml"):
            if f.name == "kustomization.yaml":
                continue
            tagged = re.findall(rf"{re.escape(REG)}/[a-z0-9-]+:\S+", f.read_text())
            assert not tagged, f"{f.relative_to(GITOPS)} hardcodes {tagged}"
        tags = {e["name"]: e["newTag"] for e in _kustomization(app)["images"]}
        rendered = set(re.findall(rf"image: ({re.escape(REG)}/\S+)", lcw["rendered"][app]))
        assert rendered, f"{app} renders no {REG} image"
        for image in rendered:
            name, _, tag = image.rpartition(":")
            assert tags.get(name) == tag, (app, image, tags)


# ── kustomize tags vs values ──────────────────────────────────────────────────────────────────


@when("the local values and the kustomizations of the raw-manifest apps are read")
def read_values(lcw):
    lcw["values"] = yaml.safe_load((GITOPS / "envs/local/values.yaml").read_text())["images"]
    lcw["repo_to_key"] = {repo: key for key, repo, _ in _push_map()}


@then(
    "each kustomization image tag equals the images value of its image key, and their Applications set no directory"
)
def tags_follow_values(lcw):
    for app in RAW_APPS:
        for entry in _kustomization(app)["images"]:
            repo = entry["name"].removeprefix(f"{REG}/")
            key = lcw["repo_to_key"][repo]
            assert entry["newTag"] == str(lcw["values"][key]), (app, entry, lcw["values"][key])
        a = _app_for(f"platform/{app}")
        assert (
            "directory" not in a["spec"]["source"]
        ), f"{a['metadata']['name']} forces directory mode"


# ── push list vs compose ──────────────────────────────────────────────────────────────────────


@when("the push script, the compose files and the local values are read")
def read_push(lcw):
    lcw["push"] = _push_map()
    lcw["compose_images"] = set()
    for name in (
        "docker-compose.services.yaml",
        "docker-compose.yaml",
        "platform-core/infra/docker-compose.yaml",
    ):
        doc = yaml.safe_load((ROOT / name).read_text()) or {}
        lcw["compose_images"] |= {
            s["image"] for s in doc.get("services", {}).values() if "image" in s
        }
    lcw["values"] = yaml.safe_load((GITOPS / "envs/local/values.yaml").read_text())["images"]
    script = (ROOT / "deploy/images/push-images.sh").read_text()
    lcw["prefixes"] = re.search(r'SRC_PREFIXES="\$\{SRC_PREFIXES:-([^}]*)\}"', script)[1].split()


@then(
    "every source image of the push list is an image of the compose files, every values image key is on "
    "the list, and every repo a gitops manifest pulls from localhost:5001 is on the list"
)
def push_matches_compose(lcw):
    push = lcw["push"]
    missing = [src for _, _, src in push if src not in lcw["compose_images"]]
    assert not missing, f"push list sources not declared by compose: {missing}"
    assert {k for k, _, _ in push} == set(lcw["values"]), set(lcw["values"]) ^ {
        k for k, _, _ in push
    }
    assert "agora-" in lcw["prefixes"], lcw["prefixes"]  # the isolation override's image names
    assert "-" in lcw["prefixes"], lcw["prefixes"]  # "-" = the bare compose name (no prefix)
    repos = {repo for _, repo, _ in push}
    pulled = set()
    for f in (GITOPS / "platform").rglob("*.yaml"):
        pulled |= set(re.findall(rf"{re.escape(REG)}/([a-z0-9-]+)", f.read_text()))
    for f in (GITOPS / "argocd/apps").glob("*.yaml"):
        pulled |= set(re.findall(rf"repo: {re.escape(REG)}/([a-z0-9-]+)", f.read_text()))
    assert pulled <= repos, f"pulled but not pushed: {pulled - repos}"


# ── gitops-push ───────────────────────────────────────────────────────────────────────────────


@when("the Makefile target, the bootstrap script and the publish script are read")
def read_publish(lcw):
    mk = (ROOT / "deploy/Makefile").read_text()
    lcw["target"] = mk.split("gitops-push:", 1)[1].split("\n\n", 1)[0]
    lcw["bootstrap"] = (ROOT / "deploy/gitea/bootstrap-gitea.sh").read_text()
    lcw["publish"] = (ROOT / "deploy/gitea/publish-gitops.sh").read_text()


def _code(text: str) -> str:
    return "\n".join(ln for ln in text.splitlines() if not ln.lstrip().startswith("#"))


@then(
    "none runs git add, git commit or git push origin inside the agora checkout, the target calls "
    "publish-gitops.sh, and the publish script pushes to a Gitea remote from a temporary clone"
)
def no_origin(lcw):
    bad = re.compile(
        r"git\b[^\n]*\b(add|commit)\b|git\b[^\n]*\bpush\b[^\n]*\borigin\b|\bgit\s+init\b"
    )
    for name in ("target", "bootstrap"):
        code = _code(lcw[name])
        assert not bad.search(code), f"{name} still runs git against the agora checkout"
    assert "publish-gitops.sh" in lcw["target"] and "publish-gitops.sh" in lcw["bootstrap"]
    pub = _code(lcw["publish"])
    assert "mktemp -d" in pub and "trap " in pub and "git clone" in pub
    assert "git add -A" in pub and 'cd "${WORK}/repo"' in pub
    assert "gitea.localtest.me" in pub and "github.com" not in pub
    assert re.search(r"git push -q \"\$\{REMOTE_NAME\}\" HEAD:main", pub)
    assert 'git -C "${ROOT}" add' not in pub and 'git -C "${ROOT}" commit' not in pub


@when("the Deployments of platform-gitops that mount a ReadWriteOnce claim are read")
def read_rwo_deployments(lcw):
    claims, deployments = set(), []
    for path in sorted((GITOPS / "platform").rglob("*.yaml")):
        for doc in yaml.safe_load_all(path.read_text()):
            if not isinstance(doc, dict):
                continue
            if doc.get("kind") == "PersistentVolumeClaim" and "ReadWriteOnce" in (
                doc.get("spec", {}).get("accessModes") or []
            ):
                claims.add(doc["metadata"]["name"])
            elif doc.get("kind") == "Deployment":
                deployments.append((path.relative_to(GITOPS), doc))
    lcw["rwo"] = [
        (path, doc)
        for path, doc in deployments
        if any(
            (v.get("persistentVolumeClaim") or {}).get("claimName") in claims
            for v in doc["spec"]["template"]["spec"].get("volumes") or []
        )
    ]
    assert lcw["rwo"], "no Deployment mounts a ReadWriteOnce claim"


@then(
    "each one with a single replica uses the Recreate strategy, so a rollout never runs two pods on one volume"
)
def rwo_recreate(lcw):
    wrong = [
        str(path)
        for path, doc in lcw["rwo"]
        if doc["spec"].get("replicas", 1) == 1
        and (doc["spec"].get("strategy") or {}).get("type") != "Recreate"
    ]
    assert not wrong, wrong


@when("the Redpanda manifest of platform-gitops is read")
def read_redpanda(lcw):
    path = GITOPS / "platform" / "infra" / "redpanda.yaml"
    lcw["redpanda"] = [d for d in yaml.safe_load_all(path.read_text()) if isinstance(d, dict)]


@then(
    "the redpanda container mounts a volume at `/var/lib/redpanda/data` that is backed by a "
    "`PersistentVolumeClaim` declared in the same manifest, and the pod's `fsGroup` lets the "
    "`redpanda` user (gid 101) write it"
)
def redpanda_on_claim(lcw):
    docs = lcw["redpanda"]
    claims = {d["metadata"]["name"] for d in docs if d.get("kind") == "PersistentVolumeClaim"}
    deploy = next(
        d for d in docs if d.get("kind") == "Deployment" and d["metadata"]["name"] == "redpanda"
    )
    pod = deploy["spec"]["template"]["spec"]
    container = next(c for c in pod["containers"] if c["name"] == "redpanda")
    mount = next(
        (
            m
            for m in container.get("volumeMounts") or []
            if m["mountPath"] == "/var/lib/redpanda/data"
        ),
        None,
    )
    assert mount, "redpanda container has no volumeMount at /var/lib/redpanda/data"
    volume = next(v for v in pod.get("volumes") or [] if v["name"] == mount["name"])
    claim = (volume.get("persistentVolumeClaim") or {}).get("claimName")
    assert (
        claim in claims
    ), f"volume {volume['name']} claim {claim!r} not declared in {sorted(claims)}"
    assert (pod.get("securityContext") or {}).get("fsGroup") == 101


# ── modelserve images ─────────────────────────────────────────────────────────────────────────


@when(
    "the modelserve app of platform-gitops is rendered with kustomize and the push script is read"
)
def render_modelserve(lcw):
    if shutil.which("kubectl") is None:
        pytest.fail("kubectl is not installed")
    run = subprocess.run(
        ["kubectl", "kustomize", "platform/modelserve"],
        cwd=GITOPS, capture_output=True, text=True, timeout=120,
    )  # fmt: skip
    assert run.returncode == 0, f"kustomize modelserve failed: {run.stderr[:400]}"
    lcw["ms_docs"] = [d for d in yaml.safe_load_all(run.stdout) if isinstance(d, dict)]
    lcw["push"] = _push_map()


@then(
    "every container image of modelserve-router, tei-embed and tei-rerank is a localhost:5001 repo that "
    "the push list ships, tei-embed and tei-rerank run tei-fake, and no image is pulled from ghcr.io or "
    "carries the `agora/` prefix"
)
def modelserve_images_local(lcw):
    repos = {repo for _, repo, _ in lcw["push"]}
    deployments = {
        d["metadata"]["name"]: d for d in lcw["ms_docs"] if d.get("kind") == "Deployment"
    }
    for name in ("modelserve-router", "tei-embed", "tei-rerank"):
        assert name in deployments, f"{name} is not rendered, got {sorted(deployments)}"
        spec = deployments[name]["spec"]["template"]["spec"]
        containers = (spec.get("containers") or []) + (spec.get("initContainers") or [])
        assert containers, name
        for c in containers:
            image = c["image"]
            assert image.startswith(f"{REG}/"), f"{name}: {image} is not from {REG}"
            repo = image.removeprefix(f"{REG}/").rpartition(":")[0]
            assert repo in repos, f"{name}: {repo} is not on the push list {sorted(repos)}"
            if name.startswith("tei-"):
                assert repo == "tei-fake", f"{name}: runs {repo}, not tei-fake"
            assert "ghcr.io" not in image and "agora/" not in image, (name, image)


# ── one owner per path ────────────────────────────────────────────────────────────────────────


@when("the Applications of platform-gitops are read")
def read_applications(lcw):
    lcw["apps"] = []
    for p in sorted((GITOPS / "argocd/apps").glob("*.yaml")):
        for doc in yaml.safe_load_all(p.read_text()):
            if not isinstance(doc, dict) or doc.get("kind") != "Application":
                continue
            source = (doc.get("spec") or {}).get("source") or {}
            if source.get("path") is None:
                continue
            lcw["apps"].append(
                (
                    doc["metadata"]["name"],
                    str(source["path"]).strip("/"),
                    bool((source.get("directory") or {}).get("recurse")),
                )
            )
    assert lcw["apps"], "no Application with a source path"


@then(
    "their source paths are pairwise distinct, and no path is a parent of another app's path when "
    "the parent app recurses into directories"
)
def one_owner_per_path(lcw):
    apps = lcw["apps"]
    seen: dict[str, str] = {}
    for name, path, _ in apps:
        assert path not in seen, f"{name} and {seen[path]} both sync {path}"
        seen[path] = name
    for name, path, recurse in apps:
        if not recurse:
            continue
        nested = [o for o, op, _ in apps if o != name and op.startswith(path + "/")]
        assert not nested, f"{name} recurses into {path}, which holds {nested}"


def _pod_specs(node):
    """Every pod spec in a manifest: Deployment/StatefulSet/Job templates and CronJob job templates."""
    if isinstance(node, dict):
        if "containers" in node and isinstance(node["containers"], list):
            yield node
        for value in node.values():
            yield from _pod_specs(value)
    elif isinstance(node, list):
        for value in node:
            yield from _pod_specs(value)


@when("every manifest under platform-gitops/platform is read")
def read_all_images(lcw):
    images = []
    for path in sorted((GITOPS / "platform").rglob("*.yaml")):
        for doc in yaml.safe_load_all(path.read_text()):
            for pod in _pod_specs(doc):
                for c in (pod.get("initContainers") or []) + pod["containers"]:
                    images.append((str(path.relative_to(GITOPS)), c["image"]))
    assert images, "no container images found under platform-gitops/platform"
    lcw["images"] = images


@then(
    "each container and init container image outside localhost:5001 has a tag, the tag is not "
    "`latest`, and no image is `minio/mc`"
)
def third_party_pinned(lcw):
    wrong = []
    for path, image in lcw["images"]:
        if image.startswith("localhost:5001/"):
            continue
        name, _, tag = image.rpartition(":") if ":" in image.split("/")[-1] else (image, "", "")
        if not tag or tag == "latest" or name.removeprefix("docker.io/") == "minio/mc":
            wrong.append(f"{path}: {image}")
    assert not wrong, wrong
