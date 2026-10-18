"""Manifest checks for change frontend-cookie-secure, task 1.6.

Run: python -m pytest -q platform-gitops/tests/test_frontend_cookie_secure_manifests.py
"""
from pathlib import Path

import pytest
import yaml

ROOT = Path(__file__).resolve().parent.parent
REPO = ROOT.parent
BASE = ROOT / "envs/services/team-frontend.yaml"


def load(path: Path) -> dict:
    assert path.exists(), f"{path} does not exist"
    return yaml.safe_load(path.read_text()) or {}


def merged_env(env: str) -> dict:
    """Helm-style map merge: overlay env keys layered over the base env."""
    base = load(BASE).get("env") or {}
    overlay = load(ROOT / f"envs/{env}/services/team-frontend.yaml").get("env") or {}
    return {**base, **overlay}


@pytest.mark.parametrize("env", ["staging", "prod"])
def test_scenario_cookie_secure_staging_and_prod_set_it(env):
    overlay = load(ROOT / f"envs/{env}/services/team-frontend.yaml")
    # Must be the string "true" (a YAML bool would render as a different env value type).
    assert overlay.get("env", {}).get("COOKIE_SECURE") == "true"


@pytest.mark.parametrize("env", ["staging", "prod"])
def test_scenario_cookie_secure_overlay_keeps_base_env_merged(env):
    merged = merged_env(env)
    base = load(BASE)["env"]
    assert merged["COOKIE_SECURE"] == "true"
    for k, v in base.items():
        assert merged[k] == v, f"base env {k} lost or changed by the {env} overlay"


@pytest.mark.parametrize("env", ["staging", "prod"])
def test_scenario_cookie_secure_overlay_keeps_existing_settings(env):
    overlay = load(ROOT / f"envs/{env}/services/team-frontend.yaml")
    assert overlay["ingress"]["enabled"] is True
    assert overlay["ingress"]["host"]


def test_scenario_cookie_secure_local_compose_leaves_it_unset_boundary():
    assert "COOKIE_SECURE" not in (load(BASE).get("env") or {})
    assert "COOKIE_SECURE" not in (ROOT / "envs/local/values.yaml").read_text()
    for name in ("docker-compose.services.yaml", "platform-e2e/docker-compose.e2e.yaml"):
        p = REPO / name
        if p.exists():
            assert "COOKIE_SECURE" not in p.read_text(), name
