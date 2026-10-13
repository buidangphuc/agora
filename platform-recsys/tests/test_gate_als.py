"""recsys-als policy (ADR-0014 numbers) in the packaged policies.yaml: RAT-01..RAT-04."""

from __future__ import annotations

import pytest
from modelregistry import apply

M = "recsys-als"


def _champ(reg):
    return reg.add_version(M, {"ndcg@10": 0.100, "coverage@10": 0.50})


def test_als_promotable_then_applied_after_publish(reg):
    # RAT-01
    old = _champ(reg)
    d = reg.evaluate(M, {"ndcg@10": 0.102, "coverage@10": 0.45})
    assert d.outcome == "promotable"
    assert reg.aliases(M) == {"champion": old}  # evaluate moves no alias
    apply(M, d.version)
    assert reg.tags(M, d.version)["gate.outcome"] == "promoted"
    assert reg.aliases(M) == {"champion": d.version, "previous": old}


def test_als_below_threshold_rejected(reg):
    # RAT-02
    old = _champ(reg)
    d = reg.evaluate(M, {"ndcg@10": 0.1005, "coverage@10": 0.50})
    assert d.outcome == "rejected"
    tags = reg.tags(M, d.version)
    assert tags["gate.outcome"] == "rejected" and "ndcg@10" in tags["gate.reason"]
    assert reg.aliases(M) == {"champion": old}


def test_als_coverage_guard(reg):
    # RAT-03
    _champ(reg)
    d = reg.evaluate(M, {"ndcg@10": 0.150, "coverage@10": 0.39})
    assert d.outcome == "rejected"
    assert "coverage@10" in reg.tags(M, d.version)["gate.reason"]


def test_als_boundary_values_pass(reg):
    # RAT-04
    _champ(reg)
    d = reg.evaluate(M, {"ndcg@10": 0.101, "coverage@10": 0.40})
    assert d.outcome == "promotable"
    assert reg.tags(M, d.version)["gate.outcome"] == "promotable"


def test_als_first_candidate_without_champion_is_promotable(reg):
    # RAT-01 (no champion: judged on floors only; recsys-als has none)
    d = reg.evaluate(M, {"ndcg@10": 0.01, "coverage@10": 0.01})
    assert d.outcome == "promotable"


@pytest.mark.parametrize("bad", [{"ndcg@10": 0.2}, {"coverage@10": 0.5}])
def test_als_missing_metric_rejected(reg, bad):
    # RAT-02 (an incomplete candidate is rejected, not accepted)
    _champ(reg)
    assert reg.evaluate(M, bad).outcome == "rejected"
