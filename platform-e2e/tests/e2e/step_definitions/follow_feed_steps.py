"""Follow-feed steps: buyer follows a seller, the seller publishes, the feed shows it.

Everything goes through the gateway as real users. Each scenario registers its own
buyer and seller (private follow graph, parallel-safe). The feed is filled
asynchronously (team-domain outbox -> listing.events -> team-engagement consumer),
so reads poll with a timeout instead of asserting immediately.
"""

from __future__ import annotations

import base64
import json
import time

from pytest_bdd import given, then, when

from config.settings import get_settings
from src.api.services import BaseService
from src.models import Listing, User
from src.utils import data as fake
from tests.e2e.flows import login_via_api, seed_listing
from tests.e2e.support.world import World

SETTINGS = get_settings()

_ENGAGEMENT = "/platform.engagement.v1.EngagementService"
_FEED_POLL_SECONDS = 60
_POLL_INTERVAL = 2


def _principal_id(token: str) -> str:
    """Decode the `sub` claim (principal id) from a gateway JWT."""
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")


def _api(world: World, token: str) -> BaseService:
    svc = BaseService(token=token)
    world.add_cleanup(svc.close)
    return svc


def _register_seller(world: World, prefix: str) -> User:
    name = fake.unique_username(prefix)
    token = world.service_factory.auth.register(name, SETTINGS.seed_password, "seller")
    return User(username=name, password=SETTINGS.seed_password, role="seller", token=token)


def _publish(world: World, seller: User, label: str) -> str:
    listing = Listing(
        title=f"[E2E][Feed] {label} {fake.price_vnd():d}",
        category_id="cat-electronics",
        price=1_000_000,
        stock=5,
        status="published",
        description="Sản phẩm seed tự động cho follow feed E2E.",
    )
    seed_listing(world, listing, seller)
    return listing.listing_id


def _feed(world: World) -> list[str]:
    buyer: User = world.state.extra["feed_buyer"]
    resp = _api(world, buyer.token).post(
        f"{_ENGAGEMENT}/ListFollowedListings", {"page": {"pageSize": 50}}
    )
    return list(resp.get("listingIds") or [])


def _poll_feed(world: World, predicate, seconds: int = _FEED_POLL_SECONDS) -> list[str]:
    """Poll the feed until predicate(ids) holds; return the last ids read."""
    deadline = time.time() + seconds
    ids = _feed(world)
    while not predicate(ids) and time.time() < deadline:
        time.sleep(_POLL_INTERVAL)
        ids = _feed(world)
    return ids


@given("a buyer who follows a seller")
def buyer_follows_seller(world: World) -> None:
    seller = _register_seller(world, "feed_seller")
    buyer = User(
        username=fake.unique_username("feed_buyer"), password=SETTINGS.seed_password, role="buyer"
    )
    login_via_api(world, buyer)
    seller_id = _principal_id(seller.token)
    assert seller_id, "could not derive the seller principal id from its token"
    _api(world, buyer.token).post(f"{_ENGAGEMENT}/FollowSeller", {"sellerId": seller_id})
    world.state.seeded_seller = seller
    world.state.extra.update(feed_buyer=buyer, feed_seller=seller, feed_listings=[])


@when("the seller publishes a listing")
def seller_publishes_listing(world: World) -> None:
    seller: User = world.state.extra["feed_seller"]
    listing_id = _publish(world, seller, "first")
    world.state.extra["feed_listings"].append(listing_id)
    world.logger.info(f"Seller published listing {listing_id}")


@when("the seller publishes a second listing")
def seller_publishes_second_listing(world: World) -> None:
    seller: User = world.state.extra["feed_seller"]
    # Leave a clear gap so the two events carry distinct occurred_at values.
    time.sleep(1.5)
    listing_id = _publish(world, seller, "second")
    world.state.extra["feed_listings"].append(listing_id)
    world.logger.info(f"Seller published second listing {listing_id}")


@when("another seller publishes a listing")
def other_seller_publishes_listing(world: World) -> None:
    other = _register_seller(world, "feed_other_seller")
    world.state.extra["other_listing"] = _publish(world, other, "other")


@when("the listing appears in the buyer's follow feed")
@then("the listing appears in the buyer's follow feed")
def listing_appears_in_feed(world: World) -> None:
    listing_id = world.state.extra["feed_listings"][0]
    ids = _poll_feed(world, lambda got: listing_id in got)
    assert listing_id in ids, (
        f"listing {listing_id} not in the follow feed within {_FEED_POLL_SECONDS}s (got {ids}) — "
        "check team-engagement runs the listing.events consumer (KAFKA_ENABLED=true) and that "
        "team-domain relayed the ListingChanged event"
    )


@then("the buyer's follow feed lists the second listing before the first")
def second_listing_listed_first(world: World) -> None:
    first, second = world.state.extra["feed_listings"][:2]
    ids = _poll_feed(world, lambda got: first in got and second in got)
    assert first in ids and second in ids, f"both listings expected in the feed, got {ids}"
    assert ids.index(second) < ids.index(
        first
    ), f"feed must be newest first: expected {second} before {first}, got {ids}"


@when("the seller deletes the listing")
def seller_deletes_listing(world: World) -> None:
    seller: User = world.state.extra["feed_seller"]
    listing_id = world.state.extra["feed_listings"][0]
    _api(world, seller.token).post(
        "/platform.listing.v1.ListingService/DeleteListing", {"id": listing_id}
    )


@then("the listing leaves the buyer's follow feed")
def listing_leaves_feed(world: World) -> None:
    listing_id = world.state.extra["feed_listings"][0]
    ids = _poll_feed(world, lambda got: listing_id not in got)
    assert (
        listing_id not in ids
    ), f"deleted listing {listing_id} still in the follow feed after {_FEED_POLL_SECONDS}s: {ids}"


@then("the other seller's listing is not in the buyer's follow feed")
def other_listing_not_in_feed(world: World) -> None:
    other = world.state.extra["other_listing"]
    ids = _feed(world)
    assert other not in ids, f"unfollowed seller's listing leaked into the feed: {ids}"
