"""Steps for recommendations/gbl_ranker_serving.feature (recsys-gbdt-trainer)."""

from __future__ import annotations

from pytest_bdd import then, when

from tests.e2e.support import gbl_support as gbl
from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra.setdefault("gbl", {})


@when(
    "the serving generation carries a ranker artifact that scores the second of two home-feed candidates higher, and the candidates' online features exist"
)
def gbl_publish_artifact(world: World) -> None:
    first, second, serving = gbl.seed(world)
    _x(world).update(pair=(first, second))
    # Without an artifact the fixed weights keep the candidates' own order (similarity decides).
    _x(world)["fixed"] = rss.ids(
        rss.await_recommend(
            world,
            lambda r: rss.ids(r)[:2] == [first, second],
            "Recommend does not return the seeded pair in its own order",
        )
    )
    serving.put(gbl.ranker_key(serving), gbl.artifact())
    # team-ai re-checks an absent artifact within seconds, so the published one is picked up.
    _x(world)["response"] = rss.await_recommend(
        world,
        lambda r: rss.ids(r)[:2] == [second, first],
        "Recommend does not rank the dearer candidate first with the artifact published",
    )


@then("a Recommend call through the gateway ranks the second above the first")
def gbl_second_above_first(world: World) -> None:
    first, second = _x(world)["pair"]
    got = rss.ids(_x(world)["response"])
    assert got.index(second) < got.index(first), got


@then("without the artifact the same candidates were ranked in their own order")
def gbl_fixed_order(world: World) -> None:
    first, second = _x(world)["pair"]
    assert _x(world)["fixed"][:2] == [first, second], _x(world)["fixed"]
