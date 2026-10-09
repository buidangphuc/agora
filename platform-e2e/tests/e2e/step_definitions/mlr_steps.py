"""Steps for the ML recsys changes (area mlr). Black box: the real recsys job image (mlr_job_flow) and,
for the nearline scenarios, beacons posted through the gateway edge.

Every scenario runs in the worker's own Redis DB / Qdrant namespace; nothing touches serving data.
"""

from __future__ import annotations

import uuid

from pytest_bdd import given, then, when

from src.constants import gateway_endpoints as ep
from tests.e2e.flows.mlr_job_flow import run_plan
from tests.e2e.support.world import World

FEATURES = ("weight", "user_items", "item_users", "top_score")
TWO_TOWER = {"ENABLE_TWO_TOWER": "true"}


def _ctx(world: World) -> dict:
    return world.state.extra.setdefault("mlr", {})


def _steps(world: World, steps: list[dict]) -> list[dict]:
    results = run_plan(steps)
    _ctx(world)["results"] = results
    return results


def _results(world: World) -> list[dict]:
    return _ctx(world)["results"]


def _recsys(world: World, plan: list[dict]) -> None:
    """Run recsys steps; every step must have exited 0 unless the scenario expects a refusal."""
    results = _steps(world, plan)
    _ctx(world)["last"] = results[-1]


def _tower_collection(step: dict) -> str:
    names = [n for n in step["collections"] if "_tower__" in n]
    assert len(names) >= 1, f"no two-tower collection in {sorted(step['collections'])}"
    return next(n for n in names if n.endswith(step["model_version"]))


def _beacon(world: World, event_type: str, listing: str, session: str, category: str) -> None:
    body = {
        "type": event_type,
        "listingId": listing,
        "sessionId": session,
        "itemCategory": category,
        "path": f"/listing/{listing}",
    }
    resp = world.service_factory.tracking.send("POST", ep.TRACK, json_body=body)
    assert resp.status_code == 202, f"beacon refused: {resp.status_code} {resp.text}"


def _user_id(world: World) -> str:
    user = world.state.current_user
    assert user is not None and user.user_id, "no logged-in buyer"
    return user.user_id


# ── nearline ────────────────────────────────────────────────────────────────
@when("the buyer views two listings in one session and the nearline consumer drains the topic")
def view_two_then_drain(world: World) -> None:
    tag = uuid.uuid4().hex[:10]
    ctx = _ctx(world)
    ctx.update(
        a=f"mlr-{tag}-a", b=f"mlr-{tag}-b", cat_a=f"mlr-cat-{tag}-a", cat_b=f"mlr-cat-{tag}-b",
        user=_user_id(world),
    )  # fmt: skip
    session = f"mlr-sess-{tag}"
    _beacon(world, "view", ctx["a"], session, ctx["cat_a"])
    _beacon(world, "view", ctx["b"], session, ctx["cat_b"])
    (result,) = _steps(world, [{"op": "nearline"}])
    assert result["exit_code"] == 0, result["log_tail"]


@then(
    "the nearline layer records the second listing then the first in the buyer's recent items list in Redis"
)
def recents_in_redis(world: World) -> None:
    ctx = _ctx(world)
    entry = _results(world)[0]["nearline"][f"recs:nearline:user:{ctx['user']}:items"]
    assert [m for m, _ in entry["members"]][:2] == [ctx["b"], ctx["a"]], entry
    assert entry["ttl"] > 0


@then("the category affinities of both listings are incremented")
def category_affinities(world: World) -> None:
    ctx = _ctx(world)
    fields = _results(world)[0]["nearline"][f"recs:nearline:user:{ctx['user']}:cats"]["fields"]
    assert float(fields[ctx["cat_a"]]) >= 1.0 and float(fields[ctx["cat_b"]]) >= 1.0, fields


@when(
    "the buyer views the same two listings in two sessions and the nearline consumer drains the topic"
)
def view_twice_then_drain(world: World) -> None:
    tag = uuid.uuid4().hex[:10]
    ctx = _ctx(world)
    ctx.update(a=f"mlr-{tag}-a", b=f"mlr-{tag}-b")
    for n in (1, 2):
        session = f"mlr-sess-{tag}-{n}"
        _beacon(world, "view", ctx["a"], session, "mlr-cat")
        _beacon(world, "view", ctx["b"], session, "mlr-cat")
    (result,) = _steps(world, [{"op": "nearline"}])
    assert result["exit_code"] == 0, result["log_tail"]


@then("the co-view count between the two listings is 2 in Redis")
def coview_count(world: World) -> None:
    ctx = _ctx(world)
    entry = _results(world)[0]["nearline"][f"recs:nearline:coview:{ctx['a']}"]
    assert dict(entry["members"]) == {ctx["b"]: 2.0}, entry


_ISO_KEY = "recs:nearline:user:mlr-iso:items"


@given("nearline keys exist in Redis")
def nearline_keys_exist(world: World) -> None:
    _ctx(world)["plan"] = [
        {"op": "redis_zadd", "key": _ISO_KEY, "member": "mlr-listing", "score": 1790000000.0}
    ]


@when("the recsys job promotes three generations in a row")
def three_promotions(world: World) -> None:
    plan = _ctx(world)["plan"] + [{"op": "recsys"}] * 3
    results = _steps(world, plan)
    assert all(r["exit_code"] == 0 for r in results[1:]), [r["log_tail"] for r in results[1:]]
    assert [r["summary"]["decision"] for r in results[1:]] == ["promoted"] * 3


@then("the nearline keys are unchanged and keep their TTL")
def nearline_untouched(world: World) -> None:
    for step in _results(world)[1:]:
        entry = step["nearline"][_ISO_KEY]
        assert entry["members"] == [["mlr-listing", 1790000000.0]], entry
        assert entry["ttl"] > 0


# ── drift ───────────────────────────────────────────────────────────────────
@when("the recsys job runs with an empty registry")
def run_once(world: World) -> None:
    _recsys(world, [{"op": "recsys"}])
    assert _ctx(world)["last"]["exit_code"] == 0, _ctx(world)["last"]["log_tail"]


@then("the run summary and the model's metadata report the drift status no_baseline")
def first_run_no_baseline(world: World) -> None:
    last = _ctx(world)["last"]
    assert last["summary"]["drift"]["status"] == "no_baseline", last["summary"]
    assert last["models"][last["model_version"]]["parameters"]["drift"]["status"] == "no_baseline"


@then("the model stores its distribution summary for the next run")
def stores_distribution(world: World) -> None:
    last = _ctx(world)["last"]
    dist = last["models"][last["model_version"]]["parameters"]["distribution"]
    assert set(dist) == set(FEATURES) and all(dist[f] for f in FEATURES), dist


def _two_runs(world: World, second_env: dict | None = None, first_env: dict | None = None) -> None:
    results = _steps(
        world, [{"op": "recsys", "env": first_env or {}}, {"op": "recsys", "env": second_env or {}}]
    )
    assert all(r["exit_code"] == 0 for r in results), [r["log_tail"] for r in results]
    _ctx(world)["first"], _ctx(world)["last"] = results


@when("the recsys job runs twice on the same governed dataset")
def run_twice(world: World) -> None:
    _two_runs(world)


@then(
    "the second run's drift names the first run as its baseline and reports a PSI and a level for each feature"
)
def second_run_drift(world: World) -> None:
    first, last = _ctx(world)["first"], _ctx(world)["last"]
    drift = last["summary"]["drift"]
    assert drift["baseline_version"] == first["model_version"], drift
    assert set(drift["features"]) == set(FEATURES)
    assert all("psi" in v and "drift_level" in v for v in drift["features"].values())


@then("none of the features is flagged")
def none_flagged(world: World) -> None:
    drift = _ctx(world)["last"]["summary"]["drift"]
    assert (
        drift["status"] == "ok"
        and drift["is_drifted"] is False
        and drift["num_features_drifted"] == 0
    ), drift


@when("the recsys job runs a second time with DRIFT_ALERT_THRESHOLD set to 0")
def run_second_with_zero_threshold(world: World) -> None:
    _two_runs(world, second_env={"DRIFT_ALERT_THRESHOLD": "0"})


@then(
    "every feature of the second run's drift is flagged and its metadata carries the same verdict"
)
def all_flagged(world: World) -> None:
    last = _ctx(world)["last"]
    drift = last["summary"]["drift"]
    assert drift["is_drifted"] is True and drift["num_features_drifted"] == len(FEATURES), drift
    assert last["models"][last["model_version"]]["parameters"]["drift"] == drift


@then("the second run is promoted")
def second_promoted(world: World) -> None:
    last = _ctx(world)["last"]
    assert last["summary"]["decision"] == "promoted", last["summary"]
    assert last["champion"] == last["model_version"]


_PROM = "/tmp/mlr-drift.prom"


@when("the recsys job runs twice with DRIFT_METRICS_PATH set")
def run_twice_with_prom(world: World) -> None:
    results = _steps(
        world,
        [{"op": "recsys", "env": {"DRIFT_METRICS_PATH": _PROM}},
         {"op": "recsys", "env": {"DRIFT_METRICS_PATH": _PROM}, "read": [_PROM]}],
    )  # fmt: skip
    assert all(r["exit_code"] == 0 for r in results), [r["log_tail"] for r in results]
    _ctx(world)["last"] = results[-1]


@then(
    "the drift metrics file holds a recsys_feature_psi line for each feature and a recsys_model_drift_alert line"
)
def prom_file(world: World) -> None:
    text = _ctx(world)["last"]["files"][_PROM]
    for feature in FEATURES:
        assert f'recsys_feature_psi{{feature="{feature}"' in text, text
    assert "recsys_model_drift_alert " in text


# ── two-tower ───────────────────────────────────────────────────────────────
def _run_tower(
    world: World, runs: int = 1, env: dict | None = None, features: str = "standard"
) -> None:
    step = {"op": "recsys", "env": {**TWO_TOWER, **(env or {})}, "features": features}
    results = _steps(world, [step] * runs)
    assert all(r["exit_code"] == 0 for r in results), [r["log_tail"] for r in results]
    _ctx(world)["last"] = results[-1]


@when("the recsys job runs with the two-tower stage enabled over the feature snapshots")
def run_tower(world: World) -> None:
    _run_tower(world)


@then("the summary reports two_tower_items greater than zero")
def reports_count(world: World) -> None:
    assert _ctx(world)["last"]["summary"]["two_tower_items"] > 0


@then("the generation's two-tower collection holds that many points")
def collection_count(world: World) -> None:
    last = _ctx(world)["last"]
    assert len(last["collections"][_tower_collection(last)]) == last["summary"]["two_tower_items"]


@when("the recsys job runs three times with the two-tower stage enabled")
def run_tower_thrice(world: World) -> None:
    _run_tower(world, runs=3)
    _ctx(world)["all"] = _results(world)


@then("every point of each run's two-tower collection carries that run's model version")
def points_carry_generation(world: World) -> None:
    for step in _ctx(world)["all"]:
        points = step["collections"][_tower_collection(step)]
        assert points and {p["model_version"] for p in points} == {step["model_version"]}


@then("only the serving and previous generations' two-tower collections remain")
def only_two_generations(world: World) -> None:
    steps = _ctx(world)["all"]
    remaining = sorted(n for n in steps[-1]["collections"] if "_tower__" in n)
    assert remaining == sorted(
        n for n in (_tower_collection(steps[1]), _tower_collection(steps[2]))
    ), remaining


@then(
    "the item without interactions has a non-zero two-tower vector that differs from another item's"
)
def cold_vector(world: World) -> None:
    last = _ctx(world)["last"]
    by_item = {p["listing_id"]: p["vector"] for p in last["collections"][_tower_collection(last)]}
    assert any(by_item["mlr-cold"]) and by_item["mlr-cold"] != by_item["la"]


@then("the item without interactions has no ALS vector")
def cold_has_no_als(world: World) -> None:
    last = _ctx(world)["last"]
    als = [n for n in last["collections"] if "_items__" in n]
    assert als, sorted(last["collections"])
    assert all(p["listing_id"] != "mlr-cold" for n in als for p in last["collections"][n])


@then("the summary reports a first-epoch and a last-epoch loss")
def reports_loss(world: World) -> None:
    tower = _ctx(world)["last"]["summary"]["two_tower"]
    assert tower["loss_first"] is not None and tower["loss_last"] is not None, tower


@then("the model's metadata records the pairs, the epochs and the feature snapshots it trained on")
def records_lineage(world: World) -> None:
    last = _ctx(world)["last"]
    meta = last["models"][last["model_version"]]["parameters"]["two_tower"]
    assert meta["pairs"] > 0 and meta["epochs"] > 0, meta
    snap = meta["features"]["items"]
    assert (
        snap["view"] == "item_popularity" and snap["version"] == 1 and len(snap["sha256"]) == 64
    ), snap


@when("the recsys job runs with the two-tower stage enabled and no feature snapshots")
def run_tower_without_snapshots(world: World) -> None:
    results = _steps(world, [{"op": "recsys", "env": TWO_TOWER, "features": "none"}])
    _ctx(world)["last"] = results[-1]


@then("the job exits 2 and its log names ITEM_FEATURES_DIR")
def exits_two(world: World) -> None:
    last = _ctx(world)["last"]
    assert last["exit_code"] == 2 and "ITEM_FEATURES_DIR" in last["log_tail"], last["log_tail"]


@then("no model is registered")
def no_model(world: World) -> None:
    assert _ctx(world)["last"]["models"] == {}


@when(
    "the recsys job runs with untrained towers over a catalogue with an item without any features"
)
def run_untrained(world: World) -> None:
    _run_tower(world, env={"TWO_TOWER_EPOCHS": "0"}, features="with_blank")


@then("the summary counts one refused vector")
def one_refused(world: World) -> None:
    assert _ctx(world)["last"]["summary"]["two_tower"]["refused"] == 1


@then("the two-tower collection has no point for that item")
def no_blank_point(world: World) -> None:
    last = _ctx(world)["last"]
    assert all(p["listing_id"] != "mlr-blank" for p in last["collections"][_tower_collection(last)])


@when("the recsys job runs with the two-tower stage disabled")
def run_tower_disabled(world: World) -> None:
    results = _steps(world, [{"op": "recsys", "features": "none"}])
    assert results[0]["exit_code"] == 0, results[0]["log_tail"]
    _ctx(world)["last"] = results[0]


@then("the summary has no two-tower entries")
def no_tower_summary(world: World) -> None:
    summary = _ctx(world)["last"]["summary"]
    assert "two_tower_items" not in summary and "two_tower" not in summary


@then("no two-tower collection is written")
def no_tower_collection(world: World) -> None:
    assert not [n for n in _ctx(world)["last"]["collections"] if "_tower" in n]
