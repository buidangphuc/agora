"""Static checks for recsys-auto-train task 1.13 and the monorepo-root build context.

Scenarios RAT-33 (compose side: the same three variables) and RAT-34 (compose wiring of the nightly chain).
The Dockerfile COPYs platform-recsys/... and platform-core/packages/py-modelregistry/..., so both recsys
services build from the repo root and a root .dockerignore keeps that context small.
"""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

ROOT = Path(__file__).resolve().parents[2]
COMPOSE = ROOT / "docker-compose.services.yaml"
DOCKERIGNORE = ROOT / ".dockerignore"


@pytest.fixture(scope="module")
def services() -> dict:
    return yaml.safe_load(COMPOSE.read_text())["services"]


def _all_services() -> dict:
    """Services of docker-compose.yaml merged with every file it `include`s."""
    top = yaml.safe_load((ROOT / "docker-compose.yaml").read_text())
    merged = dict(top.get("services") or {})
    for entry in top.get("include", []):
        path = entry if isinstance(entry, str) else entry["path"]
        merged.update(yaml.safe_load((ROOT / path).read_text()).get("services") or {})
    return merged


def _env(svc: dict) -> dict:
    env = svc.get("environment", {})
    if isinstance(env, list):
        return dict(e.split("=", 1) for e in env)
    return {k: str(v) for k, v in env.items()}


def _depends(svc: dict) -> set:
    dep = svc.get("depends_on", [])
    return set(dep) if isinstance(dep, (list, dict)) else {dep}


def test_rat33_compose_job_sets_chain_env(services):
    # RAT-33 compose side
    env = _env(services["platform-recsys"])
    assert env.get("MLFLOW_TRACKING_URI") == "http://mlflow:5050"
    assert env.get("ENABLE_GBDT") == "true"
    assert env.get("ENABLE_TWO_TOWER") == "true"


def test_rat34_compose_job_waits_for_mlflow(services):
    # RAT-34 compose side: the registry exists as a compose service and the job depends on it.
    assert "mlflow" in _all_services()
    assert "mlflow" in _depends(services["platform-recsys"])


def test_rat34_compose_job_points_at_snapshot_dirs(services):
    # RAT-34: GBDT and two-tower read the featurestore snapshots from the read-only /features volume.
    svc = services["platform-recsys"]
    env = _env(svc)
    for key in (
        "ITEM_FEATURES_DIR",
        "USER_FEATURES_DIR",
        "ITEM_ATTRIBUTES_DIR",
        "USER_PREFERENCES_DIR",
        "RANK_DATASET_DIR",
    ):
        assert env.get(key, "").startswith("/features/"), key
    assert "featurestore_data:/features:ro" in svc["volumes"]


@pytest.mark.parametrize("name", ["platform-recsys", "platform-recsys-nearline"])
def test_build_context_is_monorepo_root(services, name):
    # Build-context follow-up: the Dockerfile needs platform-core/ and platform-recsys/ from the root.
    build = services[name]["build"]
    assert build["context"] == "."
    assert build["dockerfile"] == "platform-recsys/Dockerfile"


def _ignore_lines() -> list:
    assert DOCKERIGNORE.is_file(), "root .dockerignore is missing"
    return [
        ln.strip() for ln in DOCKERIGNORE.read_text().splitlines() if ln.strip() and not ln.startswith("#")
    ]


def test_dockerignore_excludes_everything_then_allows_build_inputs():
    lines = _ignore_lines()
    assert lines[0] in ("*", "**"), "ignore everything first, then allow-list"
    allowed = {ln[1:].rstrip("/") for ln in lines if ln.startswith("!")}
    assert "platform-recsys" in allowed
    assert any(a.startswith("platform-core") for a in allowed)
    assert any(a.endswith("py-modelregistry") for a in allowed) or "platform-core" in allowed


def test_dockerignore_keeps_context_small():
    # Caches, venvs and git data must not ride along inside the allowed directories.
    joined = "\n".join(_ignore_lines())
    assert "__pycache__" in joined
    assert "venv" in joined
