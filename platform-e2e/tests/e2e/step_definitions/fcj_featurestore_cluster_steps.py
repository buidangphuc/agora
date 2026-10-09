"""Static assertions on platform-gitops raw manifests (featurestore-cluster-job). No cluster."""

from __future__ import annotations

import os
import re
import subprocess
import tempfile
import time
from pathlib import Path

import pytest
import yaml
from pytest_bdd import then, when

GITOPS = Path(__file__).resolve().parents[3].parent / "platform-gitops"


@pytest.fixture
def fcj():
    return {}


def _docs(rel: str) -> list[dict]:
    return [d for d in yaml.safe_load_all((GITOPS / rel).read_text()) if d]


def _one(docs, kind, name):
    return next(d for d in docs if d["kind"] == kind and d["metadata"]["name"] == name)


def _pod(cron):
    return cron["spec"]["jobTemplate"]["spec"]["template"]["spec"]


def _config(docs, name):
    return _one(docs, "ConfigMap", name)["data"]


def _minutes(schedule: str) -> int:
    m, h = schedule.split()[:2]
    return int(h) * 60 + int(m)


@when("the featurestore CronJob manifest is rendered")
def render_fs(fcj):
    fcj["fs"] = _docs("platform/featurestore/featurestore.yaml")


@then(
    "its init containers run materialize then parity, its main container runs dataset, and "
    "concurrencyPolicy is Forbid"
)
def steps_in_order(fcj):
    cron = _one(fcj["fs"], "CronJob", "featurestore")
    pod = _pod(cron)
    assert [c["args"] for c in pod["initContainers"]] == [["materialize"], ["parity"]]
    assert [c["args"] for c in pod["containers"]] == [["dataset"]]
    images = {
        c["image"].rsplit("/", 1)[-1].split(":")[0]
        for c in pod["initContainers"] + pod["containers"]
    }
    assert images == {"platform-featurestore"}, images
    assert cron["spec"]["concurrencyPolicy"] == "Forbid"


@when("the featurestore and recsys CronJobs are rendered")
def render_both(fcj):
    fcj["fs"] = _docs("platform/featurestore/featurestore.yaml")
    fcj["rx"] = _docs("platform/recsys/cronjob.yaml")


@then(
    "the featurestore schedule fires earlier in the day than recsys and an Application syncs "
    "platform/featurestore"
)
def scheduled_before(fcj):
    fs = _one(fcj["fs"], "CronJob", "featurestore")["spec"]["schedule"]
    rx = _one(fcj["rx"], "CronJob", "platform-recsys")["spec"]["schedule"]
    assert _minutes(fs) < _minutes(rx), (fs, rx)
    assert _config(fcj["fs"], "featurestore-config")["SCHEDULE"] == fs
    app = _docs("argocd/apps/featurestore.yaml")[0]
    assert app["kind"] == "Application" and app["spec"]["source"]["path"] == "platform/featurestore"


def _mounts(pod):
    return {
        m["name"]: m
        for c in pod["containers"] + pod.get("initContainers", [])
        for m in c["volumeMounts"]
    }


@when("the featurestore volume manifests are rendered")
def render_vol(fcj):
    render_both(fcj)


@then(
    "the featurestore writes claim featurestore-data and recsys mounts it read-only under "
    "DATASET_DIR with no emptyDir"
)
def shared_volume(fcj):
    fs_pod = _pod(_one(fcj["fs"], "CronJob", "featurestore"))
    rx_pod = _pod(_one(fcj["rx"], "CronJob", "platform-recsys"))
    pvc = _one(fcj["fs"], "PersistentVolumeClaim", "featurestore-data")
    assert pvc
    fs_vol = {v["name"]: v for v in fs_pod["volumes"]}["featurestore-data"]
    assert fs_vol["persistentVolumeClaim"]["claimName"] == "featurestore-data"
    for c in fs_pod["containers"]:
        m = next(m for m in c["volumeMounts"] if m["name"] == "featurestore-data")
        assert m["mountPath"] == "/features" and not m.get("readOnly")
    rx_vols = {v["name"]: v for v in rx_pod["volumes"]}
    assert rx_vols["featurestore-data"]["persistentVolumeClaim"]["claimName"] == "featurestore-data"
    assert not any("emptyDir" in v for v in rx_pod["volumes"]), rx_pod["volumes"]
    dataset_dir = _config(fcj["rx"], "recsys-config")["DATASET_DIR"]
    for name, mount in _mounts(rx_pod).items():
        assert mount.get("readOnly") is True, name
        assert dataset_dir.startswith(mount["mountPath"].rstrip("/") + "/")
    assert _config(fcj["fs"], "featurestore-config")["FEATURESTORE_OFFLINE_DIR"] == "/features"


@when("the analytics export manifests are rendered")
def render_analytics(fcj):
    render_both(fcj)
    fcj["an"] = _docs("platform/team-analytics/consumer.yaml")


@then(
    "team-analytics exports to claim analytics-data and the featurestore mounts it read-only at its input dir"
)
def analytics_export(fcj):
    dep = _one(fcj["an"], "Deployment", "team-analytics")
    pod = dep["spec"]["template"]["spec"]
    env = {e["name"]: e["value"] for e in pod["containers"][0]["env"]}
    mount = next(m for m in pod["containers"][0]["volumeMounts"] if m["name"] == "analytics-data")
    assert env["PARQUET_EXPORT_PATH"].startswith(mount["mountPath"].rstrip("/") + "/")
    vol = next(v for v in pod["volumes"] if v["name"] == "analytics-data")
    assert vol["persistentVolumeClaim"]["claimName"] == "analytics-data"
    assert _one(fcj["an"], "PersistentVolumeClaim", "analytics-data")
    fs_pod = _pod(_one(fcj["fs"], "CronJob", "featurestore"))
    input_dir = _config(fcj["fs"], "featurestore-config")["FEATURESTORE_INPUT_DIR"]
    for c in fs_pod["initContainers"] + fs_pod["containers"]:
        m = next(m for m in c["volumeMounts"] if m["name"] == "analytics-data")
        assert m["mountPath"] == input_dir and m["readOnly"] is True
    fvol = next(v for v in fs_pod["volumes"] if v["name"] == "analytics-data")
    assert fvol["persistentVolumeClaim"]["claimName"] == "analytics-data"


def _guard(fcj):
    rx = _docs("platform/recsys/cronjob.yaml")
    cfg = _config(rx, "recsys-config")
    init = _pod(_one(rx, "CronJob", "platform-recsys"))["initContainers"][0]
    assert init["command"][:2] == ["sh", "-c"]
    return init["command"][2], cfg


def _run_guard(script: str, cfg: dict, dataset_dir: str):
    env = {"PATH": os.environ["PATH"], **cfg, "DATASET_DIR": dataset_dir}
    return subprocess.run(["sh", "-c", script], env=env, capture_output=True, text=True, timeout=30)


@when("the recsys freshness guard runs against a missing directory and an empty one")
def guard_absent(fcj):
    script, cfg = _guard(fcj)
    with tempfile.TemporaryDirectory() as tmp:
        empty = Path(tmp) / "empty"
        empty.mkdir()
        fcj["runs"] = [
            _run_guard(script, cfg, str(Path(tmp) / "missing")),
            _run_guard(script, cfg, str(empty)),
        ]


@then("it exits non-zero both times")
def nonzero_both(fcj):
    assert all(r.returncode != 0 for r in fcj["runs"]), [r.returncode for r in fcj["runs"]]


def _snapshot(tmp: str, age_minutes: int) -> str:
    d = Path(tmp) / "als_interactions" / "v1"
    d.mkdir(parents=True)
    f = d / "as_of=20260101T000000Z.parquet"
    f.write_bytes(b"x")
    t = time.time() - age_minutes * 60
    os.utime(f, (t, t))
    return str(d)


@when("the recsys freshness guard runs against a directory with only an old snapshot")
def guard_stale(fcj):
    script, cfg = _guard(fcj)
    with tempfile.TemporaryDirectory() as tmp:
        d = _snapshot(tmp, int(cfg["MAX_DATASET_AGE_MINUTES"]) + 60)
        fcj["runs"] = [_run_guard(script, cfg, d)]


@then("it exits non-zero")
def nonzero(fcj):
    assert fcj["runs"][0].returncode != 0


@when("the recsys freshness guard runs against a directory with a new snapshot")
def guard_fresh(fcj):
    script, cfg = _guard(fcj)
    with tempfile.TemporaryDirectory() as tmp:
        d = _snapshot(tmp, 5)
        fcj["runs"] = [_run_guard(script, cfg, d)]


@then("it exits zero")
def zero(fcj):
    r = fcj["runs"][0]
    assert r.returncode == 0, r.stderr


@when("the featurestore security manifests are rendered")
def render_sec(fcj):
    render_fs(fcj)


@then(
    "every container is locked down with resources, the NetworkPolicy allows no ingress and only DNS "
    "and valkey egress, and no secret is in Git"
)
def hardened(fcj):
    docs = fcj["fs"]
    pod = _pod(_one(docs, "CronJob", "featurestore"))
    assert pod["securityContext"]["runAsNonRoot"] is True
    for c in pod["initContainers"] + pod["containers"]:
        sc = c["securityContext"]
        assert sc["allowPrivilegeEscalation"] is False and sc["readOnlyRootFilesystem"] is True
        assert sc["capabilities"]["drop"] == ["ALL"]
        for k in ("requests", "limits"):
            assert {"cpu", "memory"} <= set(c["resources"][k]), c["name"]
    np = _one(docs, "NetworkPolicy", "featurestore")["spec"]
    assert np["podSelector"]["matchLabels"] == {"app": "featurestore"}
    assert "Ingress" in np["policyTypes"] and not np.get("ingress")
    peers = [p for rule in np["egress"] for p in rule["to"]]
    kinds = {
        (
            "dns"
            if p.get("podSelector", {}).get("matchLabels", {}).get("k8s-app") == "kube-dns"
            else p.get("podSelector", {}).get("matchLabels", {}).get("app")
        )
        for p in peers
    }
    assert kinds == {"dns", "valkey"}, kinds
    assert not [d for d in docs if d["kind"] == "Secret"]
    cfg = _config(docs, "featurestore-config")
    bad = [
        k
        for k, v in cfg.items()
        if re.search(r"(?i)password|secret|token|key", k) or re.search(r"://[^/@\s]+:[^/@\s]+@", v)
    ]
    assert not bad, bad
