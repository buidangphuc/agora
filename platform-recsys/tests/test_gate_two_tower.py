"""recsys-two-tower policy (+0.5 % recall@10, guard coverage@10 0.8, floor coverage@10 0.5)."""

from __future__ import annotations

M = "recsys-two-tower"


def test_first_candidate_below_floor_rejected(reg):
    # RAT-12
    d = reg.evaluate(M, {"recall@10": 0.3, "coverage@10": 0.49})
    assert d.outcome == "rejected"
    reason = reg.tags(M, d.version)["gate.reason"]
    assert "coverage@10" in reason and "0.5" in reason
    assert reg.aliases(M) == {}


def test_first_candidate_at_floor_is_promotable(reg):
    # RAT-12 (floor inclusive)
    assert reg.evaluate(M, {"recall@10": 0.3, "coverage@10": 0.5}).outcome == "promotable"


def test_boundary_inclusive(reg):
    # RAT-41: +0.5 % recall@10, coverage@10 exactly 0.8x the champion's and >= 0.5
    reg.add_version(M, {"recall@10": 0.2000, "coverage@10": 0.7500})
    d = reg.evaluate(M, {"recall@10": 0.2010, "coverage@10": 0.6000})
    assert d.outcome == "promotable"
    assert reg.tags(M, d.version)["gate.outcome"] == "promotable"


def test_floor_applies_even_with_a_champion(reg):
    # RAT-12 / RAT-41: within 0.8x of the champion but under the 0.5 floor is rejected
    reg.add_version(M, {"recall@10": 0.2000, "coverage@10": 0.5500})
    d = reg.evaluate(M, {"recall@10": 0.3000, "coverage@10": 0.4900})
    assert d.outcome == "rejected"
    assert "coverage@10" in d.reason
