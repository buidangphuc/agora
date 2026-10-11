"""Steps for recsys-gbdt-trainer (area rgt). Black box: the real platform-recsys job image (rgt_job_flow),
one isolated Redis DB and Qdrant namespace per worker; nothing touches serving data.

`_score` is the reader's algorithm written from the spec (design D7), independent of the image's own scorer.
"""

from __future__ import annotations

import math

from pytest_bdd import then, when

from tests.e2e.flows.rgt_job_flow import run_plan
from tests.e2e.support.world import World

FEATURES = [
    "item_popularity.views_7d",
    "item_popularity.clicks_7d",
    "item_popularity.add_to_cart_7d",
    "item_popularity.favorites_current",
    "item_popularity.review_count",
    "item_popularity.avg_rating",
    "item_popularity.ctr_7d",
    "item_attributes.price",
]
BASE = [100.0, 5.0, 1.0, 1.0, 3.0, 4.0, 0.05, 0.0]
CHEAP, DEAR = [*BASE[:-1], 20_000.0], [*BASE[:-1], 480_000.0]
PROTOCOL = "temporal-lists-v1"


def _ctx(world: World) -> dict:
    return world.state.extra.setdefault("rgt", {})


def _run(world: World, steps: list[dict]) -> list[dict]:
    results = run_plan(steps)
    _ctx(world)["steps"], _ctx(world)["last"] = results, results[-1]
    return results


def _ok(results: list[dict]) -> None:
    assert all(r["exit_code"] == 0 for r in results), [r["log_tail"] for r in results]


def _artifact(step: dict) -> dict:
    key = f"recs:v1:gen:{step['serving']}:ranker"
    assert key in step["rankers"], f"no {key} in {sorted(step['rankers'])}\n{step['log_tail']}"
    return step["rankers"][key]["doc"]


def _score(artifact: dict, x: list[float]) -> float:
    """base_score + learning_rate * sum of the leaf each tree sends x to (feature -1 marks a leaf)."""
    total = 0.0
    for tree in artifact["trees"]:
        node = 0
        while tree["feature"][node] >= 0:
            f = tree["feature"][node]
            node = tree["left"][node] if x[f] <= tree["threshold"][node] else tree["right"][node]
        total += tree["value"][node]
    return artifact["base_score"] + artifact["learning_rate"] * total


def _gbdt_meta(step: dict) -> dict:
    return step["models"][f"gbdt-{step['model_version']}"]


# ── the trainer and its gate ────────────────────────────────────────────────
@when(
    "the recsys job runs with the GBDT stage enabled over a ranking dataset whose clicks depend on item price"
)
def run_trainer(world: World) -> None:
    _ok(_run(world, [{}]))


@then(
    "the run summary reports the GBDT candidate as promoted with an ndcg@10 greater than the "
    "baseline_ndcg@10 of the fixed-weight ranker"
)
def promoted_and_better(world: World) -> None:
    gbdt = _ctx(world)["last"]["summary"]["gbdt"]
    m = gbdt["metrics"]
    assert gbdt["decision"] == "promoted", gbdt
    assert m["ndcg@10"] > m["baseline_ndcg@10"] * 1.05, m


@when("the GBDT stage publishes its artifact")
def publishes(world: World) -> None:
    _ok(_run(world, [{}]))


@then(
    "the artifact lists the features item_popularity.views_7d, item_popularity.clicks_7d, "
    "item_popularity.add_to_cart_7d, item_popularity.favorites_current, item_popularity.review_count, "
    "item_popularity.avg_rating, item_popularity.ctr_7d and item_attributes.price, in that order, with the "
    "view versions"
)
def feature_list(world: World) -> None:
    art = _artifact(_ctx(world)["last"])
    assert art["format"] == "agora-gbdt/1" and art["features"] == FEATURES, art["features"]
    assert art["feature_views"] == {"item_popularity": 1, "item_attributes": 1}
    assert art["ctr_feature"] == "item_popularity.ctr_7d" and set(art["defaults"]) == set(FEATURES)


@when("the GBDT stage runs with a path set for the training rows")
def runs_with_rows(world: World) -> None:
    _ok(_run(world, [{"rows": True, "env": {"GBDT_CTR_MIN_IMPRESSIONS": "100"}}]))


@then(
    "every row records a ctr_source of debiased or fallback, both occur, and the model's metadata counts each"
)
def ctr_sources(world: World) -> None:
    last = _ctx(world)["last"]
    rows = last["rows"]
    sources = [r["ctr_source"] for r in rows]
    assert rows and set(sources) == {"debiased", "fallback"}, {
        s: sources.count(s) for s in set(sources)
    }
    counts = _gbdt_meta(last)["parameters"]["ctr_source_counts"]
    assert counts == {
        "debiased": sources.count("debiased"),
        "fallback": sources.count("fallback"),
    }, counts
    assert all(f in rows[0] for f in FEATURES)


@when("the GBDT stage evaluates its candidate")
def evaluates(world: World) -> None:
    _ok(_run(world, [{}]))


@then(
    "the model's metadata records an evaluation protocol, a cutoff, and that every held-out impression is "
    "later than every training impression"
)
def holdout(world: World) -> None:
    meta = _gbdt_meta(_ctx(world)["last"])
    assert meta["metrics"]["eval_protocol"] == PROTOCOL, meta["metrics"]
    h = meta["parameters"]["holdout"]
    assert (
        h["cutoff"]
        and h["train_last_at"] < h["test_first_at"]
        and h["cutoff"] == h["test_first_at"]
    ), h
    assert h["train_lists"] > 0 and h["test_lists"] > 0


# ── refusals and the disabled path ──────────────────────────────────────────
@when("the recsys job runs with the GBDT stage enabled and no ranking dataset")
def no_dataset(world: World) -> None:
    _run(world, [{"features": "no_rank_dataset"}])


@then("the job exits 2, its log names RANK_DATASET_DIR, and no model is registered")
def exits_two(world: World) -> None:
    last = _ctx(world)["last"]
    assert last["exit_code"] == 2 and "RANK_DATASET_DIR" in last["log_tail"], last["log_tail"]
    assert last["models"] == {}


@when("the recsys job runs without enabling the GBDT stage")
def disabled(world: World) -> None:
    _ok(_run(world, [{"env": {"ENABLE_GBDT": None}}]))


@then("the summary has no GBDT entry and the generation has no ranker artifact")
def no_gbdt(world: World) -> None:
    last = _ctx(world)["last"]
    assert "gbdt" not in last["summary"] and last["rankers"] == {}, last["summary"]
    assert not [v for v in last["models"] if v.startswith("gbdt-")]


# ── the artifact is the model ───────────────────────────────────────────────
@when(
    "an independent evaluator reads the artifact from recs:v1:gen:<generation>:ranker and scores a cheap "
    "and an expensive item"
)
def scores(world: World) -> None:
    _ok(_run(world, [{"score": [CHEAP, DEAR]}]))


@then("its scores equal the trainer's to 1e-9 and the cheap item scores higher")
def scores_agree(world: World) -> None:
    last = _ctx(world)["last"]
    art = _artifact(last)
    mine = [_score(art, CHEAP), _score(art, DEAR)]
    for got, want in zip(mine, last["reference_scores"], strict=True):
        assert math.isclose(got, want, rel_tol=1e-9, abs_tol=1e-9), (mine, last["reference_scores"])
    assert mine[0] > mine[1], mine


# ── the gate ────────────────────────────────────────────────────────────────
@when(
    "the recsys job runs with the GBDT stage enabled and a promotion improvement threshold no model can reach"
)
def unreachable_threshold(world: World) -> None:
    _ok(_run(world, [{"env": {"PROMOTION_MIN_RELATIVE_IMPROVEMENT": "10"}}]))


@then(
    "the GBDT candidate is recorded as rejected with its reason, the generation is serving, and no ranker "
    "artifact exists for it"
)
def rejected(world: World) -> None:
    last = _ctx(world)["last"]
    meta = _gbdt_meta(last)
    assert meta["status"] == "rejected" and "baseline" in meta["parameters"]["gate_reason"], meta
    assert last["summary"]["gbdt"]["decision"] == "rejected"
    assert last["serving"] == last["model_version"] and last["summary"]["decision"] == "promoted"
    assert last["rankers"] == {} and last["gbdt_champion"] is None


# ── generations ─────────────────────────────────────────────────────────────
@when("the recsys job promotes three generations in a row with the GBDT stage enabled")
def three_generations(world: World) -> None:
    _ok(_run(world, [{"env": {"PROMOTION_FORCE": "true"}}] * 3))


@then(
    "a ranker artifact with a TTL exists for the serving and previous generations and none for the oldest"
)
def retention(world: World) -> None:
    first, second, third = _ctx(world)["steps"]
    keys = {k: v["ttl"] for k, v in third["rankers"].items()}
    assert sorted(keys) == sorted(
        f"recs:v1:gen:{g['model_version']}:ranker" for g in (second, third)
    ), keys
    assert all(ttl > 0 for ttl in keys.values()), keys
    assert f"recs:v1:gen:{first['model_version']}:ranker" not in keys
    assert (
        third["serving"] == third["model_version"] and third["previous"] == second["model_version"]
    )
