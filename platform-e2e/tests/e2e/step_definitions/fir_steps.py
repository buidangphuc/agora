"""Steps for featurestore-item-attributes on the recsys side (area fir). Black box: the real
platform-recsys job image (fir_job_flow), one isolated namespace per worker."""

from __future__ import annotations

import re

from pytest_bdd import then, when

from tests.e2e.flows.fir_job_flow import run_plan
from tests.e2e.support.world import World

COLD = ("fir-cold-a", "fir-cold-b")


def _ctx(world: World) -> dict:
    return world.state.extra.setdefault("fir", {})


def _run(world: World, features: str, env: dict | None = None) -> dict:
    (step,) = run_plan([{"features": features, "env": env or {}}])
    _ctx(world)["last"] = step
    return step


def _tower(step: dict) -> list[dict]:
    names = [
        n for n in step["collections"] if "_tower__" in n and n.endswith(step["model_version"])
    ]
    assert names, f"no two-tower collection in {sorted(step['collections'])}\n{step['log_tail']}"
    return step["collections"][names[0]]


def _tower_params(step: dict) -> dict:
    return step["models"][step["model_version"]]["parameters"]["two_tower"]


@when(
    "the recsys job runs with the two-tower stage enabled over attribute snapshots that hold two "
    "listings with no engagement and no ALS factor"
)
def run_cold(world: World) -> None:
    step = _run(world, "attrs")
    assert step["exit_code"] == 0, step["log_tail"]


@then("both cold listings have a non-zero two-tower vector and the vectors differ")
def cold_vectors(world: World) -> None:
    step = _ctx(world)["last"]
    by_item = {p["listing_id"]: p["vector"] for p in _tower(step)}
    assert all(c in by_item and any(by_item[c]) for c in COLD), sorted(by_item)
    assert by_item[COLD[0]] != by_item[COLD[1]]
    als = [n for n in step["collections"] if "_items__" in n]
    assert als and all(p["listing_id"] not in COLD for n in als for p in step["collections"][n])


@when(
    "the recsys job runs with the two-tower stage enabled over attribute and preference snapshots"
)
def run_attrs(world: World) -> None:
    step = _run(world, "attrs")
    assert step["exit_code"] == 0, step["log_tail"]


@then(
    "the model's metadata records item_attributes@v1 and user_preferences@v1 with their file and "
    "SHA-256 and the category vocabulary size"
)
def records_lineage(world: World) -> None:
    feats = _tower_params(_ctx(world)["last"])["features"]
    for key, view in (("attributes", "item_attributes"), ("preferences", "user_preferences")):
        snap = feats[key]
        assert snap["view"] == view and snap["version"] == 1, snap
        assert snap["snapshot"].endswith(".parquet") and re.fullmatch(
            r"[0-9a-f]{64}", snap["sha256"]
        ), snap
    assert feats["category_vocab"] == 2, feats


@when(
    "the recsys job runs with the two-tower stage enabled, TWO_TOWER_REQUIRE_ATTRIBUTES true and no "
    "attribute snapshots"
)
def run_required_missing(world: World) -> None:
    _run(world, "no_attrs", {"TWO_TOWER_REQUIRE_ATTRIBUTES": "true"})


@then("the job exits 2, its log names ITEM_ATTRIBUTES_DIR, and no model is registered")
def exits_two(world: World) -> None:
    step = _ctx(world)["last"]
    assert step["exit_code"] == 2 and "ITEM_ATTRIBUTES_DIR" in step["log_tail"], step["log_tail"]
    assert step["models"] == {}


@when(
    "the recsys job runs with the two-tower stage enabled over popularity and activity snapshots only"
)
def run_without_attributes(world: World) -> None:
    _run(world, "no_attrs")


@then("the job exits 0 and the model's metadata records no attribute snapshot")
def no_attribute_lineage(world: World) -> None:
    step = _ctx(world)["last"]
    assert step["exit_code"] == 0, step["log_tail"]
    feats = _tower_params(step)["features"]
    assert feats["attributes"] is None and feats["preferences"] is None, feats
