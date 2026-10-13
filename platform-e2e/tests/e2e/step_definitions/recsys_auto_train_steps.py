"""Steps for recommendations/recsys_auto_train.feature (change recsys-auto-train, all @destructive).

Black box, from the spec only. The real platform-recsys image runs against the stack's live serving
data (Redis DB 0, default Qdrant collections) and MLflow on :5050 (src/api/recsys_job.py runs it
in a throwaway container on the stack network; src/api/mlflow_registry.py talks MLflow REST).

Determinism (D13): every scenario starts with a reset (delete the `recsys-*` registered models,
forget every generation), then seeds explicitly: registry champions through the library's `seed`,
a prior generation g1 published by the job itself, the dataset snapshot written fresh. Pass and
fail are driven by seeding a weak or a strong champion.

Assertions name "the newest version of recsys-<model>": no generation -> version mapping is assumed.
"""

from __future__ import annotations

import os
import re
import time

from pytest_bdd import given, parsers, then, when

from src.api.mlflow_registry import RECSYS_MODELS, STRONG, WEAK, MlflowRegistry
from src.api.recsys_job import (
    RESET,
    compose_chain,
    job_action,
    mlflow_container,
    pointers,
    run_actions,
    serving_state,
    start_mlflow,
    stop_mlflow,
)
from src.api.services.recommendation_service import CONTEXT_HOMEPAGE
from tests.e2e.support.pear_edge_support import DC_WRAPPER_DEFAULT
from tests.e2e.support.world import World

_SETTLE_S = 20.0  # team-ai memoises the serving pointer for ~5 s
_POLL_S = 0.5
_FIXTURE_USER_KEYS = [f"u-{i:04d}" for i in range(1, 51)]
_MODEL = {"als": "recsys-als", "gbdt": "recsys-gbdt", "two-tower": "recsys-two-tower"}


def _ctx(world: World) -> dict:
    return world.state.extra.setdefault("rat", {})


def _registry(world: World) -> MlflowRegistry:
    ctx = _ctx(world)
    if "registry" not in ctx:
        ctx["registry"] = MlflowRegistry()
    return ctx["registry"]


def _buyer_id(world: World) -> str:
    user = world.state.current_user
    return user.user_id if user is not None and user.user_id else ""


def _recommend(world: World) -> dict:
    return world.service_factory.recommendation.recommend(context=CONTEXT_HOMEPAGE, limit=10)


def _ranked_ids(answer: dict) -> list[str]:
    return [i["listingId"] for i in sorted(answer.get("items") or [], key=lambda i: i["rank"])]


def _await_recommend_version(world: World, expected: str, timeout_s: float = _SETTLE_S) -> dict:
    start = time.monotonic()
    while True:
        answer = _recommend(world)
        if answer.get("modelVersion") == expected:
            return answer
        assert time.monotonic() - start < timeout_s, (
            f"gateway Recommend still reports {answer.get('modelVersion')!r} after "
            f"{timeout_s:.0f}s, expected {expected!r}"
        )
        time.sleep(_POLL_S)


# ── Given: reset and explicit seeding ─────────────────────────────────────
@given("the recsys serving state and the recsys registry are reset")
def reset_everything(world: World) -> None:
    ctx = _ctx(world)
    ctx["t0_ms"] = int(time.time() * 1000)
    run_actions([RESET])
    _registry(world).reset_registry()
    state = serving_state()
    assert state["serving"] is None and state["previous"] is None, state
    assert not state["generations"] and not state["collections"], state


@given("a prior generation g1 is published and serving, then the registry is cleared again")
def prior_generation_g1(world: World) -> None:
    ctx = _ctx(world)
    # The job bootstraps g1 (the registry is empty). Publishing registers a version, so the
    # registry is emptied once more: the seeded champion of the scenario is then the only version.
    (run,) = run_actions([job_action("good_a")], buyer_id=_buyer_id(world))
    assert run.exit_code == 0, f"publishing g1 failed ({run.exit_code}):\n{run.log[-2500:]}"
    ctx["g1"] = serving_state()["serving"]
    assert ctx["g1"], "the job published no generation to act as g1"
    _registry(world).reset_registry()
    ctx["before"] = serving_state()
    # Let team-ai's memoised pointer settle on g1, then remember what a buyer is answered.
    ctx["answer_before"] = _await_recommend_version(world, ctx["g1"])


@given(parsers.parse('a {kind} champion is seeded for "{model}"'))
def seed_champion(world: World, kind: str, model: str) -> None:
    metrics = {"weak": WEAK, "strong": STRONG}[kind][model]
    registry = _registry(world)
    registry.seed(model, metrics)
    champion = registry.alias(model, "champion")
    assert champion is not None, f"seeding {model} left no champion alias"
    _ctx(world).setdefault("seeded", {})[model] = champion


@given("weak champions are seeded for recsys-als, recsys-gbdt and recsys-two-tower")
def seed_three_weak(world: World) -> None:
    for model in RECSYS_MODELS:
        seed_champion(world, "weak", model)


@given("the recsys registry is down")
def registry_down(world: World) -> None:
    registry = _registry(world)
    # Registered first so the teardown runs even when stopping or the scenario fails.
    world.add_cleanup(lambda: start_mlflow(registry))
    stop_mlflow()
    assert not registry.healthy(), f"{mlflow_container()} was stopped but /health still answers"


@given("the newest dataset snapshot is 26 hours old")
def stale_snapshot(world: World) -> None:
    _ctx(world)["age_hours"] = 26


@given("a dataset of fifty users keyed u-0001 to u-0050")
def keyed_dataset(world: World) -> None:
    _ctx(world)["dataset"] = "keyed"


# ── When ──────────────────────────────────────────────────────────────────
def _run_job(world: World, **kwargs) -> None:
    ctx = _ctx(world)
    registry = _registry(world)
    ctx["counts_before"] = registry.version_counts() if registry.healthy() else None
    ctx.setdefault("before", serving_state())
    dataset = ctx.get("dataset", "better_b")
    age = ctx.get("age_hours", 0)
    (ctx["result"],) = run_actions(
        [job_action(dataset, age_hours=age, **kwargs)], buyer_id=_buyer_id(world)
    )


@when("the recsys job runs on a new dataset")
@when("the recsys job runs")
def run_job(world: World) -> None:
    _run_job(world)


@when("the recsys job runs with MAX_DATASET_AGE_MINUTES set to 1500")
def run_job_with_age_limit(world: World) -> None:
    _run_job(world, env={"MAX_DATASET_AGE_MINUTES": "1500"})


@when("the featurestore dataset job and then the recsys compose job run")
def run_compose_chain(world: World) -> None:
    ctx = _ctx(world)
    wrapper = os.getenv("DC_WRAPPER", DC_WRAPPER_DEFAULT)
    ctx["chain"] = compose_chain(wrapper)


# ── Then: the job ─────────────────────────────────────────────────────────
@then(parsers.parse("the job exits with code {code:d}"))
def exit_code_is(world: World, code: int) -> None:
    run = _ctx(world)["result"]
    assert run.exit_code == code, f"exit {run.exit_code}, expected {code}:\n{run.log[-2500:]}"


@then(parsers.parse("the job exits with code {code:d} within {seconds:d} seconds"))
def exit_code_within(world: World, code: int, seconds: int) -> None:
    run = _ctx(world)["result"]
    assert run.exit_code == code, f"exit {run.exit_code}, expected {code}:\n{run.log[-2500:]}"
    assert run.duration_s <= seconds, f"the job took {run.duration_s:.1f}s, limit {seconds}s"


@then("the job did not start Spark")
def no_spark(world: World) -> None:
    run = _ctx(world)["result"]
    assert not run.started_spark, f"Spark started:\n{run.log[-2500:]}"


@then("the chain's jobs both exit 0")
def chain_exit_zero(world: World) -> None:
    chain = _ctx(world)["chain"]
    for step in chain:
        assert step.exit_code == 0, f"{step.step} exited {step.exit_code}:\n{step.output[-3000:]}"
    assert [s.step for s in chain] == ["featurestore-dataset", "platform-recsys"], chain


# ── Then: the registry ────────────────────────────────────────────────────
def _newest(world: World, model: str) -> dict:
    version = _registry(world).newest_version(model)
    assert version is not None, f"{model} has no version in MLflow"
    return version


@then(parsers.parse('the newest version of "{model}" is tagged "{tag}" "{value}"'))
def newest_tagged(world: World, model: str, tag: str, value: str) -> None:
    version = _newest(world, model)
    if value in ("promoted", "rejected"):
        # a version the job created, not the seeded champion that was there before the run
        seeded = _ctx(world).get("seeded", {}).get(model)
        assert int(version["version"]) != seeded, (
            f"the job registered no new version of {model}: the newest one (v{seeded}) is the "
            "seeded champion"
        )
    tags = _registry(world).tags(version)
    assert tags.get(tag) == value, f"{model} v{version['version']} tags: {tags}"


@then(parsers.parse('the "gate.reason" of the newest version of "{model}" names "{text}"'))
def newest_reason_names(world: World, model: str, text: str) -> None:
    reason = _registry(world).tags(_newest(world, model)).get("gate.reason", "")
    assert text in reason, f"gate.reason {reason!r} does not name {text!r}"


@then(parsers.parse('the alias "{alias}" of "{model}" names its newest version'))
def alias_is_newest(world: World, alias: str, model: str) -> None:
    newest = int(_newest(world, model)["version"])
    assert _registry(world).alias(model, alias) == newest, (
        alias,
        _registry(world).alias(model, alias),
        newest,
    )


@then(parsers.parse('the alias "{alias}" of "{model}" still names the seeded version'))
def alias_is_seeded(world: World, alias: str, model: str) -> None:
    seeded = _ctx(world)["seeded"][model]
    assert _registry(world).alias(model, alias) == seeded, (
        alias,
        _registry(world).alias(model, alias),
        seeded,
    )


@then("no recsys model has a new version in MLflow")
def no_new_versions(world: World) -> None:
    ctx = _ctx(world)
    assert (
        ctx["counts_before"] is not None
    ), "the registry versions were not recorded before the run"
    now = _registry(world).version_counts()
    assert now == ctx["counts_before"], f"versions before {ctx['counts_before']}, now {now}"


@then("each of recsys-als, recsys-gbdt and recsys-two-tower has a newest version tagged promoted")
def three_promoted(world: World) -> None:
    for model in RECSYS_MODELS:
        newest_tagged(world, model, "gate.outcome", "promoted")


# ── Then: serving ─────────────────────────────────────────────────────────
@then("the serving pointer names a generation other than g1")
def serving_moved(world: World) -> None:
    ctx = _ctx(world)
    serving = pointers()["serving"]
    assert serving and serving != ctx["g1"], f"serving {serving!r}, g1 {ctx['g1']!r}"
    ctx["g2"] = serving


@then("the previous pointer names g1")
def previous_is_g1(world: World) -> None:
    assert pointers()["previous"] == _ctx(world)["g1"], (pointers(), _ctx(world)["g1"])


@then("a buyer's Recommend answers from the new generation")
def recommend_from_new(world: World) -> None:
    ctx = _ctx(world)
    answer = _await_recommend_version(world, ctx["g2"])
    ids = _ranked_ids(answer)
    assert ids, f"Recommend returned no items: {answer}"
    assert all(i.startswith("rgp-item-") for i in ids), f"items outside the new dataset: {ids}"


@then("the serving and previous pointers are unchanged")
def pointers_unchanged(world: World) -> None:
    before = _ctx(world)["before"]
    assert {k: pointers()[k] for k in ("serving", "previous")} == {
        k: before[k] for k in ("serving", "previous")
    }, (before, pointers())


@then("no new generation keys or collections exist")
def no_new_generation(world: World) -> None:
    before, now = _ctx(world)["before"], serving_state()
    assert now["generations"] == before["generations"], (before["generations"], now["generations"])
    assert now["collections"] == before["collections"], (before["collections"], now["collections"])


@then("a buyer's Recommend answers are identical to before the run")
def recommend_identical(world: World) -> None:
    ctx = _ctx(world)
    answer = _recommend(world)
    before = ctx["answer_before"]
    assert answer.get("modelVersion") == before.get("modelVersion"), (answer, before)
    assert _ranked_ids(answer) == _ranked_ids(before), (_ranked_ids(answer), _ranked_ids(before))


# ── PII scan (RAT-25, RAT-49) ─────────────────────────────────────────────
@then(
    "a scan of every run's params, metrics, tags, run name and artifacts finds no fixture user key"
)
def scan_finds_nothing(world: World) -> None:
    registry = _registry(world)
    run_ids = [
        v["run_id"] for model in registry.registered_models() for v in registry.versions(model)
    ]
    run_ids += registry.run_ids_since(_ctx(world)["t0_ms"])
    report = registry.scan_runs(run_ids, _FIXTURE_USER_KEYS)
    # The scan must have looked at the job's runs: params/metrics/tags and real artifact files.
    assert report.runs >= 2, f"the scan only covered {report.runs} run(s)"
    assert report.artifact_files >= 1, "the scan found no artifact file to read"
    assert not report.hits, f"user keys found in MLflow: {report.hits[:10]}"
    _ctx(world)["scan"] = report


@then("no artifact is named like user factors")
def no_user_factor_artifact(world: World) -> None:
    paths = _ctx(world)["scan"].artifact_paths
    bad = [p for p in paths if re.search(r"user", p, re.I)]
    assert not bad, f"artifacts named like user data: {bad}"


@given(parsers.parse('a scratch MLflow run with "{key}" logged as a param and as artifact text'))
def scratch_run(world: World, key: str) -> None:
    registry = _registry(world)
    run_id = registry.create_scratch_run()
    world.add_cleanup(lambda: registry.delete_run(run_id))
    registry.log_param(run_id, "note", key)
    registry.log_artifact_text(run_id, "note.txt", f"planted {key} here")
    _ctx(world)["scratch"] = run_id


@when("the PII scan runs on that run for the fixture user keys")
def scan_scratch(world: World) -> None:
    ctx = _ctx(world)
    ctx["scratch_report"] = _registry(world).scan_runs([ctx["scratch"]], _FIXTURE_USER_KEYS)


@then(parsers.parse('the scan reports "{key}" in a param and in an artifact'))
def scan_reports_planted(world: World, key: str) -> None:
    hits = _ctx(world)["scratch_report"].hits
    wheres = {h.where for h in hits if h.key == key}
    assert "param" in wheres, f"no param hit for {key}: {hits}"
    assert "artifact_content" in wheres, f"no artifact hit for {key}: {hits}"
