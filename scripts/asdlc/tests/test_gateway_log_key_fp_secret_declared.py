"""Task 1.6 of gateway-principal-log-fingerprint: GW-FP-12a, GW-FP-12b, GW-FP-12c."""

from __future__ import annotations

import re
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[3]
KEY = "LOG_KEY_FP_SECRET"
COMPOSE = ROOT / "docker-compose.services.yaml"
ENV_EXAMPLE = ROOT / "team-gateway" / ".env.example"
README = ROOT / "team-gateway" / "README.md"
VAULT = ROOT / "platform-gitops" / "platform" / "vault-config" / "vault-config.yaml"
ENV_VALUES = ROOT / "platform-gitops" / "envs" / "services" / "team-gateway.yaml"
APP = ROOT / "platform-gitops" / "argocd" / "apps" / "team-gateway.yaml"


def _compose_gateway_env() -> dict:
    env = yaml.safe_load(COMPOSE.read_text())["services"]["team-gateway"]["environment"]
    if isinstance(env, dict):
        return {k: "" if v is None else str(v) for k, v in env.items()}
    out = {}
    for item in env:
        k, _, v = item.partition("=")
        out[k] = v
    return out


def _example_env() -> dict:
    out = {}
    for line in ENV_EXAMPLE.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, _, v = line.partition("=")
            out[k.strip()] = v.strip().strip("\"'")
    return out


def test_GW_FP_12a_compose_declares_key_with_non_empty_local_value():
    assert _compose_gateway_env().get(KEY, "").strip() != ""


def test_GW_FP_12a_env_example_declares_key_with_non_empty_local_value():
    assert _example_env().get(KEY, "").strip() != ""


def test_GW_FP_12b_readme_env_table_has_key_row():
    rows = [ln for ln in README.read_text().splitlines() if ln.lstrip().startswith("|") and f"`{KEY}`" in ln]
    assert rows


def test_GW_FP_12c_vault_seeds_key_into_gateway_bag():
    text = VAULT.read_text()
    m = re.search(r'SERVICES:\s*"([^"]*)"', text)
    assert m and "team-gateway" in m.group(1).split()
    assert f'-field={KEY} "svc/${{svc}}/config"' in text
    assert re.search(rf'{KEY}="\$log_key"', text)


def test_GW_FP_12c_gitops_secrets_enabled_for_gateway():
    assert yaml.safe_load(ENV_VALUES.read_text())["secrets"]["enabled"] is True
    app = yaml.safe_load(APP.read_text())
    values = yaml.safe_load(app["spec"]["sources"][1]["helm"]["values"])
    assert values["secrets"]["enabled"] is True


def test_GW_FP_12c_no_literal_secret_in_gitops_gateway_files():
    for path in (VAULT, ENV_VALUES, APP):
        for ln in path.read_text().splitlines():
            if KEY not in ln:
                continue
            m = re.search(rf'{KEY}["\']?\s*[:=]\s*(\S+)', ln)
            if m:
                assert m.group(1).strip("\"'").startswith("$"), f"{path.name}: literal {KEY}"
    for path in (ENV_VALUES, APP):
        assert f"{KEY}:" not in path.read_text()
