"""Scenarios: Unknown model has no policy; Malformed policy is rejected; Resolution order;
Shipped policies.yaml loads and holds no live model; Omitted optional keys take defaults."""
from __future__ import annotations

from importlib import resources

import pytest
import yaml

from modelregistry import PolicyError, evaluate


def _eval(env, model="m", metrics=None, policies_path=None):
    c = env.candidate(metrics if metrics is not None else {"ndcg@10": 0.2})
    return evaluate(model, c["run_id"], c["metrics"], c["artifact_path"], c["lineage"],
                    policies_path=policies_path)


def test_unknown_model_raises_before_any_write(env):
    # Scenario: Unknown model has no policy
    with pytest.raises(PolicyError) as ei:
        _eval(env, model="not-in-policies")
    assert "not-in-policies" in str(ei.value)
    assert env.versions("not-in-policies") == []
    assert env.aliases("not-in-policies") == {}


@pytest.mark.parametrize(
    "entry,key",
    [
        ({"primary": "ndcg@10", "guards": {"coverage@10": 0}}, "coverage@10"),
        ({"primary": "ndcg@10", "min_improvment": 0.01}, "min_improvment"),
    ],
)
def test_malformed_entries_raise(env, entry, key):
    # Scenario: Malformed policy is rejected
    path = env.set_policies({"bad-model": entry})
    with pytest.raises(PolicyError) as ei:
        _eval(env, model="bad-model", policies_path=str(path))
    assert "bad-model" in str(ei.value)
    assert key in str(ei.value)
    assert env.versions("bad-model") == []


@pytest.mark.parametrize(
    "entry",
    [
        {"min_relative_improvement": 0.01},                         # primary required
        {"primary": 5},                                             # wrong type
        {"primary": "ndcg@10", "min_relative_improvement": -0.1},   # out of range
        {"primary": "ndcg@10", "floors": ["a"]},                    # wrong type
    ],
)
def test_malformed_entries_raise_more_classes(env, entry):
    # Scenario: Malformed policy is rejected (equivalence classes: missing, type, range)
    path = env.set_policies({"bad-model": entry})
    with pytest.raises(PolicyError) as ei:
        _eval(env, model="bad-model", policies_path=str(path))
    assert "bad-model" in str(ei.value)
    assert env.versions("bad-model") == []


def test_resolution_order(env, tmp_path, monkeypatch):
    # Scenario: Resolution order. Observable: which `primary` the gate used.
    arg_file = env.set_policies({"x": {"primary": "arg_metric"}}, tmp_path / "arg.yaml")
    env_file = env.set_policies({"x": {"primary": "env_metric"}}, tmp_path / "env.yaml")
    monkeypatch.setenv("MODELREGISTRY_POLICIES", str(env_file))

    # argument wins: arg_metric present -> promotable
    d = _eval(env, "x", {"arg_metric": 1.0}, policies_path=str(arg_file))
    assert d.outcome == "promotable"
    # env wins without the argument: env_metric missing -> rejected naming it
    d = _eval(env, "x", {"arg_metric": 1.0})
    assert d.outcome == "rejected"
    assert "env_metric" in d.reason
    # neither: packaged file used, holds no entry for x
    monkeypatch.delenv("MODELREGISTRY_POLICIES")
    with pytest.raises(PolicyError):
        _eval(env, "x", {"arg_metric": 1.0})


def test_shipped_file_loads(env, monkeypatch):
    # Scenario: Shipped policies.yaml loads and holds no live model
    monkeypatch.delenv("MODELREGISTRY_POLICIES", raising=False)
    text = resources.files("modelregistry").joinpath("policies.yaml").read_text()
    data = yaml.safe_load(text) or {}
    assert set(data) <= {"finetune-classifier", "recsys-als", "recsys-gbdt", "recsys-two-tower"}
    assert "#" in text  # commented example is documented but not loaded
    # loading through the public API succeeds: a model absent from it is PolicyError, not a parse error
    with pytest.raises(PolicyError) as ei:
        _eval(env, "no-such-model")
    assert "no-such-model" in str(ei.value)


def test_defaults(env):
    # Scenario: Omitted optional keys take defaults
    env.set_policies({"d": {"primary": "f1"}})
    # no floors: first candidate with finite primary is promotable
    first = _eval(env, "d", {"f1": 0.30})
    assert first.outcome == "promotable"
    from modelregistry import apply
    apply("d", first.version)
    # min_relative_improvement=0.0: an equal candidate passes (inclusive), no guards required
    equal = _eval(env, "d", {"f1": 0.30})
    assert equal.outcome == "promotable"
    # but a worse one fails
    worse = _eval(env, "d", {"f1": 0.29})
    assert worse.outcome == "rejected"
