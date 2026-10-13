"""Offline recsys pipeline: evaluation, model registry and promotion gate.

Black-box: every scenario runs the real platform-recsys job image (`python -m recsys`)
against the stack's Redis and Qdrant, in a per-worker namespace (recsys_job_flow), and
asserts on what the job reports, what it leaves in the stores and what it registers in MLflow.
Nothing is mocked and nothing runs in-process.

The registry is MLflow (recsys-auto-train): the job reads MLFLOW_TRACKING_URI, every scenario
starts from an empty `recsys-*` registry (it is shared by all workers, so these scenarios are
@destructive) and the gate decides from the `recsys-als` policy (+1 % ndcg@10 over the champion,
inclusive). An equal-scoring retrain cannot clear that, which is how a rejected candidate is made
without a lever in the job's environment. A rejected incumbent comparison is also driven by seeding
a champion no candidate can reach.
"""

from __future__ import annotations

import time

import pytest
from pytest_bdd import given, then, when

from src.api.mlflow_registry import STRONG, MlflowRegistry
from src.api.recsys_job import MLFLOW_IN_STACK
from tests.e2e.flows.recsys_job_flow import JobRun, run_recsys_job
from tests.e2e.support.world import World

_MLFLOW_ENV = {"MLFLOW_TRACKING_URI": MLFLOW_IN_STACK}
ALS = "recsys-als"


def registry(world: World) -> MlflowRegistry:
    return world.state.extra.setdefault("mlflow_registry", MlflowRegistry())


def newest_als(world: World) -> dict:
    version = registry(world).newest_version(ALS)
    assert version is not None, "recsys-als has no version in MLflow"
    return version


def newest_tags(world: World) -> dict[str, str]:
    return registry(world).tags(newest_als(world))


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
    # The registry is shared by every worker: start from an empty one (D13 reset).
    try:
        registry(world).reset_registry()
    except Exception as exc:  # noqa: BLE001
        pytest.fail(f"MLflow registry not reachable or not resettable: {exc!r}")
    world.state.extra["recsys_plan"] = (events, [{**_MLFLOW_ENV, **run} for run in runs])


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
    # The namespace and the registry start empty: no champion, no serving artifacts.
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
def seeded_strong_champion(world: World) -> None:
    # A champion seeded in MLflow that no candidate can reach: the run's candidate is rejected.
    _plan(world, _warehouse_events(), [{}])
    registry(world).seed(ALS, STRONG[ALS])
    world.state.extra["seeded_version"] = registry(world).alias(ALS, "champion")
    assert world.state.extra["seeded_version"] is not None


@given("a candidate model rejected by the promotion gate")
def bootstrap_then_equal_retrain(world: World) -> None:
    # Run 1 bootstraps a champion (and publishes it); run 2 retrains on the same data, scores the
    # same, and cannot clear the +1 % the recsys-als policy asks for.
    _plan(world, _warehouse_events(), [{}, {}])


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
        assert run.exit_code == 0, f"the job signalled failure: {run.summary}\n{run.log}"


# ── Then ──────────────────────────────────────────────────────────────────
@then("the run reports ndcg@10 and coverage@10 metrics attributed to the run's model_version")
def metrics_attributed_to_run(world: World) -> None:
    (run,) = _runs(world)
    version = run.summary["model_version"]
    metrics = run.summary["metrics"]
    assert "ndcg@10" in metrics and "coverage@10" in metrics, metrics
    assert metrics["test_events"] > 0, metrics
    # MLflow holds exactly these metrics on the newest recsys-als version (the library stores `@`
    # as `_at_`), and the stores hold exactly this run's generation.
    recorded = registry(world).run_metrics(newest_als(world)["run_id"])
    assert recorded.get("ndcg_at_10") == pytest.approx(metrics["ndcg@10"]), recorded
    assert recorded.get("coverage_at_10") == pytest.approx(metrics["coverage@10"]), recorded
    assert run.state["items"]["model_versions"] == [version]


@then("no candidate is registered and the promotion gate is skipped")
def not_a_candidate(world: World) -> None:
    (run,) = _runs(world)
    assert run.summary["decision"] == "skipped", run.summary
    assert "no usable holdout" in run.summary["reason"]
    assert run.summary["metrics"].get("test_events", 0) == 0
    counts = registry(world).version_counts()
    assert not counts.get(ALS), f"a recsys-als version was registered: {counts}"
    assert registry(world).alias(ALS, "champion") is None
    assert (
        run.state["items"]["count"] == 0 and run.state["serving_model_version"] is None
    ), run.state


@then("the candidate status is recorded as rejected")
def candidate_recorded_rejected(world: World) -> None:
    (run,) = _runs(world)
    seeded = world.state.extra["seeded_version"]
    newest = newest_als(world)
    assert int(newest["version"]) != seeded, "the newest version is the seeded champion"
    assert newest_tags(world).get("gate.outcome") == "rejected", newest_tags(world)
    assert run.summary["decision"] == "rejected", run.summary


@then("the champion alias still names the previous version")
def champion_unchanged(world: World) -> None:
    seeded = world.state.extra["seeded_version"]
    assert registry(world).alias(ALS, "champion") == seeded, (
        registry(world).alias(ALS, "champion"),
        seeded,
    )


@then("serving vector collections and active model_version remain on the previous generation")
def previous_generation_still_serving(world: World) -> None:
    first, second = _runs(world)
    previous = first.summary["model_version"]
    assert second.summary["decision"] == "rejected", second.summary
    # The second run registered a version, tagged it rejected and left the champion on the first.
    assert newest_tags(world).get("gate.outcome") == "rejected", newest_tags(world)
    assert int(newest_als(world)["version"]) == 2, registry(world).version_counts()
    assert registry(world).alias(ALS, "champion") == 1
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
    # No champion existed: the version is the first one and became the champion.
    assert registry(world).version_counts().get(ALS) == 1, registry(world).version_counts()
    assert newest_tags(world).get("gate.outcome") == "promoted", newest_tags(world)
    assert registry(world).alias(ALS, "champion") == int(newest_als(world)["version"])
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
    # The comparison itself: which metric and the candidate's value ...
    assert s["primary_metric"] == "ndcg@10"
    assert s["candidate_value"] == s["metrics"]["ndcg@10"]
    # ... and the summary agrees with what the registry recorded for this candidate.
    assert newest_tags(world).get("gate.outcome") == s["decision"], (newest_tags(world), s)
