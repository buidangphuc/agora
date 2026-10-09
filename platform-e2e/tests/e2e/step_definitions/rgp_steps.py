"""Steps for recsys-generation-publish (area rgp-e2e). Every scenario is @destructive.

Black box: the real platform-recsys image runs on governed dataset fixtures that include the
scenario's buyer (recsys_job_flow, live mode: the stack's serving Redis DB 0, Qdrant aliases and the
registry), and recommendations are read through the gateway as that buyer
(RecommendationService/Recommend, `modelVersion`). Redis pointers, generation keys, Qdrant aliases
and collections and the registry come from the job driver's snapshot after each run.

Scenarios rewrite what every other recommendation scenario reads. The binder restores the serving
state once, after the module, by re-running the "good A" publish from a clean slate.
"""

from __future__ import annotations

import re
import time

from pytest_bdd import given, parsers, then, when

from src.api.services.recommendation_service import CONTEXT_HOMEPAGE
from tests.e2e.flows.recsys_job_flow import JobRun, run_recsys_job
from tests.e2e.support.world import World

ITEM_ALIAS = "item_als_vectors"
_ORDINAL = {"first": 0, "second": 1, "third": 2}
_FOLLOW_S = 10.0
_SETTLE_S = 20.0  # the 5 s pointer memo plus slack, for scenarios that are not about latency
_POLL_S = 0.5


def _ctx(world: World) -> dict:
    return world.state.extra.setdefault("rgp", {"versions": [], "state": None, "before": None})


def _buyer_id(world: World) -> str:
    user = world.state.current_user
    assert user is not None and user.user_id, "no logged-in buyer to put in the dataset fixtures"
    return user.user_id


def _publish(world: World, runs: list[dict], *, reset: bool) -> list[JobRun]:
    """Run the job live over fixture variants and record what each run left behind."""
    ctx = _ctx(world)
    result = run_recsys_job(
        [], runs, live=True, reset=reset, buyer_id=_buyer_id(world), expect_summary=True
    )
    for run in result:
        assert run.exit_code == 0, f"the job signalled failure: {run.summary}\n{run.log}"
    if reset:
        ctx["versions"] = []
    for run in result:
        if run.summary["decision"] == "promoted":
            ctx["versions"].append(run.summary["model_version"])
    ctx["state"] = result[-1].state
    ctx["runs"] = result
    return result


def _version(world: World, which: str) -> str:
    versions = _ctx(world)["versions"]
    return versions[_ORDINAL[which]]


def _gateway_version(world: World) -> str:
    data = world.service_factory.recommendation.recommend(context=CONTEXT_HOMEPAGE, limit=10)
    return data.get("modelVersion", "")


def _await_gateway_version(world: World, expected: str, timeout_s: float) -> float:
    """Seconds until the buyer's Recommend through the gateway reports `expected`."""
    start = time.monotonic()
    seen = ""
    while True:
        seen = _gateway_version(world)
        elapsed = time.monotonic() - start
        if seen == expected:
            return elapsed
        assert (
            elapsed < timeout_s
        ), f"gateway Recommend still reports {seen!r} after {elapsed:.1f}s, expected {expected!r}"
        time.sleep(_POLL_S)


def _pointers(state: dict) -> dict:
    return {
        "serving": state["serving"],
        "previous": state["previous"],
        "alias": state["aliases"].get(ITEM_ALIAS),
    }


# ── Given ─────────────────────────────────────────────────────────────────
@given("a model has been promoted on the good dataset")
@given("only one generation was ever promoted")
def rgp_one_model_promoted(world: World) -> None:
    (run,) = _publish(world, [{"@fixture": "good_a"}], reset=True)
    assert run.state["serving"] == run.summary["model_version"], run.state
    _ctx(world)["before"] = run.state


@given("a model has been promoted on the good dataset and the gateway reports it")
def rgp_one_model_promoted_and_visible(world: World) -> None:
    rgp_one_model_promoted(world)
    # The pointer is memoised by team-ai: let it hold the first model before the next promotion.
    _await_gateway_version(world, _version(world, "first"), _SETTLE_S)


@given("two models have been promoted")
@given("two models were promoted")
def rgp_two_models_promoted(world: World) -> None:
    first, second = _publish(world, [{"@fixture": "good_a"}, {"@fixture": "better_b"}], reset=True)
    assert first.summary["decision"] == second.summary["decision"] == "promoted"
    _ctx(world)["before"] = second.state


# ── When ──────────────────────────────────────────────────────────────────
@when(
    "the recsys job promotes a model on one dataset and then promotes a better model on a second dataset"
)
def rgp_promote_two(world: World) -> None:
    rgp_two_models_promoted(world)


@when("a third model is promoted after the two above")
def rgp_promote_third(world: World) -> None:
    _publish(world, [{"@fixture": "third_c"}], reset=False)


@when("a new model is promoted")
def rgp_promote_new(world: World) -> None:
    _publish(world, [{"@fixture": "better_b"}], reset=False)
    _ctx(world)["promoted_at"] = time.monotonic()


@when("the recsys job runs on a dataset whose candidate fails the promotion gate")
def rgp_run_regressing(world: World) -> None:
    (run,) = _publish(world, [{"@fixture": "regressing"}], reset=False)
    assert run.summary["decision"] == "rejected", run.summary
    assert "qdrant" not in run.summary and "cache" not in run.summary, run.summary


@when(
    "the recsys job runs on a dataset in which every user interacted with the same three items only"
)
def rgp_run_one_size(world: World) -> None:
    (run,) = _publish(world, [{"@fixture": "one_size"}], reset=False)
    _ctx(world)["candidate"] = run


@when("`python -m recsys rollback` runs")
def rgp_run_rollback(world: World) -> None:
    ctx = _ctx(world)
    (run,) = run_recsys_job(
        [],
        [{"@command": "rollback"}],
        live=True,
        reset=False,
        buyer_id=_buyer_id(world),
        expect_summary=False,
    )
    ctx["state"] = run.state
    ctx["rollback"] = run


# ── Then ──────────────────────────────────────────────────────────────────
@then(parsers.parse("`recs:v1:serving` is the {which} model"))
def rgp_serving_is(world: World, which: str) -> None:
    state = _ctx(world)["state"]
    assert state["serving"] == _version(world, which), (state["serving"], _ctx(world)["versions"])
    # recs:v1:model_version mirrors serving, for older readers.
    assert state["serving_model_version"] == state["serving"], state


@then(parsers.parse("`recs:v1:previous` is the {which}"))
def rgp_previous_is(world: World, which: str) -> None:
    state = _ctx(world)["state"]
    assert state["previous"] == _version(world, which), (state["previous"], _ctx(world)["versions"])


@then("the alias points at the second model's collection")
def rgp_alias_points_at_second(world: World) -> None:
    state = _ctx(world)["state"]
    collection = f"{ITEM_ALIAS}__{_version(world, 'second')}"
    assert state["aliases"].get(ITEM_ALIAS) == collection, state["aliases"]
    assert collection in state["collections"], state["collections"]


@then(
    parsers.parse(
        "a buyer's recommendations through the gateway report the {which} model's version"
    )
)
def rgp_gateway_reports(world: World, which: str) -> None:
    _await_gateway_version(world, _version(world, which), _SETTLE_S)


@then(
    "within 10 seconds a buyer's recommendations through the gateway report the new model's version"
)
def rgp_gateway_follows_within_10s(world: World) -> None:
    ctx = _ctx(world)
    new = ctx["versions"][-1]
    assert new != ctx["versions"][0], ctx["versions"]
    # The clock started when the job finished publishing, not when this step began.
    budget = _FOLLOW_S - (time.monotonic() - ctx["promoted_at"])
    _await_gateway_version(world, new, max(budget, _POLL_S))


@then("no key or collection of the first model remains")
def rgp_first_model_gone(world: World) -> None:
    state = _ctx(world)["state"]
    first = _version(world, "first")
    assert first not in state["gen_keys"], state["gen_keys"]
    assert not [c for c in state["collections"] if c.endswith(f"__{first}")], state["collections"]
    assert set(state["gen_keys"]) <= {_version(world, "second"), _version(world, "third")}


@then("`recs:v1:serving`, `recs:v1:previous` and the alias are the same as before the run")
def rgp_pointers_unchanged(world: World) -> None:
    ctx = _ctx(world)
    assert _pointers(ctx["state"]) == _pointers(ctx["before"]), (ctx["before"], ctx["state"])


@then(
    "the candidate is registered as `rejected` with a reason naming the list overlap or item coverage check"
)
def rgp_one_size_rejected(world: World) -> None:
    run: JobRun = _ctx(world)["candidate"]
    version = run.summary["model_version"]
    assert run.summary["decision"] == "rejected", run.summary
    recorded = run.state["models"].get(version)
    assert recorded is not None, f"{version} not in registry: {run.state['models']}"
    assert recorded["status"] == "rejected", recorded
    reasons = [
        run.summary.get("reason") or "",
        (recorded["metrics"] or {}).get("gate_reason") or "",
    ]
    assert all(re.search(r"overlap|coverage", r, re.I) for r in reasons), reasons
    assert "qdrant" not in run.summary and "cache" not in run.summary, run.summary


@then("the registry champion is the first")
def rgp_champion_is_first(world: World) -> None:
    state = _ctx(world)["state"]
    first, second = _version(world, "first"), _version(world, "second")
    assert state["champion"] == first, state["champion"]
    assert state["models"][first]["status"] == "champion", state["models"][first]
    assert state["models"][second]["status"] == "archived", state["models"][second]


@then("it exits non-zero")
def rgp_exits_nonzero(world: World) -> None:
    run: JobRun = _ctx(world)["rollback"]
    assert run.exit_code != 0, f"rollback exited 0:\n{run.log}"
