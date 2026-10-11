"""Steps for serving-switch-atomicity (area ssa-e2e).

Black box: the real platform-recsys image publishes generations (same driver as
recsys-generation-publish), similar items are read through the gateway as a logged-in buyer, and the
serving pointer / aliases are read and written in the stack's real Redis and Qdrant (host-published
ports). Each generation's item collection gets a marker point (listing id ``ssa-marker-<version>``)
carrying the seed listing's own vector, so it is the nearest neighbour and names the collection a
Recommend answer was read from. Pointer and alias edits simulate the states a crashed or lagging
publish leaves; the module teardown republishes a real generation through the production path.
"""

from __future__ import annotations

import json
import os
import subprocess
import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from src.api.services.recommendation_service import CONTEXT_SIMILAR_ITEMS
from tests.e2e.flows.recsys_job_flow import run_recsys_job
from tests.e2e.step_definitions.rgp_steps import _buyer_id, _ctx, _publish, _version
from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World

QDRANT_URL = os.getenv("QDRANT_URL", "http://localhost:6333")
ITEM_ALIAS = "item_als_vectors"
USER_ALIAS = "user_als_vectors"
SEED = "rgp-item-0-0"  # in every fixture dataset of the job driver
# Must equal platform-recsys recsys/load/qdrant.py `_NS` (point id = uuid5(NS, listing id)).
_NS = uuid.UUID("6f7a1e2c-9b3d-4c5a-8e21-0d9f4a2b1c00")
_SETTLE_S = 20.0  # the 5 s pointer memo plus slack
_POLL_S = 0.5


def ssa_point_id(source_id: str) -> str:
    return str(uuid.uuid5(_NS, source_id))


def ssa_marker(version: str) -> str:
    return f"ssa-marker-{version}"


def ssa_item_collection(version: str) -> str:
    return f"{ITEM_ALIAS}__{version}"


def ssa_user_collection(version: str) -> str:
    return f"{USER_ALIAS}__{version}"


def ssa_qdrant(method: str, path: str, body: dict | None = None) -> dict:
    resp = httpx.request(method, f"{QDRANT_URL}{path}", json=body, timeout=30.0)
    assert resp.status_code < 300, f"qdrant {method} {path} -> {resp.status_code}: {resp.text}"
    return resp.json()


def ssa_collections() -> set[str]:
    return {c["name"] for c in ssa_qdrant("GET", "/collections")["result"]["collections"]}


def ssa_move_alias(alias: str, collection: str) -> None:
    ssa_qdrant(
        "POST",
        "/collections/aliases",
        {
            "actions": [
                {"delete_alias": {"alias_name": alias}},
                {"create_alias": {"collection_name": collection, "alias_name": alias}},
            ]
        },
    )


def ssa_delete_aliases() -> None:
    names = [a["alias_name"] for a in ssa_qdrant("GET", "/aliases")["result"]["aliases"]]
    ssa_qdrant(
        "POST",
        "/collections/aliases",
        {"actions": [{"delete_alias": {"alias_name": n}} for n in names]},
    )


def ssa_add_marker(version: str) -> None:
    """A point in the generation's item collection with the seed listing's own vector."""
    collection = ssa_item_collection(version)
    got = ssa_qdrant(
        "POST",
        f"/collections/{collection}/points",
        {"ids": [ssa_point_id(SEED)], "with_vector": True, "with_payload": False},
    )["result"]
    assert got, f"seed {SEED} is not in {collection}"
    ssa_qdrant(
        "PUT",
        f"/collections/{collection}/points?wait=true",
        {
            "points": [
                {
                    "id": ssa_point_id(ssa_marker(version)),
                    "vector": got[0]["vector"],
                    "payload": {"listing_id": ssa_marker(version), "model_version": version},
                }
            ]
        },
    )


def ssa_similar_ids(world: World) -> list[str]:
    data = world.service_factory.recommendation.recommend(
        context=CONTEXT_SIMILAR_ITEMS, seed_listing_id=SEED, limit=10
    )
    items = sorted(data.get("items") or [], key=lambda it: it.get("rank", 0))
    return [it.get("listingId", "") for it in items]


def ssa_await_marker(world: World, expected: str, others: list[str]) -> None:
    """Poll similar items until the expected generation's marker is served; the other generations'
    markers must be absent from that answer."""
    deadline = time.monotonic() + _SETTLE_S
    seen: list[str] = []
    while True:
        seen = ssa_similar_ids(world)
        if ssa_marker(expected) in seen:
            break
        assert time.monotonic() < deadline, (
            f"similar items never contained {ssa_marker(expected)} within {_SETTLE_S:.0f}s; "
            f"last answer: {seen}"
        )
        time.sleep(_POLL_S)
    present = [o for o in others if ssa_marker(o) in seen]
    assert not present, f"answer mixed generations: {seen} (unexpected markers of {present})"


def ssa_redis(world: World) -> rss.Keys:
    return rss.keys(world, rss.SERVING_DB)


# ── Given ─────────────────────────────────────────────────────────────────
@given("two models have been promoted")
def ssa_two_models(world: World) -> None:
    first, second = _publish(world, [{"@fixture": "good_a"}, {"@fixture": "better_b"}], reset=True)
    assert first.summary["decision"] == second.summary["decision"] == "promoted"
    _ctx(world)["before"] = second.state


@given("each generation's item collection holds a marker next to the seed listing")
def ssa_markers(world: World) -> None:
    for which in ("first", "second"):
        ssa_add_marker(_version(world, which))


# ── When ──────────────────────────────────────────────────────────────────
@when("the buyer opens similar items for the seed listing")
def ssa_open_similar(world: World) -> None:
    _ctx(world)[
        "ssa_answer"
    ] = None  # the Then step polls the gateway until the pointer memo settles


@when(
    "the item alias is moved to the second model's collection while `recs:v1:serving` still names the first"
)
def ssa_alias_ahead(world: World) -> None:
    first, second = _version(world, "first"), _version(world, "second")
    redis = ssa_redis(world)
    ssa_move_alias(ITEM_ALIAS, ssa_item_collection(second))
    redis.put(rss.SERVING_KEY, first)  # the publish crashed before the pointer switch


@when(
    "`recs:v1:serving` names the second model while the item alias still points at the first model's collection"
)
def ssa_alias_behind(world: World) -> None:
    first, second = _version(world, "first"), _version(world, "second")
    ssa_redis(world).put(rss.SERVING_KEY, second)
    ssa_move_alias(ITEM_ALIAS, ssa_item_collection(first))


@when("`recs:v1:serving` is absent and the item alias points at the first model's collection")
def ssa_no_pointer(world: World) -> None:
    first = _version(world, "first")
    ssa_redis(world).remove(rss.SERVING_KEY)
    ssa_move_alias(ITEM_ALIAS, ssa_item_collection(first))


@when("a third model is promoted and the aliases are then deleted and a fourth model is promoted")
def ssa_third_then_fourth(world: World) -> None:
    _publish(world, [{"@fixture": "third_c"}], reset=False)
    ssa_delete_aliases()
    # The structural gate and metric gate are not under test here: force the promotion.
    _publish(world, [{"@fixture": "better_b", "PROMOTION_FORCE": "true"}], reset=False)
    assert len(_ctx(world)["versions"]) == 4, _ctx(world)["versions"]


@when("`python -m recsys rollback` runs")
def ssa_rollback(world: World) -> None:
    ctx = _ctx(world)
    (run,) = run_recsys_job(
        [],
        [{"@command": "rollback"}],
        live=True,
        reset=False,
        buyer_id=_buyer_id(world),
        expect_summary=False,
    )
    assert run.exit_code == 0, f"rollback failed:\n{run.log}"
    ctx["state"] = run.state


@when("the running team-ai container's environment is inspected")
def ssa_inspect_team_ai(world: World) -> None:
    names = subprocess.run(
        ["docker", "ps", "--filter", "name=team-ai-svc", "--format", "{{.Names}}"],
        capture_output=True,
        text=True,
        check=True,
        timeout=30,
    ).stdout.split()
    assert names, "no running team-ai-svc container"
    env = json.loads(
        subprocess.run(
            ["docker", "inspect", "-f", "{{json .Config.Env}}", names[0]],
            capture_output=True,
            text=True,
            check=True,
            timeout=30,
        ).stdout
    )
    world.state.extra["ssa_env"] = dict(e.split("=", 1) for e in env if "=" in e)


# ── Then ──────────────────────────────────────────────────────────────────
@then("the answer is read from the second model's collection")
def ssa_answer_second(world: World) -> None:
    ssa_await_marker(world, _version(world, "second"), [_version(world, "first")])


@then(
    parsers.parse("similar items through the gateway are read from the {which} model's collection")
)
def ssa_answer_from(world: World, which: str) -> None:
    other = "second" if which == "first" else "first"
    ssa_await_marker(world, _version(world, which), [_version(world, other)])


@then("the item and user collections of the third and fourth model still exist")
def ssa_third_fourth_exist(world: World) -> None:
    names = ssa_collections()
    for which in ("third", "fourth"):
        version = _ctx(world)["versions"][{"third": 2, "fourth": 3}[which]]
        for collection in (ssa_item_collection(version), ssa_user_collection(version)):
            assert collection in names, f"{collection} was deleted; collections: {sorted(names)}"
    state = _ctx(world)["state"]
    assert state["serving"] == _ctx(world)["versions"][3], state
    assert state["previous"] == _ctx(world)["versions"][2], state


@then("no collection of the first and second model remains")
def ssa_first_second_gone(world: World) -> None:
    names = ssa_collections()
    for version in _ctx(world)["versions"][:2]:
        left = [n for n in names if n.endswith(f"__{version}")]
        assert not left, f"collections of {version} remain: {left}"


@then("none of its POSTGRES values is a listing service credential")
def ssa_no_listing_credentials(world: World) -> None:
    env = world.state.extra["ssa_env"]
    postgres = {k: v for k, v in env.items() if k.startswith("POSTGRES_")}
    assert postgres, f"team-ai has no POSTGRES_* env at all: {sorted(env)}"
    leaked = {k: v for k, v in postgres.items() if "listing" in v.lower()}
    assert not leaked, f"team-ai holds listing's credentials: {sorted(leaked)}"
