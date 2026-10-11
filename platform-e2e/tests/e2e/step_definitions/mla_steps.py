"""Steps for the nearline-CTR scenarios (add-recsys-nearline-signals, wire-debiased-ctr-ranker-features)."""

from __future__ import annotations

from pytest_bdd import then, when

from tests.e2e.support import mla_support as mla
from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra.setdefault("mla", {})


@when(
    "two home-feed candidates have equal model scores and only the second has a nearline CTR in Redis"
)
def mla_only_second_has_nearline(world: World) -> None:
    first, second = mla.listing_ids("nl")
    mla.serve_tied_pair(world, first, second)
    mla.put_ctr(world, second, clicks_ips=8.0, imprs_ips=40.0)
    _x(world).update(pair=(first, second), response=mla.await_pair(world, first, second))


@then("a Recommend call through the gateway ranks the second above the first")
def mla_second_above_first(world: World) -> None:
    first, second = _x(world)["pair"]
    got = rss.ids(_x(world)["response"])
    assert got.index(second) < got.index(first), got


@then("with no nearline row for either, the order is the candidates' own order")
def mla_no_nearline_keeps_order(world: World) -> None:
    first, second = _x(world)["pair"]
    mla.remove_ctr(second)
    got = rss.ids(mla.await_pair(world, first, second))
    assert got.index(first) < got.index(second), got


@when(
    "two candidates have equal model scores and equal raw click-through but the second's clicks came from worse positions"
)
def mla_debiased_differs(world: World) -> None:
    first, second = mla.listing_ids("db")
    # Both: 20 impressions and 2 clicks, so a raw CTR of 10%. The first's events are all at
    # position 1; the second's clicks (and two impressions) were at position 9.
    mla.put_ctr(world, first, *mla.ips_row(impressions=[1] * 20, clicks=[1] * 2))
    mla.put_ctr(world, second, *mla.ips_row(impressions=[1] * 18 + [9] * 2, clicks=[9] * 2))
    mla.serve_tied_pair(world, first, second)
    _x(world).update(pair=(first, second), response=mla.await_pair(world, first, second))


@then("the second ranks above the first in the Recommend response")
def mla_second_ranks_above(world: World) -> None:
    mla_second_above_first(world)
