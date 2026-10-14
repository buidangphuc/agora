"""Scenarios: forced promotion, unforced promote, rollback swap."""
from __future__ import annotations

import pytest

from modelregistry import RollbackError, promote, rollback


def test_force_promote_records_override(env):
    # Scenario: Force moves the alias and records the override
    old = env.add_version("m", {"ndcg@10": 0.1}, alias="champion")
    bad = env.add_version("m", {"ndcg@10": 0.0}, outcome="rejected")
    promote("m", bad, force=True, reason="hotfix for stale data")
    assert env.aliases("m") == {"champion": bad, "previous": old}
    tags = env.tags("m", bad)
    assert tags["gate.forced"] == "true"
    assert tags["gate.outcome"] == "promoted"
    assert tags["gate.reason"] == "hotfix for stale data"


@pytest.mark.parametrize("reason", [None, "", "   "])
def test_force_without_reason_raises_value_error(env, reason):
    # Scenario: Force without a reason is refused
    env.add_version("m", {"ndcg@10": 0.1}, alias="champion")
    bad = env.add_version("m", {"ndcg@10": 0.0}, outcome="rejected")
    before = env.snapshot("m")
    with pytest.raises(ValueError):
        promote("m", bad, force=True, reason=reason)
    assert env.snapshot("m") == before


def test_unforced_promote_that_passes_applies(env):
    # Scenario: Unforced promote that passes applies it
    env.add_version("m", {"ndcg@10": 0.100}, alias="champion")
    cand = env.add_version("m", {"ndcg@10": 0.200}, outcome=None)
    d = promote("m", cand)
    assert d.outcome == "promoted"
    assert env.aliases("m") == {"champion": cand, "previous": 1}


def test_unforced_promote_that_fails_leaves_champion(env):
    # Scenario: Unforced promote that fails the gate exits 0 with the decision (library side)
    env.add_version("m", {"ndcg@10": 0.100}, alias="champion")
    cand = env.add_version("m", {"ndcg@10": 0.100}, outcome=None)
    d = promote("m", cand)
    assert d.outcome == "rejected"
    assert env.aliases("m") == {"champion": 1}


def _two_champions(env):
    env.add_version("m", {"ndcg@10": 0.1}, alias="previous")
    env.add_version("m", {"ndcg@10": 0.2}, alias="champion")


def test_rollback_swaps_and_tags_reason(env):
    # Scenario: Rollback swaps the aliases
    _two_champions(env)
    rollback("m")
    assert env.aliases("m") == {"champion": 1, "previous": 2}
    assert env.tags("m", 1)["gate.reason"].startswith("rollback")


def test_double_rollback_toggles(env):
    # Scenario: A second rollback toggles back
    _two_champions(env)
    rollback("m")
    rollback("m")
    assert env.aliases("m") == {"champion": 2, "previous": 1}


def test_rollback_without_previous_raises(env):
    # Scenario: Rollback without a previous is refused
    env.add_version("m", {"ndcg@10": 0.1}, alias="champion")
    before = env.snapshot("m")
    with pytest.raises(RollbackError):
        rollback("m")
    assert env.snapshot("m") == before
