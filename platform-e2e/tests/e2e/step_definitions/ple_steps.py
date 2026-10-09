"""Steps for add-placement-engine (area ple).

Black box through the gateway: `placementId` is the only placement field on the wire. The ladder
source is read from what the training job left in Redis (the buyer's cached list and the generation's
popular list) and from the trained Qdrant collection.
"""

from __future__ import annotations

import json
import time

from pytest_bdd import parsers, then, when

from src.api.services.recommendation_service import (
    CONTEXT_HOMEPAGE,
    CONTEXT_SIMILAR_ITEMS,
    RecommendationService,
)
from tests.e2e.step_definitions.rgp_steps import _await_gateway_version, _ctx, _version
from tests.e2e.step_definitions.ssa_steps import ssa_point_id, ssa_qdrant
from tests.e2e.support import rss_support as rss
from tests.e2e.support.tii_support import register_buyer
from tests.e2e.support.world import World

_LIMIT = 5
_SETTLE_S = 20.0


def _ranked(answer: dict) -> list[str]:
    return [i["listingId"] for i in sorted(answer.get("items") or [], key=lambda i: i["rank"])]


# ── similar_items ─────────────────────────────────────────────────────────
@when(
    parsers.parse("a client queries similar_items with that listing as seed_listing_id"),
)
def query_similar_items(world: World) -> None:
    ctx = world.state.extra["tpr_seed"]
    service = world.service_factory.recommendation
    neighbours = ssa_qdrant(
        "POST",
        f"/collections/{ctx['collection']}/points/query",
        {"query": ssa_point_id(ctx["seed"]), "limit": 20, "with_payload": True},
    )["result"]["points"]
    ctx["expected"] = [p["payload"]["listing_id"] for p in neighbours]
    answer: dict = {}
    for _ in range(10):  # the first call after idle can miss team-ai's 15 ms retrieval budget
        answer = service.recommend(
            seed_listing_id=ctx["seed"], context=CONTEXT_SIMILAR_ITEMS, limit=_LIMIT
        )
        if _ranked(answer) and set(_ranked(answer)) <= set(ctx["expected"]):
            break
        time.sleep(0.3)
    ctx["answer"] = answer
    # A seed no generation knows: retrieval is empty, so the ladder falls to the global popular items.
    ctx["unknown_answer"] = service.recommend(
        seed_listing_id="e2e-ple-unknown-seed", context=CONTEXT_SIMILAR_ITEMS, limit=_LIMIT
    )
    popular = rss.keys(world, rss.SERVING_DB)
    raw = popular.get(f"{rss.serving_prefix(popular)}:popular")
    ctx["popular"] = [row["listing_id"] for row in json.loads(raw or "[]")]


@then(
    "the answer is stamped with the similar_items placement and holds the vector neighbours of the seed"
)
def similar_items_from_vectors(world: World) -> None:
    ctx = world.state.extra["tpr_seed"]
    assert ctx["answer"].get("placementId") == "similar_items", ctx["answer"]
    assert ctx["expected"], "Qdrant knows no neighbour of the seed point"
    got = _ranked(ctx["answer"])
    # team-ai re-orders the retrieved neighbours with its own ranking: a subset of the nearest ones.
    assert got and set(got) <= set(ctx["expected"]), (got, ctx["expected"])


@then("a seed the trained collection does not know is answered with the global popular items")
def unknown_seed_falls_back(world: World) -> None:
    ctx = world.state.extra["tpr_seed"]
    answer = ctx["unknown_answer"]
    ids = _ranked(answer)
    assert answer.get("placementId") == "similar_items", answer
    assert ids, "a sparse seed was answered with nothing"
    assert ctx["popular"], "the serving generation holds no popular list"
    assert set(ids) <= set(ctx["popular"]), (ids, ctx["popular"][:10])


# ── home_feed (destructive) ───────────────────────────────────────────────
def _home(service: RecommendationService) -> dict:
    return service.recommend(context=CONTEXT_HOMEPAGE, limit=_LIMIT)


@when("a user the model trained on queries the home_feed placement")
def trained_user_home(world: World) -> None:
    version = _version(world, "first")
    _await_gateway_version(world, version, _SETTLE_S)
    ctx = _ctx(world)
    ctx["trained_answer"] = _home(world.service_factory.recommendation)
    redis = rss.keys(world, rss.SERVING_DB)
    prefix = f"recs:v1:gen:{version}"
    buyer = rss.buyer_id(world)
    ctx["buyer_list"] = [
        row["listing_id"] for row in json.loads(redis.get(f"{prefix}:user:{buyer}") or "[]")
    ]
    ctx["popular"] = [
        row["listing_id"] for row in json.loads(redis.get(f"{prefix}:popular") or "[]")
    ]


@when("a user the model never saw queries the home_feed placement")
def unknown_user_home(world: World) -> None:
    token, _ = register_buyer()
    service = RecommendationService(token=token)
    try:
        _ctx(world)["cold_answer"] = _home(service)
    finally:
        service.close()


@then("the trained user's answer is stamped home_feed and drawn from that user's personalized list")
def trained_user_personalized(world: World) -> None:
    ctx = _ctx(world)
    answer = ctx["trained_answer"]
    ids = _ranked(answer)
    assert answer.get("placementId") == "home_feed", answer
    assert ctx["buyer_list"], "the training job left no personalized list for the buyer"
    assert ids and set(ids) <= set(ctx["buyer_list"]), (ids, ctx["buyer_list"])
    assert answer.get("modelVersion") == _version(world, "first"), answer


@then("the unknown user's answer is stamped home_feed and drawn from the global popular items")
def unknown_user_popular(world: World) -> None:
    ctx = _ctx(world)
    answer = ctx["cold_answer"]
    ids = _ranked(answer)
    assert answer.get("placementId") == "home_feed", answer
    assert ids and ctx["popular"], (answer, ctx["popular"])
    assert set(ids) <= set(ctx["popular"]), (ids, ctx["popular"])
