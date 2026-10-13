"""Port of the Redis ModelRegistry tests (tests/test_registry.py) to registry aliases and tags.

Same intent, observed on the MLflow file store: a candidate is recorded with its metrics, the first one
becomes champion, a better one replaces it (the old holder becomes ``previous``), a worse one is rejected,
an engineer can force, a champion from another evaluation protocol is not compared, and a decision changes
nothing until it is applied.
"""

from __future__ import annotations

import pytest
from modelregistry import GateError, apply, get_champion, mark_rejected, promote

M = "recsys-als"
OK = {"ndcg@10": 0.40, "coverage@10": 0.50}


def test_metrics_and_lineage_round_trip_through_the_registry(reg):
    # was: test_model_metadata_serialization
    d = reg.evaluate(M, {"ndcg@10": 0.42, "coverage@10": 0.60}, protocol="leave-last-new-item-v1")
    apply(M, d.version)
    champ = get_champion(M)
    assert champ["version"] == d.version
    assert champ["metrics"]["ndcg@10"] == pytest.approx(0.42)
    assert champ["metrics"]["coverage@10"] == pytest.approx(0.60)
    assert champ["tags"]["lineage.eval_protocol"] == "leave-last-new-item-v1"
    assert champ["tags"]["gate.outcome"] == "promoted"


def test_initial_promotion(reg):
    # was: test_registry_initial_promotion
    d = reg.evaluate(M, OK)
    assert d.outcome == "promotable" and reg.aliases(M) == {}
    apply(M, d.version)
    assert reg.aliases(M) == {"champion": d.version}
    assert reg.tags(M, d.version)["gate.outcome"] == "promoted"


def test_challenger_promotion_gate(reg):
    # was: test_registry_challenger_promotion_gate
    first = reg.add_version(M, OK)
    better = reg.evaluate(M, {"ndcg@10": 0.45, "coverage@10": 0.55})
    assert better.outcome == "promotable"
    apply(M, better.version)
    assert reg.aliases(M) == {"champion": better.version, "previous": first}  # was: first -> "archived"
    assert reg.tags(M, better.version)["gate.outcome"] == "promoted"

    worse = reg.evaluate(M, {"ndcg@10": 0.35, "coverage@10": 0.50})
    assert worse.outcome == "rejected"
    assert reg.tags(M, worse.version)["gate.outcome"] == "rejected"
    assert reg.aliases(M) == {"champion": better.version, "previous": first}


def test_a_rejected_candidate_cannot_be_applied(reg):
    # the rejected candidate never reaches the champion alias, even if someone tries
    reg.add_version(M, OK)
    worse = reg.evaluate(M, {"ndcg@10": 0.35, "coverage@10": 0.50})
    with pytest.raises(GateError):
        apply(M, worse.version)


def test_only_an_engineer_forces_a_promotion(reg):
    # was: test_force_promotes_a_candidate_the_gate_would_reject (PROMOTION_FORCE is gone: see RAT-35)
    same = {"ndcg@10": 0.5, "coverage@10": 0.8}
    first = reg.add_version(M, same)
    equal = reg.evaluate(M, same)
    assert equal.outcome == "rejected" and reg.aliases(M) == {"champion": first}
    promote(M, equal.version, force=True, reason="engineer override for the test")
    assert reg.aliases(M)["champion"] == equal.version
    assert reg.tags(M, equal.version).get("gate.forced")  # a forced promotion is marked as such


def test_champion_from_another_eval_protocol_is_not_compared(reg):
    # was: test_champion_from_another_eval_protocol_is_not_compared (now bounded by the floors)
    old = reg.add_version(M, {"ndcg@10": 0.75, "coverage@10": 0.8}, protocol="leave-last-new-item-v0")
    new = reg.evaluate(M, {"ndcg@10": 0.05, "coverage@10": 0.3}, protocol="leave-last-new-item-v1")
    assert new.outcome == "promotable" and new.reason.startswith("eval_protocol changed")
    apply(M, new.version)
    assert reg.aliases(M) == {"champion": new.version, "previous": old}


def test_evaluate_changes_nothing_until_the_decision_is_applied(reg):
    # was: test_evaluate_changes_nothing_until_the_decision_is_applied
    v1 = reg.add_version(M, {"ndcg@10": 0.30, "coverage@10": 0.9})
    d = reg.evaluate(M, {"ndcg@10": 0.40, "coverage@10": 0.9})
    assert d.outcome == "promotable" and reg.aliases(M) == {"champion": v1}

    mark_rejected(M, d.version, "publish failed: RuntimeError")  # was: registry.reject(decision, reason=...)

    assert reg.aliases(M) == {"champion": v1}
    assert reg.tags(M, v1)["gate.outcome"] == "promoted"
    tags = reg.tags(M, d.version)
    assert tags["gate.outcome"] == "rejected" and tags["gate.reason"] == "publish failed: RuntimeError"

    v3 = reg.evaluate(M, {"ndcg@10": 0.50, "coverage@10": 0.9})
    apply(M, v3.version)
    assert reg.aliases(M) == {"champion": v3.version, "previous": v1}
