"""Seller funnel scoping (fix/funnel-seller-scope).

Two sellers; tracking beacons are sent through the gateway for seller A's listing
only; each seller then reads GetSellerFunnel through the gateway. The listing ->
seller mapping reaches team-analytics asynchronously (listing.events), so the
positive assertion polls; the negative one runs after it, once the events have
demonstrably been attributed.
"""

from __future__ import annotations

import time
import uuid

from pytest_bdd import given, parsers, then, when

from src.api.services.tracking_service import VIEW
from src.models import Listing
from tests.e2e.step_definitions.seller_analytics_access_steps import _register_seller
from tests.e2e.support.world import World

FUNNEL_TIMEOUT_S = 90.0
FUNNEL_POLL_S = 2.0


def _views(world: World, seller_id: str) -> int:
    """Views in `seller_id`'s funnel, read through the gateway as that seller."""
    token = world.state.extra["funnel_tokens"][seller_id]
    world.service_factory.set_token(token)
    resp = world.service_factory.analytics.funnel_response(seller_id)
    assert resp.status_code == 200, f"funnel for {seller_id}: {resp.status_code} {resp.text}"
    # Connect/JSON omits zero values and may encode int64 as a string.
    return int(resp.json().get("views", 0))


@given("two sellers exist")
def two_sellers_exist(world: World) -> None:
    token_a, id_a = _register_seller(world)
    token_b, id_b = _register_seller(world)
    world.state.extra["funnel_tokens"] = {id_a: token_a, id_b: token_b}
    world.state.extra["seller_a_id"] = id_a
    world.state.extra["seller_b_id"] = id_b


@given("seller A has published a listing")
def seller_a_publishes_listing(world: World) -> None:
    seller_a = world.state.extra["seller_a_id"]
    world.service_factory.set_token(world.state.extra["funnel_tokens"][seller_a])
    listing = Listing(title=f"E2E funnel scope {uuid.uuid4().hex[:8]}")
    listing.listing_id = world.service_factory.listing.create_listing(listing)
    world.state.extra["funnel_listing_id"] = listing.listing_id


@when(parsers.parse("{count:d} view beacons are collected for seller A's listing"))
def collect_view_beacons(world: World, count: int) -> None:
    listing_id = world.state.extra["funnel_listing_id"]
    for _ in range(count):
        status = world.service_factory.tracking.emit(
            VIEW,
            listing_id=listing_id,
            session_id=f"e2e-funnel-{uuid.uuid4()}",
            page=f"/listing/{listing_id}",
        )
        assert status in (200, 202, 204), f"beacon should be accepted, got {status}"


@then(parsers.parse("seller A's funnel reports {count:d} views"))
def seller_a_funnel_reports(world: World, count: int) -> None:
    seller_a = world.state.extra["seller_a_id"]
    deadline = time.time() + FUNNEL_TIMEOUT_S
    seen = _views(world, seller_a)
    while seen != count and time.time() < deadline:
        time.sleep(FUNNEL_POLL_S)
        seen = _views(world, seller_a)
    assert seen == count, f"seller A funnel views = {seen}, want {count}"


@then("seller B's funnel reports no views")
def seller_b_funnel_reports_none(world: World) -> None:
    seen = _views(world, world.state.extra["seller_b_id"])
    assert seen == 0, f"seller B funnel counted {seen} view(s) on seller A's listing"
