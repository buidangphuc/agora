"""Scenarios: get_champion, apply / mark_rejected / register_rejected / submit."""
from __future__ import annotations

import pytest

from modelregistry import (
    GateError, apply, evaluate, get_champion, mark_rejected, register_rejected, submit,
)


def ev(env, metrics, protocol="p1", model="m"):
    c = env.candidate(metrics, protocol=protocol)
    return evaluate(model, c["run_id"], c["metrics"], c["artifact_path"], c["lineage"])


def test_get_champion_returns_run_and_metrics(env):
    # Scenario: Read the champion
    v = env.add_version("m", {"ndcg@10": 0.5}, alias="champion")
    got = get_champion("m")
    assert int(got["version"]) == v
    assert got["run_id"] == env.run_id("m", v)
    assert got["metrics"]["ndcg@10"] == pytest.approx(0.5)
    assert got["tags"]["gate.outcome"] == "promoted"


def test_get_champion_none_without_champion(env):
    # Scenario: No champion returns None, not an error
    assert get_champion("ghost") is None
    assert env.client.search_registered_models(filter_string="name='ghost'") == []
    env.add_version("m", {"ndcg@10": 0.5}, outcome="rejected")
    before = env.snapshot("m")
    assert get_champion("m") is None
    assert env.snapshot("m") == before


def test_apply_moves_champion_and_sets_previous(env):
    # Scenario: Apply after evaluate moves the aliases
    env.add_version("m", {"ndcg@10": 0.100}, alias="champion")
    d = ev(env, {"ndcg@10": 0.2})
    assert d.outcome == "promotable"
    apply("m", d.version)
    assert env.aliases("m") == {"champion": 2, "previous": 1}
    assert env.tags("m", 2)["gate.outcome"] == "promoted"


def test_apply_refused_on_rejected(env):
    # Scenario: Apply is refused on a rejected version
    env.add_version("m", {"ndcg@10": 0.1}, alias="champion")
    bad = env.add_version("m", {"ndcg@10": 0.0}, outcome="rejected")
    before = env.snapshot("m")
    with pytest.raises(GateError):
        apply("m", bad)
    assert env.snapshot("m") == before


def test_apply_refused_on_pending(env):
    # Scenario: Apply is refused on a pending version
    env.add_version("m", {"ndcg@10": 0.1}, alias="champion")
    pend = env.add_version("m", {"ndcg@10": 0.9}, outcome=None)
    before = env.snapshot("m")
    with pytest.raises(GateError):
        apply("m", pend)
    assert env.snapshot("m") == before


def test_apply_refused_on_already_promoted(env):
    # Scenario: Apply is refused on a rejected version (spec: anything but promotable)
    v = env.add_version("m", {"ndcg@10": 0.1}, alias="champion")
    before = env.snapshot("m")
    with pytest.raises(GateError):
        apply("m", v)
    assert env.snapshot("m") == before


def test_crash_between_evaluate_and_apply_leaves_aliases(env):
    # Scenario: Crash between evaluate and apply changes nothing served
    env.add_version("m", {"ndcg@10": 0.100}, alias="champion")
    stale = ev(env, {"ndcg@10": 0.102})  # promotable, never applied
    assert stale.outcome == "promotable"
    assert env.aliases("m") == {"champion": 1}
    assert env.tags("m", int(stale.version))["gate.outcome"] == "promotable"
    # passes vs champion (+1.5 %) but would fail vs the stale promotable (0.102)
    nxt = ev(env, {"ndcg@10": 0.1015})
    assert nxt.outcome == "promotable"
    assert env.aliases("m") == {"champion": 1}


def test_mark_rejected_touches_no_alias(env):
    # Scenario: mark_rejected after a downstream failure
    env.add_version("m", {"ndcg@10": 0.100}, alias="champion")
    d = ev(env, {"ndcg@10": 0.2})
    mark_rejected("m", d.version, "publish failed")
    tags = env.tags("m", int(d.version))
    assert tags["gate.outcome"] == "rejected"
    assert tags["gate.reason"] == "publish failed"
    assert env.aliases("m") == {"champion": 1}


def test_register_rejected_skips_policy(env):
    # Scenario: register_rejected records a structural rejection
    model = "no-policy-model"  # absent from the policy file: the policy must not be read
    run_id = env.new_run()
    register_rejected(model, run_id, "dataset stale",
                      {"data_version": "d@v9", "eval_protocol": "p9"})
    assert env.versions(model) == [1]
    tags = env.tags(model, 1)
    assert tags["gate.outcome"] == "rejected"
    assert tags["gate.reason"] == "dataset stale"
    assert tags["lineage.data_version"] == "d@v9"
    assert tags["lineage.eval_protocol"] == "p9"
    assert env.aliases(model) == {}


def test_submit_is_evaluate_then_apply(env):
    # Scenario: Submit is evaluate then apply
    def sub(metrics):
        c = env.candidate(metrics)
        return submit("m", c["run_id"], c["metrics"], c["artifact_path"], c["lineage"])

    first = sub({"ndcg@10": 0.100})
    assert first.outcome == "promoted"
    second = sub({"ndcg@10": 0.200})
    assert second.outcome == "promoted"
    assert env.aliases("m") == {"champion": 2, "previous": 1}
    third = sub({"ndcg@10": 0.150})
    assert third.outcome == "rejected"
    assert env.aliases("m") == {"champion": 2, "previous": 1}
