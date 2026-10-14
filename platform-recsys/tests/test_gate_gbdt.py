"""recsys-gbdt policy (+0.5 % ndcg@10, guard recall@10 0.95) in the packaged policies.yaml."""

from __future__ import annotations

M = "recsys-gbdt"


def test_equal_metric_rejected(reg):
    # RAT-40
    old = reg.add_version(M, {"ndcg@10": 0.2000, "recall@10": 0.5})
    d = reg.evaluate(M, {"ndcg@10": 0.2000, "recall@10": 0.5})
    assert d.outcome == "rejected"
    assert "ndcg@10" in reg.tags(M, d.version)["gate.reason"]
    assert reg.aliases(M) == {"champion": old}


def test_boundary_inclusive(reg):
    # RAT-41: exactly +0.5 % and exactly 0.95x the champion's recall@10
    reg.add_version(M, {"ndcg@10": 0.2000, "recall@10": 0.4000})
    d = reg.evaluate(M, {"ndcg@10": 0.2010, "recall@10": 0.3800})
    assert d.outcome == "promotable"
    assert reg.tags(M, d.version)["gate.outcome"] == "promotable"


def test_recall_guard_rejects(reg):
    # RAT-41 (just outside the guard)
    reg.add_version(M, {"ndcg@10": 0.2000, "recall@10": 0.4000})
    d = reg.evaluate(M, {"ndcg@10": 0.3000, "recall@10": 0.3700})
    assert d.outcome == "rejected"
    assert "recall@10" in d.reason


def test_champion_without_guard_metric(reg):
    # RAT-42 (policy shape: a seeded champion with ndcg@10 only skips the guard)
    reg.add_version(M, {"ndcg@10": 0.2000})
    d = reg.evaluate(M, {"ndcg@10": 0.2100, "recall@10": 0.1})
    assert d.outcome == "promotable"
    assert "recall@10" in d.reason
