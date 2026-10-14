"""Scenarios: the finetune policy is declared in the shared policy file (platform-finetune spec)."""

import pytest
import yaml
from modelregistry.policy import load_policy

from datascience.constants import paths

ENTRIES = ("recsys-als", "recsys-gbdt", "recsys-two-tower", "finetune-classifier")


@pytest.fixture(autouse=True)
def _packaged_policies(monkeypatch):
    monkeypatch.delenv("MODELREGISTRY_POLICIES", raising=False)


def test_finetune_policy_values():
    # Scenario: The entry has the agreed values
    p = load_policy("finetune-classifier")
    assert p.primary == "macro_f1"
    assert p.min_relative_improvement == 0.01
    assert p.floors == {"macro_f1": 0.5}
    assert p.guards == {}


def test_policies_file_loads_all_entries():
    # Scenario: The policy file loads with every entry
    for name in ENTRIES:
        load_policy(name)  # raises PolicyError when missing or malformed


def test_params_has_no_gate_switches():
    # Scenario: The gate settings are gone from params.yaml
    params = yaml.safe_load(paths.PARAMS_FILE.read_text())
    gate = params.get("gate") or {}
    assert "enabled" not in gate
    assert "on_fail" not in gate
