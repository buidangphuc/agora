"""Helpers for the nearline-CTR scenarios (area mla-e2e).

The platform-recsys nearline consumer keeps `recs:nearline:ctr:<listing_id>` as a HASH of the
inverse-propensity weighted sums `clicks_ips` and `imprs_ips` (contract: openspec change
add-recsys-nearline-signals, design.md). These scenarios write that hash exactly as the
consumer would and read the result through the gateway, so they check the team-ai side of the
contract. Keys use unique listing ids and are deleted in teardown.
"""

from __future__ import annotations

import json
import uuid

from tests.e2e.flows.fsm_job_flow import Online
from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World

NEARLINE_DB = rss.SERVING_DB  # the stack's team-ai Redis DB (RECS_NEARLINE_REDIS_URL points here)
NEARLINE_PREFIX = "recs:nearline"
ROW_TTL_S = 300
GAMMA = 0.5  # the consumer's IPS exponent: weight = position ** GAMMA


def weight(position: int) -> float:
    return float(max(1, position)) ** GAMMA


def ips_row(impressions: list[int], clicks: list[int]) -> tuple[float, float]:
    """`(clicks_ips, imprs_ips)` the consumer accumulates for these event positions.

    A click adds its weight to both sums (the consumer counts a click as an impression too).
    """
    clicks_ips = sum(weight(p) for p in clicks)
    imprs_ips = sum(weight(p) for p in impressions) + clicks_ips
    return clicks_ips, imprs_ips


def listing_ids(tag: str, count: int = 2) -> list[str]:
    run = uuid.uuid4().hex[:8]
    return [f"e2e-mla-{tag}-{run}-{n}" for n in range(count)]


def put_ctr(world: World, listing_id: str, clicks_ips: float, imprs_ips: float) -> None:
    """Write one nearline CTR row; it is deleted in teardown (and expires on its own)."""
    key = f"{NEARLINE_PREFIX}:ctr:{listing_id}"
    redis = Online()
    try:
        redis.call("SELECT", str(NEARLINE_DB))
        redis.call("HSET", key, "clicks_ips", repr(clicks_ips), "imprs_ips", repr(imprs_ips))
        redis.call("EXPIRE", key, str(ROW_TTL_S))
    finally:
        redis.close()
    world.add_cleanup(lambda: remove_ctr(listing_id))


def remove_ctr(listing_id: str) -> None:
    redis = Online()
    try:
        redis.call("SELECT", str(NEARLINE_DB))
        redis.call("DEL", f"{NEARLINE_PREFIX}:ctr:{listing_id}")
    finally:
        redis.close()


def serve_tied_pair(world: World, first: str, second: str) -> None:
    """Make the buyer's precomputed home-feed list the two candidates with equal model scores."""
    serving = rss.keys(world, rss.SERVING_DB)
    prefix = rss.serving_prefix(serving)
    serving.put(
        f"{prefix}:user:{rss.buyer_id(world)}",
        json.dumps([{"listing_id": first, "score": 0.5}, {"listing_id": second, "score": 0.5}]),
    )


def await_pair(world: World, first: str, second: str) -> dict:
    return rss.await_recommend(
        world,
        lambda r: set(rss.ids(r)) >= {first, second},
        "Recommend does not return the seeded pair",
    )
