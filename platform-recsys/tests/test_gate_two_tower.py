"""recsys-two-tower policy (+0.5 % recall@10, guard coverage@10 0.8, floor recall@10 0.000001).

There is no coverage floor: real catalogue coverage is ~0.0009 (clarification 2026-10-13).
"""

from __future__ import annotations

M = "recsys-two-tower"


def test_first_candidate_with_zero_recall_rejected(reg):
    # RAT-12: the recall@10 floor (0.000001) rejects a first candidate that retrieves nothing
    d = reg.evaluate(M, {"recall@10": 0.0, "coverage@10": 0.9})
    assert d.outcome == "rejected"
    reason = reg.tags(M, d.version)["gate.reason"]
    assert "recall@10" in reason and "coverage@10" not in reason
    assert reg.aliases(M) == {}


def test_first_candidate_with_tiny_coverage_becomes_champion(reg):
    # RAT-12: no coverage floor; recall > 0 is enough for a first champion
    d = reg.evaluate(M, {"recall@10": 0.0004, "coverage@10": 0.001})
    assert d.outcome == "promotable"
    assert reg.tags(M, d.version)["gate.outcome"] == "promotable"


def test_first_candidate_at_recall_floor_is_promotable(reg):
    # RAT-12 (floor inclusive)
    assert reg.evaluate(M, {"recall@10": 0.000001, "coverage@10": 0.001}).outcome == "promotable"


def test_boundary_inclusive(reg):
    # RAT-41: +0.5 % recall@10, coverage@10 exactly 0.8x the champion's
    reg.add_version(M, {"recall@10": 0.2000, "coverage@10": 0.7500})
    d = reg.evaluate(M, {"recall@10": 0.2010, "coverage@10": 0.6000})
    assert d.outcome == "promotable"
    assert reg.tags(M, d.version)["gate.outcome"] == "promotable"


def test_coverage_guard_still_applies_with_a_champion(reg):
    # RAT-41: below 0.8x the champion's coverage is rejected, whatever the absolute value
    reg.add_version(M, {"recall@10": 0.2000, "coverage@10": 0.5500})
    d = reg.evaluate(M, {"recall@10": 0.3000, "coverage@10": 0.4300})
    assert d.outcome == "rejected"
    assert "coverage@10" in d.reason


def test_recall_floor_applies_even_with_a_champion(reg):
    # RAT-12 / RAT-41: zero recall is rejected by the floor (and by the improvement rule)
    reg.add_version(M, {"recall@10": 0.0, "coverage@10": 0.5})
    d = reg.evaluate(M, {"recall@10": 0.0, "coverage@10": 0.5})
    assert d.outcome == "rejected"
    assert "recall@10" in d.reason
