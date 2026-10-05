"""Offline recsys pipeline: evaluation, model registry and promotion gate.

Black-box: every scenario runs the real platform-recsys job image (`python -m recsys`)
against the stack's Redis and Qdrant, in a per-worker namespace (recsys_job_flow), and
asserts on what the job reports and what it leaves in the stores. Nothing is mocked and
nothing runs in-process.
"""

from __future__ import annotations

import time

from pytest_bdd import given, then, when

from tests.e2e.flows.recsys_job_flow import JobRun, run_recsys_job
from tests.e2e.support.world import World

# A gate that an equal-scoring retrain cannot pass (needs +10% ndcg@10).
_STRICT_GATE = {"PROMOTION_MIN_RELATIVE_IMPROVEMENT": "0.10"}


def _events(rows: list[tuple[str, str, str, float]]) -> list[dict]:
    now = time.time()
    return [
        {"user": u, "listing": lid, "event_type": et, "ts": now - hours_ago * 3600}
        for u, lid, et, hours_ago in rows
    ]


def _warehouse_events() -> list[dict]:
    """Three users with several interactions each: the temporal split has a holdout."""
    return _events(
        [
            ("u1", "l1", "view", 9),
            ("u1", "l1", "click", 8),
            ("u1", "l2", "view", 7),
            ("u1", "l3", "view", 6),
            ("u2", "l1", "view", 5),
            ("u2", "l2", "click", 4),
            ("u2", "l3", "view", 3),
            ("u3", "l2", "view", 2),
            ("u3", "l3", "add_to_cart", 1),
            ("u3", "l1", "view", 0),
        ]
    )


def _plan(world: World, events: list[dict], runs: list[dict[str, str]]) -> None:
    world.state.extra["recsys_plan"] = (events, runs)


def _execute(world: World) -> None:
    events, runs = world.state.extra["recsys_plan"]
    world.state.extra["recsys_runs"] = run_recsys_job(events, runs)


def _runs(world: World) -> list[JobRun]:
    return world.state.extra["recsys_runs"]


# ── Given ─────────────────────────────────────────────────────────────────
@given("an offline batch pipeline run over warehouse tracking events")
@given("an empty model registry with no existing champion")
@given("an offline pipeline execution")
def single_run_over_warehouse(world: World) -> None:
    # The namespace starts empty: no champion, no serving artifacts.
    _plan(world, _warehouse_events(), [{}])


@given("an interaction window with no test events after temporal split")
def window_without_holdout(world: World) -> None:
    # One interaction per user: the per-user temporal split holds nothing out.
    _plan(
        world,
        _events([("u1", "l1", "view", 0), ("u2", "l2", "view", 0), ("u3", "l3", "view", 0)]),
        [{}],
    )


@given("an incumbent champion model in the registry")
@given("a candidate model rejected by the promotion gate")
def champion_then_strict_retrain(world: World) -> None:
    # Run 1 bootstraps a champion (and publishes it); run 2 retrains on the same
    # data under a gate that needs +10%, so it evaluates below the tolerance.
    _plan(world, _warehouse_events(), [{}, _STRICT_GATE])


# ── When ──────────────────────────────────────────────────────────────────
@when("ALS training completes and the temporal holdout is scored")
@when("the evaluation stage runs")
@when("a candidate model evaluates below the incumbent champion tolerance")
@when("the pipeline run finishes")
@when("the initial pipeline run completes with valid metrics")
@when("the pipeline completes evaluation and gating")
def run_the_job(world: World) -> None:
    _execute(world)
    for run in _runs(world):
        assert run.exit_code == 0, f"the job signalled failure: {run.summary}"


# ── Then ──────────────────────────────────────────────────────────────────
@then("the run reports ndcg@10 and coverage@10 metrics attributed to the run's model_version")
def metrics_attributed_to_run(world: World) -> None:
    (run,) = _runs(world)
    version = run.summary["model_version"]
    metrics = run.summary["metrics"]
    assert "ndcg@10" in metrics and "coverage@10" in metrics, metrics
    assert metrics["test_events"] > 0, metrics
    # The registry holds exactly these metrics under this run's own version.
    recorded = run.state["models"].get(version)
    assert recorded is not None, f"{version} not in registry: {run.state['models']}"
    assert recorded["metrics"]["ndcg@10"] == metrics["ndcg@10"]
    assert run.state["items"]["model_versions"] == [version]


@then("no candidate is registered and the promotion gate is skipped")
def not_a_candidate(world: World) -> None:
    (run,) = _runs(world)
    assert run.summary["decision"] == "skipped", run.summary
    assert "no usable holdout" in run.summary["reason"]
    assert run.summary["metrics"].get("test_events", 0) == 0
    assert run.state["models"] == {}, run.state["models"]
    assert run.state["champion"] is None
    assert run.state["items"]["count"] == 0 and run.state["serving_model_version"] is None


@then("the candidate status is recorded as rejected")
def candidate_recorded_rejected(world: World) -> None:
    first, second = _runs(world)
    assert first.summary["decision"] == "promoted", first.summary
    assert second.summary["decision"] == "rejected", second.summary
    candidate = second.state["models"][second.summary["model_version"]]
    assert candidate["status"] == "rejected", candidate


@then("the champion key still names the previous version")
def champion_unchanged(world: World) -> None:
    first, second = _runs(world)
    assert second.state["champion"] == first.summary["model_version"]


@then("serving vector collections and active model_version remain on the previous generation")
def previous_generation_still_serving(world: World) -> None:
    first, second = _runs(world)
    previous = first.summary["model_version"]
    assert second.summary["decision"] == "rejected", second.summary
    assert "qdrant" not in second.summary and "cache" not in second.summary
    # Same points, still stamped with the previous generation (no publish, no prune).
    assert second.state["items"] == first.state["items"]
    assert second.state["users"] == first.state["users"]
    assert second.state["items"]["model_versions"] == [previous]
    assert second.state["serving_model_version"] == previous


@then("the initial model is promoted as champion and published to serving stores")
def initial_model_bootstraps(world: World) -> None:
    (run,) = _runs(world)
    version = run.summary["model_version"]
    assert run.summary["decision"] == "promoted", run.summary
    assert run.summary["incumbent_version"] is None
    assert run.state["champion"] == version
    assert run.state["items"]["count"] > 0 and run.state["items"]["model_versions"] == [version]
    assert run.state["users"]["count"] > 0
    assert run.state["serving_model_version"] == version
    assert run.state["popular_cached"]


@then("the summary reports model_version, metrics, decision, and promotion reason")
def summary_is_auditable(world: World) -> None:
    (run,) = _runs(world)
    s = run.summary
    assert s["model_version"] and isinstance(s["metrics"], dict)
    assert s["decision"] in ("promoted", "rejected") and s["reason"]
    # The comparison itself: which metric, the candidate's value and the incumbent's.
    assert s["primary_metric"] == "ndcg@10"
    assert s["candidate_value"] == s["metrics"]["ndcg@10"]
    assert "incumbent_value" in s and "incumbent_version" in s
