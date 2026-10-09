"""Steps for modelserve/model_serving_gitops.feature: static checks of the GitOps manifest."""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml
from pytest_bdd import then, when

MANIFEST = (
    Path(__file__).resolve().parents[4]
    / "platform-gitops"
    / "platform"
    / "infra"
    / "modelserve.yaml"
)


@pytest.fixture
def gitops_ms() -> dict:
    return {}


@when("the platform-modelserve GitOps manifest is parsed")
def parse_manifest(gitops_ms: dict) -> None:
    assert MANIFEST.exists(), f"{MANIFEST} is missing"
    gitops_ms["docs"] = [d for d in yaml.safe_load_all(MANIFEST.read_text("utf-8")) if d]


@then(
    "a NetworkPolicy selects app modelserve-router and its only ingress sources are app team-ai, "
    "app team-search, app team-search-indexer and app prometheus on port 8100"
)
def network_policy(gitops_ms: dict) -> None:
    policies = [
        d
        for d in gitops_ms["docs"]
        if d["kind"] == "NetworkPolicy"
        and d["spec"]["podSelector"].get("matchLabels") == {"app": "modelserve-router"}
    ]
    assert (
        len(policies) == 1
    ), f"expected one policy selecting app modelserve-router, got {len(policies)}"
    spec = policies[0]["spec"]
    assert "Ingress" in spec["policyTypes"]
    rules = spec["ingress"]
    sources = {peer["podSelector"]["matchLabels"]["app"] for rule in rules for peer in rule["from"]}
    assert sources == {
        "team-ai",
        "team-search",
        "team-search-indexer",
        "prometheus",
    }, f"ingress sources: {sources}"
    assert all(
        "namespaceSelector" not in p and "ipBlock" not in p for r in rules for p in r["from"]
    )
    ports = {(p["protocol"], p["port"]) for rule in rules for p in rule["ports"]}
    assert ports == {("TCP", 8100)}, f"ingress ports: {ports}"
