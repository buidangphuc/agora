"""Scenarios from specs/model-publish-gate: evaluate, protocol, missing metrics, first version."""
from __future__ import annotations

import math

import pytest

from modelregistry import Decision, apply, evaluate

GUARD_POLICY = {
    "m": {"primary": "ndcg@10", "min_relative_improvement": 0.01,
          "guards": {"coverage@10": 0.8}},
}


def ev(env, metrics, protocol="p1", model="m", hw=None):
    c = env.candidate(metrics, protocol=protocol, hw=hw)
    return evaluate(model, c["run_id"], c["metrics"], c["artifact_path"], c["lineage"])


def champion(env, metrics, protocol="p1", model="m", **kw):
    return env.add_version(model, metrics, protocol=protocol, outcome="promoted",
                           alias="champion", **kw)


def test_pass_is_promotable_and_moves_no_alias(env):
    # Scenario: Passing candidate is promotable and no alias moves
    env.set_policies(GUARD_POLICY)
    champion(env, {"ndcg@10": 0.100, "coverage@10": 0.50})
    d = ev(env, {"ndcg@10": 0.102, "coverage@10": 0.45})
    assert isinstance(d, Decision)
    assert d.outcome == "promotable"
    assert env.tags("m", int(d.version))["gate.outcome"] == "promotable"
    assert env.aliases("m") == {"champion": 1}


def test_insufficient_improvement_rejected(env):
    # Scenario: Insufficient improvement is rejected
    env.set_policies(GUARD_POLICY)
    champion(env, {"ndcg@10": 0.100, "coverage@10": 0.50})
    d = ev(env, {"ndcg@10": 0.1005, "coverage@10": 0.50})
    assert d.outcome == "rejected"
    assert "ndcg@10" in d.reason and "0.01" in d.reason
    tags = env.tags("m", int(d.version))
    assert tags["gate.outcome"] == "rejected" and tags["gate.reason"]
    assert env.aliases("m") == {"champion": 1}


def test_guard_violation_rejected(env):
    # Scenario: Guard violation is rejected
    env.set_policies(GUARD_POLICY)
    champion(env, {"ndcg@10": 0.100, "coverage@10": 0.50})
    d = ev(env, {"ndcg@10": 0.120, "coverage@10": 0.39})
    assert d.outcome == "rejected"
    assert "coverage@10" in d.reason


def test_thresholds_are_inclusive(env):
    # Scenario: Thresholds are inclusive
    env.set_policies(GUARD_POLICY)
    champion(env, {"ndcg@10": 0.100, "coverage@10": 0.50})
    d = ev(env, {"ndcg@10": 0.101, "coverage@10": 0.40})  # exactly +1 %, exactly 0.8 * 0.50
    assert d.outcome == "promotable", d.reason


def test_threshold_tolerance_1e9(env):
    # Scenario: Floating-point rounding at the threshold is tolerated
    env.set_policies({"m": {"primary": "ndcg@10", "min_relative_improvement": 0.005}})
    champion(env, {"ndcg@10": 0.5})
    assert ev(env, {"ndcg@10": 0.5025}).outcome == "promotable"
    # boundary just outside the tolerance (0.48 % < 0.5 %) is rejected
    assert ev(env, {"ndcg@10": 0.5024}).outcome == "rejected"


def test_champion_metric_uses_latest_value(env):
    # Scenario: Champion metric is the latest value
    champion(env, {}, steps={"ndcg@10": [0.4, 0.5]})
    d = ev(env, {"ndcg@10": 0.504})  # +0.8 % vs 0.5 -> rejected; would pass vs 0.4
    assert d.outcome == "rejected"
    assert ev(env, {"ndcg@10": 0.505}).outcome == "promotable"  # +1 % vs 0.5


def test_artifact_run_and_lineage_tags(env):
    # Scenario: Run, artifact and lineage are attached to the version
    c = env.candidate({"ndcg@10": 0.2}, protocol="p1", hw="cpu")
    c["lineage"]["data_version"] = "als_interactions@v1"
    d = evaluate("m", c["run_id"], c["metrics"], c["artifact_path"], c["lineage"])
    v = int(d.version)
    assert env.run_id("m", v) == c["run_id"]
    assert any(p.endswith("weights.bin") for p in
               [f.path for f in env.client.list_artifacts(c["run_id"], "model")])
    assert env.run_metrics(c["run_id"])["ndcg@10"] == pytest.approx(0.2)
    tags = env.tags("m", v)
    assert tags["lineage.data_version"] == "als_interactions@v1"
    assert tags["lineage.eval_protocol"] == "p1"
    assert tags["lineage.train_hw"] == "cpu"
    d2 = ev(env, {"ndcg@10": 0.2})  # no train_hw given
    assert "lineage.train_hw" not in env.tags("m", int(d2.version))


def test_metric_keys_are_encoded_for_mlflow(env):
    # Scenario: Run, artifact and lineage are attached to the version
    # MLflow rejects '@' in metric names, so `ndcg@10` is stored as `ndcg_at_10` (assumption).
    c = env.candidate({"ndcg@10": 0.2, "coverage@10": 0.5})
    evaluate("m", c["run_id"], c["metrics"], c["artifact_path"], c["lineage"])
    raw = env.client.get_run(c["run_id"]).data.metrics
    assert raw["ndcg_at_10"] == pytest.approx(0.2)
    assert raw["coverage_at_10"] == pytest.approx(0.5)
    assert not any("@" in k for k in raw)


def test_decision_is_deterministic(env):
    # Scenario: Decision is deterministic
    env.set_policies(GUARD_POLICY)
    out = []
    for store in ("a", "b"):
        env.use_store(store)
        champion(env, {"ndcg@10": 0.100, "coverage@10": 0.50})
        d = ev(env, {"ndcg@10": 0.1005, "coverage@10": 0.50})
        out.append((d.outcome, d.reason))
    assert out[0] == out[1]


# -- protocol -----------------------------------------------------------------
PROTO_POLICY = {"m": {"primary": "ndcg@10", "min_relative_improvement": 0.01,
                      "floors": {"ndcg@10": 0.05}}}


def test_protocol_mismatch_floors_only(env):
    # Scenario: Protocol mismatch judges floors only
    env.set_policies(PROTO_POLICY)
    champion(env, {"ndcg@10": 0.300}, protocol="p1")
    d = ev(env, {"ndcg@10": 0.060}, protocol="p2")
    assert d.outcome == "promotable"
    assert d.reason.startswith("eval_protocol changed: p1 -> p2")


def test_protocol_mismatch_below_floor_rejected(env):
    # Scenario: Protocol mismatch below a floor is rejected
    env.set_policies(PROTO_POLICY)
    champion(env, {"ndcg@10": 0.300}, protocol="p1")
    d = ev(env, {"ndcg@10": 0.040}, protocol="p2")
    assert d.outcome == "rejected"
    assert "ndcg@10" in d.reason and "0.05" in d.reason


def test_protocol_match_is_compared(env):
    # Scenario: Matching protocol is compared
    env.set_policies(PROTO_POLICY)
    champion(env, {"ndcg@10": 0.300}, protocol="p1")
    d = ev(env, {"ndcg@10": 0.200}, protocol="p1")
    assert d.outcome == "rejected"
    assert not d.reason.startswith("eval_protocol changed")
    assert "ndcg@10" in d.reason


def test_champion_without_protocol_tag(env):
    # Scenario: Champion without a protocol tag
    env.set_policies(PROTO_POLICY)
    champion(env, {"ndcg@10": 0.300}, protocol=None)
    d = ev(env, {"ndcg@10": 0.060}, protocol="p1")
    assert d.reason.startswith("eval_protocol changed: none -> p1")
    assert d.outcome == "promotable"


# -- missing / non-finite ---------------------------------------------------
def test_missing_primary_rejected(env):
    # Scenario: Candidate missing the primary metric
    env.set_policies(GUARD_POLICY)
    d = ev(env, {"coverage@10": 0.5})  # no exception
    assert d.outcome == "rejected"
    assert "ndcg@10" in d.reason and "missing" in d.reason
    assert env.tags("m", int(d.version))["gate.outcome"] == "rejected"


def test_non_finite_metrics_rejected(env):
    # Scenario: Candidate metric is NaN or infinite
    env.set_policies({
        "m": {"primary": "ndcg@10", "guards": {"coverage@10": 0.8}},
        "m2": {"primary": "ndcg@10", "floors": {"coverage@10": 0.5}},
    })
    champion(env, {"ndcg@10": 0.1, "coverage@10": 0.5})
    d = ev(env, {"ndcg@10": 0.2, "coverage@10": math.nan})
    assert d.outcome == "rejected"
    assert "coverage@10" in d.reason and "finite" in d.reason
    d2 = ev(env, {"ndcg@10": 0.2, "coverage@10": math.inf}, model="m2")
    assert d2.outcome == "rejected"
    assert "coverage@10" in d2.reason and "finite" in d2.reason


def test_missing_floor_metric_rejected(env):
    # Scenario: Candidate missing a floor metric
    env.set_policies({"m": {"primary": "ndcg@10", "floors": {"coverage@10": 0.5}}})
    d = ev(env, {"ndcg@10": 0.2})
    assert d.outcome == "rejected"
    assert "coverage@10" in d.reason


def test_guard_missing_on_champion_skipped(env):
    # Scenario: Guard missing on the champion is skipped and noted
    env.set_policies({"m": {"primary": "ndcg@10", "min_relative_improvement": 0.01,
                            "guards": {"recall@10": 0.95}}})
    champion(env, {"ndcg@10": 0.100})
    d = ev(env, {"ndcg@10": 0.120, "recall@10": 0.1})
    assert d.outcome == "promotable"
    assert "recall@10" in d.reason and "skipped" in d.reason


@pytest.mark.parametrize("champ", [0.0, -0.2])
def test_champion_primary_not_positive(env, champ):
    # Scenario: Champion primary is zero or negative
    env.set_policies({"m": {"primary": "ndcg@10", "min_relative_improvement": 0.01,
                            "floors": {"coverage@10": 0.5}}})
    champion(env, {"ndcg@10": champ, "coverage@10": 0.9})
    assert ev(env, {"ndcg@10": 0.05, "coverage@10": 0.6}).outcome == "promotable"
    assert ev(env, {"ndcg@10": champ, "coverage@10": 0.6}).outcome == "rejected"


# -- first version ----------------------------------------------------------
FLOOR_POLICY = {"m": {"primary": "ndcg@10", "floors": {"ndcg@10": 0.05}}}


def test_first_champion_meets_floors(env):
    # Scenario: First champion via floors
    env.set_policies(FLOOR_POLICY)
    d = ev(env, {"ndcg@10": 0.06})
    assert d.outcome == "promotable"
    assert env.aliases("m") == {}
    apply("m", d.version)
    assert env.aliases("m") == {"champion": int(d.version)}


def test_first_candidate_below_floor_rejected(env):
    # Scenario: Candidate below a floor is rejected and no champion is set
    env.set_policies(FLOOR_POLICY)
    d = ev(env, {"ndcg@10": 0.049})
    assert d.outcome == "rejected"
    assert "ndcg@10" in d.reason and "0.05" in d.reason
    assert "champion" not in env.aliases("m")


def test_floor_is_inclusive(env):
    # Scenario: Candidate exactly at the floor passes
    env.set_policies(FLOOR_POLICY)
    assert ev(env, {"ndcg@10": 0.05}).outcome == "promotable"
